package http

import (
	"encoding/json"
	"net/http"
)

// encode writes v as JSON with the given status (low-level).
func encode(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// successEnvelope matches the frontend's HttpResponse<T>.
type successEnvelope struct {
	Success bool   `json:"success"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// errorEnvelope matches the frontend's ApiErrorBody.
type errorEnvelope struct {
	Success bool   `json:"success"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// respond writes a success envelope with the given data.
func respond(w http.ResponseWriter, status int, data any) {
	respondMsg(w, status, "", data)
}

// respondMsg writes a success envelope with a human-readable message.
func respondMsg(w http.ResponseWriter, status int, message string, data any) {
	encode(w, status, successEnvelope{Success: true, Status: status, Message: message, Data: data})
}

// writeError writes an error envelope. code is a short machine-readable string;
// message is human-readable (surfaced by the frontend).
func writeError(w http.ResponseWriter, status int, code, message string) {
	encode(w, status, errorEnvelope{Success: false, Status: status, Message: message, Error: code})
}
