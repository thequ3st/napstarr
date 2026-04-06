package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/thequ3st/napstarr/internal/auth"
	"github.com/thequ3st/napstarr/internal/database"
)

type contextKey string

const userContextKey contextKey = "user"

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// Error writes a JSON error response.
func Error(w http.ResponseWriter, message string, status int) {
	JSON(w, status, map[string]string{"error": message})
}

// AuthRequired validates the session cookie and injects the user into the request context.
func AuthRequired(next http.HandlerFunc, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		user, err := auth.ValidateSession(db, cookie.Value)
		if err != nil {
			Error(w, "invalid session", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// AdminRequired checks that the authenticated user is an admin.
func AdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r)
		if user == nil || !user.IsAdmin {
			Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(r *http.Request) *database.User {
	user, _ := r.Context().Value(userContextKey).(*database.User)
	return user
}

// RequestLogger logs each request with method, path, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
