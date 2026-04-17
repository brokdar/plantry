package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/off"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/config"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
	plantrymw "github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
	"github.com/jaltszeimer/plantry/backend/internal/webui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	conn, err := openDB(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := migrate(conn); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	static, err := webui.Handler()
	if err != nil {
		return fmt.Errorf("static: %w", err)
	}

	ingredientRepo := sqlite.NewIngredientRepo(conn)
	ingredientSvc := ingredient.NewService(ingredientRepo)

	// External food providers.
	offClient := off.New()
	offProvider := off.NewProvider(offClient)

	var fdcProvider ingredient.FoodProvider
	if cfg.FDCAPIKey != "" {
		fdcClient := fdc.New(cfg.FDCAPIKey)
		fdcProvider = fdc.NewProvider(fdcClient)
	}

	resolver := ingredient.NewResolver(ingredientRepo, offProvider, fdcProvider)

	// Image store (optional).
	var imgStore *imagestore.Store
	if cfg.ImagePath != "" {
		imgStore, err = imagestore.New(cfg.ImagePath, nil)
		if err != nil {
			return fmt.Errorf("image store: %w", err)
		}
	}

	componentRepo := sqlite.NewComponentRepo(conn)
	componentSvc := component.NewService(componentRepo, ingredientRepo, ingredientRepo)

	slotRepo := sqlite.NewSlotRepo(conn)
	slotSvc := slot.NewService(slotRepo)

	plateRepo := sqlite.NewPlateRepo(conn)
	plateSvc := plate.NewService(plateRepo, slotRepo, componentRepo)

	weekRepo := sqlite.NewWeekRepo(conn)
	txRunner := sqlite.NewTxRunner(conn)
	plannerSvc := planner.NewService(weekRepo, plateRepo, txRunner)

	profileRepo := sqlite.NewProfileRepo(conn)
	profileSvc := profile.NewService(profileRepo)

	templateRepo := sqlite.NewTemplateRepo(conn)
	templateSvc := template.NewService(templateRepo, componentRepo, plateRepo, txRunner)

	// AI wiring (optional — disabled if PLANTRY_AI_PROVIDER is empty).
	var aiHandler *handlers.AIHandler
	if cfg.AIProvider != "" {
		aiRepo := sqlite.NewAIRepo(conn)
		var llmClient llm.Client
		switch cfg.AIProvider {
		case "anthropic":
			llmClient = anthropic.New(cfg.AIAPIKey)
		case "openai":
			llmClient = openai.New(cfg.AIAPIKey)
		case "fake":
			fc, err := fake.New(cfg.AIFakeScript)
			if err != nil {
				return fmt.Errorf("fake llm: %w", err)
			}
			llmClient = fc
		}
		tools, err := agent.NewToolSet(agent.Services{
			Components: componentSvc, Planner: plannerSvc, Plates: plateSvc,
			Profile: profileSvc, Slots: slotSvc, Ingredient: ingredientRepo,
		})
		if err != nil {
			return fmt.Errorf("build tool set: %w", err)
		}
		agentSvc := agent.NewService(aiRepo, llmClient, tools, plannerSvc, profileSvc, cfg.AIModel)
		aiHandler = handlers.NewAIHandler(agentSvc, cfg.AIProvider, cfg.AIModel)
		logger.Info("ai.enabled", "provider", cfg.AIProvider, "model", cfg.AIModel)
	} else {
		aiHandler = handlers.NewAIHandler(nil, "", "")
	}

	// Rate limiter for /api/ai/chat. Kept in main so its janitor goroutine
	// can be wired to the server's shutdown signal.
	aiRateLimiter := plantrymw.NewRateLimiter(cfg.AIRateLimitPerMin)

	h := transport.Handlers{
		Ingredients:   handlers.NewIngredientHandler(ingredientSvc),
		Lookup:        handlers.NewLookupHandler(resolver, imgStore, ingredientSvc),
		Images:        handlers.NewImageHandler(ingredientSvc, imgStore),
		Components:    handlers.NewComponentHandler(componentSvc, imgStore),
		Slots:         handlers.NewSlotHandler(slotSvc),
		Weeks:         handlers.NewWeekHandler(plannerSvc, plateSvc, componentSvc, ingredientRepo),
		Plates:        handlers.NewPlateHandler(plateSvc),
		Profile:       handlers.NewProfileHandler(profileSvc),
		Templates:     handlers.NewTemplateHandler(templateSvc),
		AI:            aiHandler,
		AIRateLimiter: aiRateLimiter,
	}
	handler := transport.NewRouter(logger, static, h)

	srv := &nethttp.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Evict idle rate-limit buckets every 5 minutes, retain 30 minutes.
	janitorStop := make(chan struct{})
	go aiRateLimiter.StartJanitor(janitorStop, 5*time.Minute, 30*time.Minute)
	defer close(janitorStop)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server.listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("server.shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Limit to 1 connection so all queries share the same busy_timeout and
	// WAL pragmas set in the DSN (pragmas are per-connection in SQLite).
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(conn, "migrations")
}
