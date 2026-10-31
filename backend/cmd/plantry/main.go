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
	"strconv"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/llmresolver"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/off"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/config"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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

	// Settings + crypto: editable app settings live in SQLite with env-var
	// fallback. API keys stored in the database are encrypted with
	// PLANTRY_SECRET_KEY, or with a key auto-generated in the data directory.
	if err := cfg.EnsureSecretKey(); err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	cipher, err := crypto.New(cfg.SecretKey)
	if err != nil {
		return fmt.Errorf("crypto: %w", err)
	}
	settingsRepo := sqlite.NewAppSettingsRepo(conn)
	envSnapshot := settingsdom.NewEnvSnapshot(map[string]string{
		"PLANTRY_AI_PROVIDER":           cfg.AIProvider,
		"PLANTRY_AI_MODEL":              cfg.AIModel,
		"PLANTRY_AI_API_KEY":            cfg.AIAPIKey,
		"PLANTRY_AI_RATE_LIMIT_PER_MIN": strconvOrEmpty(cfg.AIRateLimitPerMin),
		"PLANTRY_AI_FAKE_SCRIPT":        cfg.AIFakeScript,
		"PLANTRY_FDC_API_KEY":           cfg.FDCAPIKey,
	})
	settingsSvc := settingsdom.NewService(settingsRepo, envSnapshot, cipher)

	// External food providers.
	// A nil interface (not a nil *off.Provider) so the resolver's nil checks
	// skip OFF entirely.
	var offProvider food.BarcodeProvider
	if !cfg.OFFDisabled {
		offProvider = off.NewProvider(off.New())
	}

	// FDC provider is always constructed — it reads the key from settings
	// per-request and gracefully returns empty results when no key is set.
	fdcProvider := fdc.NewDynamicProvider(settingsSvc)

	foodRepo := sqlite.NewFoodRepo(conn)
	foodSvc := food.NewService(foodRepo).WithPortionProvider(fdcProvider)
	nutritionResolver := food.NewNutritionResolver(foodRepo)
	shoppingResolver := shopping.NewResolver(foodRepo)

	// Image store (optional).
	var imgStore *imagestore.Store
	if cfg.ImagePath != "" {
		imgStore, err = imagestore.New(cfg.ImagePath, nil)
		if err != nil {
			return fmt.Errorf("image store: %w", err)
		}
		foodSvc.WithImageStore(imgStore)
		foodSvc.WithImageSaver(imgStore)
	}

	slotRepo := sqlite.NewSlotRepo(conn)
	slotSvc := slot.NewService(slotRepo)

	txRunner := sqlite.NewTxRunner(conn)
	presetRepo := sqlite.NewPresetRepo(conn)

	// Availability gates where plates may go (D10, D12), so it is built
	// before the plate and preset services.
	availabilitySvc := availability.NewService(sqlite.NewAvailabilityRepo(conn), txRunner, presetRepo)

	profileRepo := sqlite.NewProfileRepo(conn)
	profileSvc := profile.NewService(profileRepo)

	plateRepo := sqlite.NewPlateRepo(conn)
	plateSvc := plate.NewService(plate.Deps{
		Plates:   plateRepo,
		Sessions: sqlite.NewCookSessionRepo(conn),
		Slots:    slotRepo,
		Foods:    foodRepo,
		Cells:    availabilitySvc,
		Profile:  profileSvc,
		Settings: settingsSvc,
		Tx:       txRunner,
	})
	availabilitySvc.WithPlates(plateSvc)

	presetSvc := preset.NewService(presetRepo, foodRepo, plateSvc, txRunner, foodRepo, foodRepo).
		WithSlots(slotSvc).
		WithAvailability(availabilitySvc)

	feedbackSvc := feedback.NewService(txRunner, sqlite.NewFeedbackRepo(conn), plateRepo, foodRepo, profileRepo)

	// AI wiring. The llm.Resolver consults settings on every request so
	// provider / model / API-key changes take effect without a restart.
	// Every client is metered so each LLM call lands in the AI run log.
	// Steps of runs older than ai.run_retention_days are pruned now and after
	// a finished run, at most once per hour.
	runRecorder := airun.NewRecorder(sqlite.NewAIRunRepo(conn), logger, time.Now).
		WithRetention(func(ctx context.Context) (int, error) {
			v, err := settingsSvc.Get(ctx, settingsdom.KeyAIRunRetentionDays)
			if err != nil {
				return 0, err
			}
			return strconv.Atoi(v.Raw)
		})
	runRecorder.Prune(context.Background()) // in the background
	defer runRecorder.Wait()                // runs before conn.Close
	newLLMClient := func(provider, apiKey, fakeScript string) (llm.Client, error) {
		switch provider {
		case "anthropic":
			return anthropic.New(apiKey), nil
		case "openai":
			return openai.New(apiKey), nil
		case "fake":
			return fake.New(fakeScript)
		}
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
	llmFactory := func(provider, apiKey, fakeScript string) (llm.Client, error) {
		client, err := newLLMClient(provider, apiKey, fakeScript)
		if err != nil {
			return nil, err
		}
		return runRecorder.Meter(client, provider), nil
	}
	llmResolver := llmresolver.New(settingsSvc, llmFactory)

	// Food resolver depends on llmResolver for optional AI translation
	// and pick-best ranking; nil llmResolver disables those features.
	resolver := food.NewResolver(foodRepo, offProvider, fdcProvider, llmResolver).WithRecorder(runRecorder)

	aiRepo := sqlite.NewAIRepo(conn)
	tools, err := agent.NewToolSet(agent.Services{
		Foods:             foodSvc,
		NutritionResolver: nutritionResolver,
		Plates:            plateSvc,
		Profile:           profileSvc,
		Slots:             slotSvc,
		Presets:           presetSvc,
		Availability:      availabilitySvc,
	})
	if err != nil {
		return fmt.Errorf("build tool set: %w", err)
	}
	agentSvc := agent.NewService(aiRepo, llmResolver, tools, profileSvc, settingsSvc,
		agent.WindowSources{Plates: plateSvc, Slots: slotSvc, Foods: foodRepo, Availability: availabilitySvc}, time.Now).WithRecorder(runRecorder)
	aiHandler := handlers.NewAIHandler(agentSvc, llmResolver)

	// Recipe importer (Phase 11).
	importFetcher := httpfetch.New()
	importSvc := importer.NewService(importFetcher, jsonld.Extractor{}, llmResolver, resolver, foodSvc).WithRecorder(runRecorder)
	importHandler := handlers.NewImportHandler(importSvc)

	// Rate limiter for /api/ai/chat. Initial limit comes from effective
	// settings; the SettingsHandler reconfigures it when the user changes
	// ai.rate_limit_per_min from the UI.
	initialRate := cfg.AIRateLimitPerMin
	if effective, err := settingsSvc.EffectiveAI(context.Background()); err == nil && effective.RateLimitPerMin > 0 {
		initialRate = effective.RateLimitPerMin
	}
	aiRateLimiter := plantrymw.NewRateLimiter(initialRate)

	settingsHandler := handlers.NewSettingsHandler(settingsSvc, handlers.SystemInfo{
		Port:      cfg.Port,
		DBPath:    cfg.DBPath,
		LogLevel:  cfg.LogLevel.String(),
		ImagePath: cfg.ImagePath,
		DevMode:   cfg.DevMode,
	}, aiRateLimiter)

	h := transport.Handlers{
		Foods:          handlers.NewFoodHandler(foodSvc, nutritionResolver, imgStore),
		Lookup:         handlers.NewLookupHandler(resolver, foodSvc, fdcProvider),
		ImageProxy:     handlers.NewImageProxyHandler(),
		ImageStore:     imgStore,
		Slots:          handlers.NewSlotHandler(slotSvc),
		Plates:         handlers.NewPlateHandler(plateSvc),
		CookSessions:   handlers.NewCookSessionHandler(plateSvc),
		PlanWrite:      handlers.NewPlanWriteHandler(plateSvc),
		Profile:        handlers.NewProfileHandler(profileSvc),
		Presets:        handlers.NewPresetHandler(presetSvc),
		AI:             aiHandler,
		AIRateLimiter:  aiRateLimiter,
		Feedback:       handlers.NewFeedbackHandler(feedbackSvc),
		Import:         importHandler,
		Settings:       settingsHandler,
		ShoppingRange:  handlers.NewShoppingRangeHandler(plateSvc, shoppingResolver),
		NutritionRange: handlers.NewNutritionRangeHandler(plateSvc, nutritionResolver),
		PlateMacros:    handlers.NewPlateMacrosHandler(plateSvc, nutritionResolver),
		Availability:   handlers.NewAvailabilityHandler(availabilitySvc),
		Setup:          handlers.NewSetupHandler(setup.NewService(settingsSvc, foodRepo)),
		DevMode:        cfg.DevMode,
	}
	handler := transport.NewRouter(logger, static, h)

	srv := &nethttp.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute, // generous for AI chat responses
		IdleTimeout:       120 * time.Second,
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

// strconvOrEmpty renders a non-negative int as its decimal form, returning
// "" for zero. Used when building the env snapshot so that an unset env-var
// (zero rate limit) is distinguishable from a configured one.
func strconvOrEmpty(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
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
