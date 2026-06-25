package realtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialHub starts a hub behind an httptest server and returns a connected client.
func dialHub(t *testing.T) (*Hub, *websocket.Conn) {
	t.Helper()
	hub := NewHub("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r, User{ID: "u1", Name: "Ada"})
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/socket.io/?EIO=4&transport=websocket"
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return hub, ws
}

func readFrame(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}

func TestHubHandshakeAndEmit(t *testing.T) {
	hub, ws := dialHub(t)

	// Engine.IO OPEN packet.
	if open := readFrame(t, ws); !strings.HasPrefix(open, "0{") || !strings.Contains(open, `"sid"`) {
		t.Fatalf("expected Engine.IO open, got %q", open)
	}

	// Socket.IO CONNECT → expect a CONNECT ack ("40...").
	if err := ws.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	if ack := readFrame(t, ws); !strings.HasPrefix(ack, "40") {
		t.Fatalf("expected connect ack, got %q", ack)
	}

	// Join an issue room; the first frame back is a presence broadcast.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`42["join","issue:abc"]`)); err != nil {
		t.Fatalf("write join: %v", err)
	}
	if pres := readFrame(t, ws); !strings.HasPrefix(pres, `42["presence"`) || !strings.Contains(pres, "Ada") {
		t.Fatalf("expected presence frame with user, got %q", pres)
	}

	// An emit to that issue room should arrive as a Socket.IO EVENT.
	hub.EmitToIssue("abc", "comment.created", map[string]string{"body": "hi"})
	got := readFrame(t, ws)
	if !strings.HasPrefix(got, `42["comment.created"`) || !strings.Contains(got, `"body":"hi"`) {
		t.Fatalf("expected event frame, got %q", got)
	}
}

func TestHubIgnoresEmitToUnjoinedRoom(t *testing.T) {
	hub, ws := dialHub(t)
	_ = readFrame(t, ws) // open

	// Emit to a room nobody joined; then a ping tick shouldn't carry the event.
	hub.EmitToIssue("nobody", "comment.created", map[string]string{"x": "y"})

	// Next frame within the window should be a ping ("2"), never the event.
	_ = ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return // timeout = no stray event delivered (acceptable)
		}
		if strings.HasPrefix(string(data), `42["comment.created"`) {
			t.Fatalf("received event for a room we never joined")
		}
		if string(data) == "2" {
			return
		}
	}
}
