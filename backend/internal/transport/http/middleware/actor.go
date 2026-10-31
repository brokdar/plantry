package middleware

import (
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

// Actor marks every request's changes as made by the user (D20). The agent
// overrides it for its own tool calls.
func Actor() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(actor.With(r.Context(), actor.User, 0)))
		})
	}
}
