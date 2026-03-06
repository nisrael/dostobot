package main

import (
	"net/http"
)

// ForwardAuthMiddleware enforces that every request has been authenticated by
// authentik via Traefik's forwardAuth middleware. When Traefik's forwardAuth
// approves a request, authentik injects the X-authentik-username header into
// the forwarded request. Go's net/http normalises header names to canonical
// form (e.g. "X-authentik-username" → "X-Authentik-Username"), so the lookup
// below is effectively case-insensitive. If the header is absent the request
// has not gone through authentik and is rejected with 403 Forbidden.
func ForwardAuthMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Authentik-Username") == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}
