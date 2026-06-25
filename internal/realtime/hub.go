package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// This is a minimal, self-contained Socket.IO v4 (Engine.IO v4) server over
// gorilla/websocket — just enough of the protocol to authenticate a connection,
// join/leave rooms, and emit server→client events. It implements Broadcaster.
//
// Frame reference (text frames):
//   Engine.IO: 0 open · 1 close · 2 ping · 3 pong · 4 message
//   Socket.IO (after a "4"): 0 connect · 1 disconnect · 2 event
// So an event frame is "42" + JSON array, e.g. 42["comment.created",{...}].

const (
	pingInterval = 25 * time.Second
	pingTimeout  = 20 * time.Second
	writeWait    = 10 * time.Second
)

// User identifies the authenticated principal behind a connection.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type conn struct {
	ws     *websocket.Conn
	send   chan []byte
	user   User
	hub    *Hub
	mu     sync.Mutex
	rooms  map[string]bool
	closed bool
}

// Hub tracks connections and room membership, and fans events out to rooms.
type Hub struct {
	mu       sync.RWMutex
	rooms    map[string]map[*conn]bool
	upgrader websocket.Upgrader
}

// NewHub builds an empty hub. checkOrigin decides which Origins may connect.
func NewHub(allowedOrigin string) *Hub {
	return &Hub{
		rooms: make(map[string]map[*conn]bool),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return allowedOrigin == "" || origin == "" || origin == allowedOrigin
			},
		},
	}
}

// EmitToIssue sends an event to everyone in an issue's room.
func (h *Hub) EmitToIssue(issueID, event string, payload any) {
	h.emit(IssueRoom(issueID), event, payload)
}

// EmitToOrg sends an event to everyone in an organization's room.
func (h *Hub) EmitToOrg(orgID, event string, payload any) {
	h.emit(OrgRoom(orgID), event, payload)
}

func (h *Hub) emit(room, event string, payload any) {
	frame, err := encodeEvent(event, payload)
	if err != nil {
		slog.Error("realtime: encode event", "event", event, "error", err)
		return
	}
	h.mu.RLock()
	conns := make([]*conn, 0, len(h.rooms[room]))
	for c := range h.rooms[room] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, c := range conns {
		c.enqueue(frame)
	}
}

// ServeHTTP upgrades the request to a websocket and runs the Engine.IO loop. The
// caller must have already authenticated the user.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request, user User) {
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Debug("realtime: upgrade failed", "error", err)
		return
	}
	c := &conn{ws: ws, send: make(chan []byte, 32), user: user, hub: h, rooms: map[string]bool{}}

	// Engine.IO OPEN: hand the client a session id and timing parameters.
	open := map[string]any{
		"sid":          newSID(),
		"upgrades":     []string{},
		"pingInterval": pingInterval.Milliseconds(),
		"pingTimeout":  pingTimeout.Milliseconds(),
		"maxPayload":   1000000,
	}
	if b, err := json.Marshal(open); err == nil {
		c.enqueue(append([]byte{'0'}, b...))
	}

	go c.writePump()
	c.readPump()
}

// readPump processes inbound frames until the connection closes.
func (c *conn) readPump() {
	defer c.close()
	c.ws.SetReadLimit(1 << 20)
	_ = c.ws.SetReadDeadline(time.Now().Add(pingInterval + pingTimeout))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pingInterval + pingTimeout))
	})

	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(pingInterval + pingTimeout))
		c.handleFrame(string(data))
	}
}

func (c *conn) handleFrame(frame string) {
	if frame == "" {
		return
	}
	switch frame[0] {
	case '3': // Engine.IO pong — keep-alive, nothing to do.
		return
	case '2': // Engine.IO ping from client (rare) — reply pong.
		c.enqueue([]byte{'3'})
		return
	case '1': // Engine.IO close.
		c.close()
		return
	case '4': // Engine.IO message → a Socket.IO packet follows.
		c.handleSocketIO(frame[1:])
	}
}

func (c *conn) handleSocketIO(p string) {
	if p == "" {
		return
	}
	switch p[0] {
	case '0': // CONNECT to a namespace → acknowledge the default namespace.
		c.enqueue([]byte(`40{"sid":"` + newSID() + `"}`))
	case '1': // DISCONNECT
		c.close()
	case '2': // EVENT
		c.handleEvent(p[1:])
	}
}

// handleEvent parses a Socket.IO EVENT payload ["name", arg]. Supported client
// events: "join" and "leave", whose arg is a room name string.
func (c *conn) handleEvent(raw string) {
	// Skip an optional ack id between the packet type and the JSON array.
	if i := strings.IndexByte(raw, '['); i > 0 {
		raw = raw[i:]
	}
	var msg []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil || len(msg) == 0 {
		return
	}
	var name string
	if err := json.Unmarshal(msg[0], &name); err != nil {
		return
	}
	var room string
	if len(msg) > 1 {
		_ = json.Unmarshal(msg[1], &room)
	}
	switch name {
	case "join":
		if room != "" {
			c.join(room)
		}
	case "leave":
		if room != "" {
			c.leave(room)
		}
	}
}

// writePump delivers queued frames and sends periodic Engine.IO pings.
func (c *conn) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.ws.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			// Engine.IO PING ("2") plus a websocket ping for transport liveness.
			if err := c.ws.WriteMessage(websocket.TextMessage, []byte{'2'}); err != nil {
				return
			}
		}
	}
}

func (c *conn) enqueue(frame []byte) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	select {
	case c.send <- frame:
		c.mu.Unlock()
	default:
		// Slow consumer: drop the connection rather than block the hub.
		c.mu.Unlock()
		c.close()
	}
}

func (c *conn) join(room string) {
	c.mu.Lock()
	c.rooms[room] = true
	c.mu.Unlock()

	c.hub.mu.Lock()
	if c.hub.rooms[room] == nil {
		c.hub.rooms[room] = make(map[*conn]bool)
	}
	c.hub.rooms[room][c] = true
	c.hub.mu.Unlock()

	c.hub.broadcastPresence(room)
}

func (c *conn) leave(room string) {
	c.mu.Lock()
	delete(c.rooms, room)
	c.mu.Unlock()

	c.hub.removeFromRoom(room, c)
	c.hub.broadcastPresence(room)
}

func (h *Hub) removeFromRoom(room string, c *conn) {
	h.mu.Lock()
	if set := h.rooms[room]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.rooms, room)
		}
	}
	h.mu.Unlock()
}

// broadcastPresence emits the distinct set of users currently in a room.
func (h *Hub) broadcastPresence(room string) {
	h.mu.RLock()
	seen := map[string]bool{}
	users := make([]User, 0)
	for c := range h.rooms[room] {
		if !seen[c.user.ID] {
			seen[c.user.ID] = true
			users = append(users, c.user)
		}
	}
	h.mu.RUnlock()

	payload := map[string]any{"room": room, "users": users}
	h.emit(room, EventPresence, payload)
}

func (c *conn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	rooms := make([]string, 0, len(c.rooms))
	for room := range c.rooms {
		rooms = append(rooms, room)
	}
	c.rooms = map[string]bool{}
	close(c.send)
	c.mu.Unlock()

	for _, room := range rooms {
		c.hub.removeFromRoom(room, c)
		c.hub.broadcastPresence(room)
	}
	_ = c.ws.Close()
}

// encodeEvent builds a Socket.IO EVENT frame: 42["name",payload].
func encodeEvent(event string, payload any) ([]byte, error) {
	body, err := json.Marshal([]any{event, payload})
	if err != nil {
		return nil, err
	}
	return append([]byte("42"), body...), nil
}

func newSID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
