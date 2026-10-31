package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
)

func TestActor_SetsUser(t *testing.T) {
	var got actor.Actor
	h := middleware.Actor()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = actor.From(r.Context())
	}))
	// A stale agent actor on the incoming context must not leak through.
	req := httptest.NewRequest(http.MethodPost, "/api/plates", nil).
		WithContext(actor.With(context.Background(), actor.Agent, 9))
	h.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, actor.Actor{Kind: actor.User}, got)
}
