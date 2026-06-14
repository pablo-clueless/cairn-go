package http

import (
	"encoding/json"
	"io"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// decodeJSON reads and decodes a JSON request body into dst.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
