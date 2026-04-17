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
	Lookup      *handlers.LookupHandler
	Images      *handlers.ImageHandler
	Components  *handlers.ComponentHandler
	Slots       *handlers.SlotHandler
	Weeks       *handlers.WeekHandler
	Plates      *handlers.PlateHandler
	Profile     *handlers.ProfileHandler
	Templates   *handlers.TemplateHandler
}

func NewRouter(logger *slog.Logger, staticHandler http.Handler, h Handlers) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", handlers.Health)

		if h.Components != nil {
			api.Route("/components", func(r chi.Router) {
				r.Get("/", h.Components.List)
				r.Post("/", h.Components.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.Components.Get)
					r.Put("/", h.Components.Update)
					r.Delete("/", h.Components.Delete)
					r.Get("/nutrition", h.Components.Nutrition)
					r.Post("/variant", h.Components.CreateVariant)
					r.Get("/variants", h.Components.ListVariants)
					if h.Components.HasImageStore() {
						r.Post("/image", h.Components.Upload)
						r.Delete("/image", h.Components.DeleteImage)
					}
				})
			})
		}

		if h.Slots != nil {
			api.Route("/settings/slots", func(r chi.Router) {
				r.Get("/", h.Slots.List)
				r.Post("/", h.Slots.Create)
				r.Put("/{id}", h.Slots.Update)
				r.Delete("/{id}", h.Slots.Delete)
			})
		}

		if h.Weeks != nil {
			api.Route("/weeks", func(r chi.Router) {
				r.Get("/", h.Weeks.List)
				r.Get("/current", h.Weeks.Current)
				r.Get("/by-date", h.Weeks.ByDate)
				r.Get("/{id}", h.Weeks.Get)
				r.Post("/{id}/copy", h.Weeks.Copy)
				r.Post("/{id}/plates", h.Weeks.CreatePlate)
				r.Get("/{id}/shopping-list", h.Weeks.ShoppingList)
				r.Get("/{id}/nutrition", h.Weeks.Nutrition)
			})
		}

		if h.Plates != nil {
			api.Route("/plates/{id}", func(r chi.Router) {
				r.Get("/", h.Plates.Get)
				r.Put("/", h.Plates.Update)
				r.Delete("/", h.Plates.Delete)
				r.Post("/components", h.Plates.AddComponent)
				r.Put("/components/{pcId}", h.Plates.UpdateComponent)
				r.Delete("/components/{pcId}", h.Plates.DeleteComponent)
			})
		}

		if h.Profile != nil {
			api.Route("/profile", func(r chi.Router) {
				r.Get("/", h.Profile.Get)
				r.Put("/", h.Profile.Update)
			})
		}

		if h.Templates != nil {
			api.Route("/templates", func(r chi.Router) {
				r.Get("/", h.Templates.List)
				r.Post("/", h.Templates.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.Templates.Get)
					r.Put("/", h.Templates.Update)
					r.Delete("/", h.Templates.Delete)
					r.Post("/apply", h.Templates.Apply)
				})
			})
		}

		api.Route("/ingredients", func(r chi.Router) {
			r.Get("/", h.Ingredients.List)
			r.Post("/", h.Ingredients.Create)

			if h.Lookup != nil {
				r.Get("/lookup", h.Lookup.Lookup)
				r.Post("/resolve", h.Lookup.Resolve)
			}

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.Ingredients.Get)
				r.Put("/", h.Ingredients.Update)
				r.Delete("/", h.Ingredients.Delete)
				r.Get("/portions", h.Ingredients.ListPortions)
				r.Post("/portions", h.Ingredients.UpsertPortion)
				r.Delete("/portions/{unit}", h.Ingredients.DeletePortion)
				if h.Images != nil {
					r.Post("/image", h.Images.Upload)
					r.Delete("/image", h.Images.DeleteImage)
				}
			})
		})
	})

	// Serve stored images as static files.
	if h.Images != nil && h.Images.Store() != nil {
		r.Handle("/images/*", http.StripPrefix("/images/", http.FileServer(http.Dir(h.Images.Store().BasePath()))))
	}

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
