package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// Handlers groups all per-aggregate HTTP handlers for route registration.
type Handlers struct {
	Ingredients *handlers.IngredientHandler
}

func NewRouter(logger *slog.Logger, staticHandler http.Handler, h Handlers) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", handlers.Health)

		api.Route("/ingredients", func(r chi.Router) {
			r.Get("/", h.Ingredients.List)
			r.Post("/", h.Ingredients.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.Ingredients.Get)
				r.Put("/", h.Ingredients.Update)
				r.Delete("/", h.Ingredients.Delete)
				r.Get("/portions", h.Ingredients.ListPortions)
				r.Post("/portions", h.Ingredients.UpsertPortion)
				r.Delete("/portions/{unit}", h.Ingredients.DeletePortion)
			})
		})
	})

	if staticHandler != nil {
		r.Handle("/*", staticHandler)
	}

	return r
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http.request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
