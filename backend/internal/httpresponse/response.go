// Package httpresponse contains the wire-level helpers shared by handlers and
// authorization middleware. Successful response shapes remain endpoint-specific.
package httpresponse

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// DecodeStrict is used for account writes: unsupported fields and explicit null
// values cannot silently bypass validation or masquerade as omitted PATCH fields.
func DecodeStrict(w http.ResponseWriter, r *http.Request, target any) error {
	var raw json.RawMessage
	if err := Decode(w, r, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("null account fields are not allowed")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func Error(w http.ResponseWriter, message string, status int) {
	if status >= http.StatusInternalServerError {
		message = "Internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{message})
}

// Decode accepts one JSON object, limits body size, and rejects trailing input.
// Unknown fields stay compatible with existing clients; strict endpoints retain
// their explicit DisallowUnknownFields decoders.
func Decode(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if len(raw) == 0 || raw[0] != '{' {
		return errors.New("request must contain a JSON object")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("request must contain one JSON object")
	}
	return nil
}

// Router keeps ServeMux's routing, redirects, PathValues and Allow header, while
// giving its default not-found/method-not-allowed responses the same JSON shape.
func Router(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(&routeErrorWriter{ResponseWriter: w}, r)
	})
}

type routeErrorWriter struct {
	http.ResponseWriter
	errorWritten bool
}

func (w *routeErrorWriter) WriteHeader(status int) {
	if status >= 400 {
		w.errorWritten = true
		Error(w.ResponseWriter, http.StatusText(status), status)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *routeErrorWriter) Write(body []byte) (int, error) {
	if w.errorWritten {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}
