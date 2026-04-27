package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
	plantrymw "github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
)

// Handlers groups all per-aggregate HTTP handlers for route registration.
type Handlers struct {
	Foods          *handlers.FoodHandler
	Lookup         *handlers.LookupHandler
	ImageProxy     *handlers.ImageProxyHandler
	ImageStore     *imagestore.Store
	Slots          *handlers.SlotHandler
	Plates         *handlers.PlateHandler
	Profile        *handlers.ProfileHandler
	Templates      *handlers.TemplateHandler
	AI             *handlers.AIHandler
	AIRateLimiter  *plantrymw.RateLimiter
	Feedback       *handlers.FeedbackHandler
	Import         *handlers.ImportHandler
	Settings       *handlers.SettingsHandler
	ShoppingRange  *handlers.ShoppingRangeHandler
	NutritionRange *handlers.NutritionRangeHandler
	DevMode        bool // gates dev-only debug endpoints
}

func NewRouter(logger *slog.Logger, staticHandler http.Handler, h Handlers) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(plantrymw.SecurityHeaders())
	r.Use(requestLogger(logger))

	r.Route("/api", func(api chi.Router) {
		api.Use(plantrymw.MaxBodySize(10 << 20)) // 10 MB cap; accommodates image uploads
		api.Get("/health", handlers.Health)

		if h.Foods != nil {
			api.Route("/foods", func(r chi.Router) {
				r.Get("/", h.Foods.List)
				r.Post("/", h.Foods.Create)
				r.Get("/insights", h.Foods.Insights)

				if h.Lookup != nil {
					r.Get("/lookup", h.Lookup.Lookup)
					r.Post("/resolve", h.Lookup.Resolve)
				}

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.Foods.Get)
					r.Put("/", h.Foods.Update)
					r.Delete("/", h.Foods.Delete)
					r.Get("/nutrition", h.Foods.Nutrition)
					r.Post("/favorite", h.Foods.SetFavorite)
					r.Post("/variant", h.Foods.CreateVariant)
					r.Get("/variants", h.Foods.ListVariants)
					r.Get("/portions", h.Foods.ListPortions)
					r.Post("/portions", h.Foods.UpsertPortion)
					r.Delete("/portions/{unit}", h.Foods.DeletePortion)
					r.Post("/sync-portions", h.Foods.SyncPortions)
					if h.Lookup != nil {
						r.Post("/refetch", h.Lookup.Refetch)
					}
					if h.Foods.HasImageStore() {
						r.Post("/image", h.Foods.Upload)
						r.Delete("/image", h.Foods.DeleteImage)
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

		if h.Plates != nil {
			api.Route("/plates", func(r chi.Router) {
				r.Get("/", h.Plates.List)
				r.Post("/", h.Plates.Create)
				r.Get("/by-date/{date}", h.Plates.Day)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.Plates.Get)
					r.Put("/", h.Plates.Update)
					r.Delete("/", h.Plates.Delete)
					r.Post("/skip", h.Plates.SetSkipped)
					r.Post("/components", h.Plates.AddComponent)
					r.Put("/components/{pcId}", h.Plates.UpdateComponent)
					r.Delete("/components/{pcId}", h.Plates.DeleteComponent)
					if h.Feedback != nil {
						r.Put("/feedback", h.Feedback.Put)
						r.Delete("/feedback", h.Feedback.Delete)
					}
				})
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

		if h.AI != nil {
			limiter := h.AIRateLimiter
			if limiter == nil {
				limiter = plantrymw.NewRateLimiter(0)
			}
			api.Route("/ai", func(r chi.Router) {
				r.With(limiter.Middleware("error.ai.rate_limit_exceeded")).
					Post("/chat", h.AI.Chat)
				r.Get("/conversations", h.AI.ListConversations)
				r.Route("/conversations/{id}", func(r chi.Router) {
					r.Get("/", h.AI.GetConversation)
					r.Delete("/", h.AI.DeleteConversation)
				})
				if h.DevMode {
					r.Get("/debug/system-prompt", h.AI.DebugSystemPrompt)
				}
			})
		}

		if h.Settings != nil {
			api.Route("/settings", func(r chi.Router) {
				r.Get("/", h.Settings.List)
				r.Put("/{key}", h.Settings.Set)
				r.Delete("/{key}", h.Settings.Delete)
				r.Get("/system", h.Settings.System)
				r.Get("/ai", h.Settings.AISummary)
				r.Get("/ai/models", h.Settings.Models)
			})
		}

		if h.ImageProxy != nil {
			api.Post("/image/fetch-url", h.ImageProxy.Fetch)
		}

		if h.Import != nil {
			api.Route("/import", func(r chi.Router) {
				r.Post("/extract", h.Import.Extract)
				r.Post("/resolve", h.Import.Resolve)
				r.Get("/lookup", h.Import.LookupLine)
			})
		}

		if h.ShoppingRange != nil {
			api.Get("/shopping-list", h.ShoppingRange.List)
		}

		if h.NutritionRange != nil {
			api.Get("/nutrition", h.NutritionRange.List)
		}
	})

	// Serve stored images as static files.
	if h.ImageStore != nil {
		r.Handle("/images/*", http.StripPrefix("/images/", http.FileServer(http.Dir(h.ImageStore.BasePath()))))
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
