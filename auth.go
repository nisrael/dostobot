package main

import (
	"crypto/subtle"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// Auth holds credentials for HTTP Basic Auth.
type Auth struct {
	username     string
	passwordHash []byte
}

// newAuth creates an Auth from a username and a password value.
// passwordHash may be a bcrypt hash (starting with $2a$ or $2b$) or a
// plaintext password that will be hashed immediately. If empty, a default
// insecure password ("changeme") is used with a warning.
func newAuth(username, passwordValue string) *Auth {
	if passwordValue == "" {
		log.Println("WARNING: AUTH_PASSWORD_HASH is not set – using default insecure password 'changeme'. Set AUTH_PASSWORD_HASH to a bcrypt hash.")
		hash, _ := bcrypt.GenerateFromPassword([]byte("changeme"), bcrypt.DefaultCost)
		return &Auth{username: username, passwordHash: hash}
	}
	// Detect existing bcrypt hashes ($2a$, $2b$, $2y$)
	if len(passwordValue) > 4 && passwordValue[:3] == "$2" {
		return &Auth{username: username, passwordHash: []byte(passwordValue)}
	}
	// Treat as plaintext – hash it now
	log.Println("INFO: AUTH_PASSWORD_HASH is a plaintext value – hashing it with bcrypt.")
	hash, err := bcrypt.GenerateFromPassword([]byte(passwordValue), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("auth: failed to hash password: %v", err)
	}
	return &Auth{username: username, passwordHash: hash}
}

// Middleware wraps h and requires valid HTTP Basic Auth credentials.
func (a *Auth) Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || !a.check(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="DostoBot"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// check validates username/password using constant-time comparison.
func (a *Auth) check(username, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) == 1
	passOK := bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password)) == nil
	return userOK && passOK
}
