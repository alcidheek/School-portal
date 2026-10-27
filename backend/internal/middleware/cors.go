package middleware

import (
	"net/http"
)

// CORSMiddleware allows the React frontend to communicate with the API.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := "http://localhost:5173"

		// Allow requests from our React frontend.
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Vary", "Origin")

		// Allow frontend request headers.
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization",
		)

		// Allow the HTTP methods used by the API.
		w.Header().Set(
			"Access-Control-Allow-Methods",
			"GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS",
		)

		// Handle browser preflight requests.
		if r.Method == http.MethodOptions &&
			r.Header.Get("Origin") == allowedOrigin &&
			r.Header.Get("Access-Control-Request-Method") != "" {

			method := r.Header.Get("Access-Control-Request-Method")

			switch method {
			case http.MethodGet,
				http.MethodHead,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return

			default:
				http.Error(
					w,
					"Unsupported preflight method",
					http.StatusBadRequest,
				)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
