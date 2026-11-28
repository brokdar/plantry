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
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdclocal"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/llmresolver"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/off"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/config"
	"github.com/jaltszeimer/plantry/backend/internal/devclock"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chefhelp"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
	plantrymw "github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
	"github.com/jaltszeimer/plantry/backend/internal/webui"
)

// version is set at build time: -ldflags "-X main.version=<git describe>".
var version = "dev"

// commit is set at build time: -ldflags "-X main.commit=<short sha>".
var commit = ""

func main() {
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if err := runBackup(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
		return
	}
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

	fresh, err := isFreshDB(conn)
	if err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
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
	if err := seedFreshInstall(context.Background(), settingsSvc, fresh); err != nil {
		return fmt.Errorf("seed fresh install: %w", err)
	}

	// External food providers.
	// A nil interface (not a nil *off.Provider) so the resolver's nil checks
	// skip OFF entirely.
	var offProvider food.BarcodeProvider
	if !cfg.OFFDisabled {
		offProvider = off.NewProvider(off.New())
	}

	// FDC provider is always constructed — it reads the key from settings
	// per-request and gracefully returns empty results when no key is set.
	fdcProvider := fdc.NewDynamicProvider(settingsSvc).WithFallback(fdclocal.New())

	foodRepo := sqlite.NewFoodRepo(conn)
	foodSvc := food.NewService(foodRepo).WithPortionProvider(fdcProvider)
	nutritionResolver := food.NewNutritionResolver(foodRepo)
	shoppingSvc := shopping.NewService(shopping.Deps{
		Source: sqlite.NewShoppingRepo(conn), Foods: foodRepo, Settings: settingsSvc, Slots: sqlite.NewSlotRepo(conn),
	})

	// Image store (optional).
	var imgStore *imagestore.Store
	if cfg.ImagePath != "" {
		imgStore, err = imagestore.New(cfg.ImagePath, nil)
		if err != nil {
			return fmt.Errorf("image store: %w", err)
		}
		// Downloads on behalf of a fetched page (discovery) dial public
		// addresses only; dev mode allows loopback for the e2e fixture site.
		imgStore.SetGuardedClient(httpfetch.GuardedClient(nil, cfg.DevMode))
		foodSvc.WithImageStore(imgStore)
		foodSvc.WithImageSaver(imgStore)
	}

	slotRepo := sqlite.NewSlotRepo(conn)
	slotSvc := slot.NewService(slotRepo)

	txRunner := sqlite.NewTxRunner(conn)
	slotSvc.WithTx(txRunner)
	presetRepo := sqlite.NewPresetRepo(conn)

	// Availability gates where plates may go (D10, D12), so it is built
	// before the plate and preset services.
	availabilitySvc := availability.NewService(sqlite.NewAvailabilityRepo(conn), txRunner, presetRepo)

	profileRepo := sqlite.NewProfileRepo(conn)
	profileSvc := profile.NewService(profileRepo)

	plateRepo := sqlite.NewPlateRepo(conn)
	sessionRepo := sqlite.NewCookSessionRepo(conn)
	aiRunRepo := sqlite.NewAIRunRepo(conn)
	// No run survives a stop: end the rows a stop or a crash left running.
	if n, err := aiRunRepo.FailInterruptedRuns(context.Background(), time.Now()); err != nil {
		return fmt.Errorf("end interrupted ai runs: %w", err)
	} else if n > 0 {
		logger.Warn("ai.run.interrupted", "runs", n)
	}
	clock := devclock.New()
	if !cfg.DevClock.IsZero() {
		clock.Set(cfg.DevClock)
	}
	plateSvc := plate.NewService(plate.Deps{
		Now:      clock.Now,
		Plates:   plateRepo,
		Sessions: sessionRepo,
		Slots:    slotRepo,
		Foods:    foodRepo,
		Cells:    availabilitySvc,
		Profile:  profileSvc,
		Settings: settingsSvc,
		Events:   sqlite.NewPlateEventRepo(conn),
		Outcomes: aiRunRepo,
		Tx:       txRunner,

		EffectiveRole: shopping.RoleOf,
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
	// A finished run stores its cost at the prices of that moment: the
	// built-in table plus ai.price_overrides.
	priceOverrides := func(ctx context.Context) (map[string]airun.Price, error) {
		v, err := settingsSvc.Get(ctx, settingsdom.KeyAIPriceOverrides)
		if err != nil {
			return nil, err
		}
		return airun.ParsePriceOverrides(v.Raw)
	}
	runRecorder := airun.NewRecorder(aiRunRepo, logger, time.Now).
		WithAppVersion(appVersion()).
		WithPrices(priceOverrides).
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

	// Plan generation (Phase 3 B6, B7): runs in the background. Shutdown
	// cancels the running ones and waits a bounded time for them to finish
	// their run rows (before the recorder and the database). Without a
	// chooser in the request it uses the LLM when a provider is configured,
	// else the heuristic (D11).
	planLight := sqlite.NewPlanLightRepo(conn)
	foodAttributeRepo := sqlite.NewFoodAttributeRepo(conn)
	// Food change log (VAR-5): each producer registers its Restorer here.
	foodRestorers := map[foodevent.Kind]foodevent.Restorer{}
	foodEventRepo := sqlite.NewFoodEventRepo(conn)
	foodEventSvc := foodevent.NewService(sqlite.NewTxRunner(conn), foodEventRepo, foodRestorers, clock.Now)
	foodSvc.WithEvents(foodEventSvc)
	foodRestorers[foodevent.KindRecipe] = foodSvc
	// Recipe attributes (ATTR): derived on read, user edits logged as
	// attribute food events.
	flavorSvc := flavor.NewService(foodAttributeRepo, foodRepo, nutritionResolver, foodEventSvc)
	foodRestorers[foodevent.KindAttribute] = flavorSvc
	// Taste (TASTE): signals from ratings, hand edits, favourites and tips;
	// the planner's soft taste key.
	tasteSvc := taste.NewService(taste.Deps{
		Store: sqlite.NewTasteRepo(conn), Attributes: flavorSvc, Foods: foodRepo,
		Events: foodEventSvc, History: foodEventRepo, Now: clock.Now,
	})
	foodRestorers[foodevent.KindTasteItem] = tasteSvc
	plateSvc.WithTaste(tasteSvc)
	feedbackSvc.WithTaste(tasteSvc).WithTrials(foodSvc)
	foodSvc.WithFavourites(tasteSvc)
	// Kitchen: the foods the household never eats keep recipes holding them
	// out of every auto-placed plate (planning) and out of tips.
	kitchenRepo := sqlite.NewKitchenRepo(conn)
	kitchenSvc := kitchen.NewService(kitchenRepo)
	neverFoods := func(ctx context.Context) (map[int64]bool, error) {
		items, err := kitchenSvc.NeverItems(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[int64]bool, len(items))
		for _, f := range items {
			out[f.ID] = true
		}
		return out, nil
	}
	var themeSvc *theme.Service // needs the builder
	planBuilder := planning.NewBuilder(planning.BuilderDeps{
		Light: planLight, Attributes: foodAttributeRepo,
		Foods: foodSvc, FoodRows: foodRepo, Nutrition: nutritionResolver, Cells: availabilitySvc,
		Plates: plateRepo, Sessions: sessionRepo, Presets: presetRepo, Slots: slotRepo,
		Settings: settingsSvc, Profile: profileSvc, Corrections: sqlite.NewPlateEventRepo(conn),
		Version: sqlite.NewDataVersion(conn), Taste: tasteSvc, Never: neverFoods,
		Themes: func(ctx context.Context, start time.Time) ([]theme.Theme, error) {
			return themeSvc.Themes(ctx, start)
		},
	})
	readyFinds := &lazyFinds{}
	var findSvc *finds.Service // built after the planner
	themeSvc = theme.NewService(theme.Deps{
		Store: sqlite.NewThemeRepo(conn), ChefOn: settingsSvc.ChefEnabled,
		Library: func(ctx context.Context, t theme.Theme) (int, error) {
			return planBuilder.ThemeLibrary(ctx, t, clock.Now())
		},
		Find: func(ctx context.Context, t string, n int) error {
			return findSvc.ThemeSet(ctx, t, n)
		},
		Cuisines: settingsSvc.DiscoveryCuisines,
		Loves:    lovedCuisines(tasteSvc.View),
		Now:      clock.Now,
	})
	planningSvc := planning.NewService(planning.Deps{
		Builder:     planBuilder,
		Finds:       readyFinds,
		ThemeDishes: themeSvc.NewDishes,
		Light:       planLight,
		Plates:      plateSvc, PlateRows: plateRepo, Sessions: sessionRepo, Cells: availabilitySvc, Overrides: availabilitySvc, FoodRows: foodRepo, Settings: settingsSvc,
		RunLog: planning.RecorderLog{Recorder: runRecorder}, Runs: aiRunRepo, Tx: txRunner,
		Progress: planning.NewProgress(),
		Choosers: map[airun.Chooser]planning.Chooser{
			airun.ChooserHeuristic: planning.Heuristic{},
		},
		// PL2: a model only reads the wish into constraints; the heuristic plans.
		Extractor: planning.NewLLMExtractor(llmResolver).WithEffort(func(ctx context.Context) string {
			v, err := settingsSvc.Get(ctx, settingsdom.KeyAIReasoningEffortGenerate)
			if err != nil {
				slog.WarnContext(ctx, "plan.effort.setting", "error", err)
				return ""
			}
			return v.Raw
		}),
		Logger: logger,
	})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), planningShutdownWait)
		defer cancel()
		if err := planningSvc.Shutdown(ctx); err != nil {
			logger.Warn("plan.shutdown runs still running", "error", err)
		}
	}()

	planViewSvc := planview.NewService(planview.Deps{
		Plates:       plateRepo,
		Sessions:     sessionRepo,
		Foods:        foodRepo,
		Nutrition:    nutritionResolver,
		Availability: availabilitySvc,
		Slots:        slotRepo,
		Settings:     settingsSvc,
		Profile:      profileSvc,
		Recipes:      foodRepo,
		Runs:         aiRunRepo,
		Diets:        foodSvc,
		Light:        planLight,
		Progress:     planningSvc,
		Prep: func(ctx context.Context, ids []int64) (map[int64]*food.Food, error) {
			if on, err := settingsSvc.ChefEnabled(ctx); err != nil || !on {
				return nil, err
			}
			return loadRecipes(ctx, foodRepo, ids)
		},
		Steps: func(ctx context.Context, ids []int64) (map[int64]*food.Food, error) {
			return loadRecipes(ctx, foodRepo, ids)
		},
		Tx: txRunner,
	})

	// Food resolver depends on llmResolver for optional AI translation
	// and pick-best ranking; nil llmResolver disables those features.
	resolver := food.NewResolver(foodRepo, offProvider, fdcProvider, llmResolver).WithRecorder(runRecorder)
	// The one matching path (MATCH-4): imports, the review's re-search and
	// Discover. Its chef_match call asks the Chef guard (set below).
	var matchGuard *chef.Guard
	var offSearcher food.LangSearcher
	if ls, ok := offProvider.(food.LangSearcher); ok {
		offSearcher = ls
	}
	matcher := food.NewMatcher(foodRepo, fdcProvider, offSearcher).WithPicker(llmResolver, runRecorder,
		func(ctx context.Context, requested bool) (food.MatchPermit, error) {
			d, err := matchGuard.Check(ctx, airun.KindChefMatch, requested)
			// Chef off with a provider set: AI translation and pick stay on
			// as the uncapped lookup run kind, as before the Chef existed.
			return food.MatchPermit{Run: d.Run, Warn: d.Warn, Uncapped: d.Reason == chef.ReasonOff}, err
		})

	// Assistant recipe changes (VAR-1, VAR-2, HELP-4): filled variants and
	// in-place edits through matching and the gates, a chef_help run on the
	// recipe page.
	recipeChangeSvc := recipechange.NewService(foodRepo, sqlite.NewFoodOriginRepo(conn), matcher,
		sqlite.NewRecipeChangeRepo(conn), profileSvc, foodEventSvc).WithModel(llmResolver, runRecorder)
	foodRestorers[foodevent.KindRecipeContent] = recipeChangeSvc
	aiRepo := sqlite.NewAIRepo(conn)
	changeRepo := sqlite.NewAIChangeRepo(conn)
	changes := agent.NewChanges(changeRepo, plateSvc, availabilitySvc, planLight, txRunner, clock.Now)
	findRequester := &lazyFinds{}
	tools, err := agent.NewToolSet(agent.Services{
		Foods:             foodSvc,
		NutritionResolver: nutritionResolver,
		Plates:            plateSvc,
		Profile:           profileSvc,
		Slots:             slotSvc,
		Presets:           presetSvc,
		Availability:      availabilitySvc,
		Settings:          settingsSvc,
		PlanView:          planViewSvc,
		Planner:           planningSvc,
		Cells:             availabilitySvc,
		Changes:           changeRepo,
		State: func(ctx context.Context, w window.Window, today time.Time) (planning.State, error) {
			return planBuilder.Build(ctx, w, today, w.Start, w.End)
		},
		Light:   planLight,
		Tx:      txRunner,
		Undo:    changes,
		Recipes: agent.RecipeTools{Changes: recipeChangeSvc, Attributes: flavorSvc},
		Taste:   tasteSvc,
		Finds:   agent.FindTools{Requester: findRequester, ChefOn: settingsSvc.ChefEnabled},
		Themes:  agent.ThemeTools{Setter: themeSvc, ChefOn: settingsSvc.ChefEnabled},
	})
	if err != nil {
		return fmt.Errorf("build tool set: %w", err)
	}
	agentSvc := agent.NewService(aiRepo, llmResolver, tools, profileSvc, settingsSvc,
		agent.PlanContext{Build: planBuilder.Build, Summary: planning.RenderPlanSummary}, clock.Now).WithRecorder(runRecorder)
	aiHandler := handlers.NewAIHandler(agentSvc, changes, llmResolver)

	// Run log read side: runs, insights and prices. Costs are computed on
	// read from the built-in table plus ai.price_overrides.
	insightsSvc := airun.NewInsights(aiRunRepo, priceOverrides)
	aiRunsHandler := handlers.NewAIRunsHandler(insightsSvc, func(ctx context.Context) (string, error) {
		cfg, err := settingsSvc.EffectiveAI(ctx)
		return cfg.Model, err
	})

	// Recipe importer (Phase 11).
	// Guarded fetches (discovery's LLM-supplied URLs) are re-checked at dial
	// time; user-pasted import URLs stay unguarded.
	var devHandler *handlers.DevHandler
	if cfg.DevMode {
		devHandler = handlers.NewDevHandler(clock)
	}
	importFetcher := httpfetch.New(httpfetch.WithDialGuard(nil, cfg.DevMode))
	importSvc := importer.NewService(importFetcher, jsonld.Extractor{}, llmResolver, resolver, foodSvc).WithRecorder(runRecorder).WithMatcher(matcher).
		WithChecker(importer.NewChecker(llmResolver, runRecorder, func(ctx context.Context, requested bool) (importer.CheckPermit, error) {
			d, err := matchGuard.Check(ctx, airun.KindChefImportCheck, requested)
			return importer.CheckPermit{Run: d.Run, Warn: d.Warn}, err
		}))
	importHandler := handlers.NewImportHandler(importSvc)

	// Discovery (D7): research → verify → annotate → store, one run
	// at a time in the background. LLM-supplied URLs
	// pass the public-URL guard (dev mode allows loopback for the
	// e2e fixture site). Shutdown waits for the running run (before the
	// recorder and the database), cancelling it when the wait runs out.
	suggestionRepo := sqlite.NewRecipeSuggestionRepo(conn)
	discoveryGuard := func(u string) error { return httpfetch.PublicURL(u, nil, cfg.DevMode) }
	suggestionStore := &discovery.Store{Repo: suggestionRepo}
	if imgStore != nil {
		suggestionStore.Images = imgStore
	}
	discoverySvc := discovery.NewService(discovery.Deps{
		LLM:       llmResolver,
		Prefs:     settingsSvc,
		ChefOn:    settingsSvc.ChefEnabled,
		Foods:     foodRepo,
		Recorder:  runRecorder,
		Profile:   profileSvc,
		Catalogue: foodRepo,
		Verifier:  &discovery.Verifier{Extractor: importSvc, Index: suggestionRepo, Guard: discoveryGuard},
		Annotator: &discovery.Annotator{Matcher: matcher, Used: foodRepo, Nutrition: nutritionResolver},
		Store:     suggestionStore,
		Logger:    logger,
	})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), discoveryShutdownWait)
		defer cancel()
		if err := discoverySvc.Shutdown(ctx); err != nil {
			logger.Warn("discovery.shutdown run cancelled", "error", err)
		}
	}()

	// Capped Chef work (COST-1..3): every capped run asks the guard; queued
	// work runs in the background, at most chefJobConcurrency at a time.
	// Boot recovery requeues jobs a crash or shutdown left running.
	// Shutdown waits for running jobs (before the recorder and the
	// database), cancelling them when the wait runs out.
	// Batch substitutions (HELP-3, HELP-7): rule-based, no provider needed.
	// A table substitute the catalogue lacks is added through the matching
	// path; options the household cannot eat are left out (SUB-1, SUB-3).
	substitutionSvc := substitution.NewService(foodRepo, sessionRepo, sqlite.NewSubstitutionRepo(conn), recipeChangeSvc, foodEventSvc).
		WithCatalogue(matcher.WithoutPicker(), sqlite.NewFoodOriginRepo(conn)).
		WithHousehold(gatesHousehold(profileSvc.Get, kitchenRepo.NeverItems))
	foodRestorers[foodevent.KindSubstitution] = substitutionSvc
	chefGuard := &chef.Guard{Settings: settingsSvc, Spend: aiRunRepo, Prices: priceOverrides, Now: time.Now}
	matchGuard = chefGuard
	// Cook-mode help (HELP-2): explain a step, what went wrong; always runs,
	// its cost counts against the cap (HELP-8).
	helpSvc := chefhelp.NewService(sessionRepo, foodRepo, profileSvc, settingsSvc, chefGuard, llmResolver, runRecorder)
	// Attribute labelling (ATTR-2): a chef_attributes job per saved recipe
	// and the one-time library pass, each a capped run.
	var labelling *flavor.Labelling
	chefRunner := chef.NewRunner(chef.RunnerConfig{
		Store: sqlite.NewChefJobRepo(conn), Guard: chefGuard,
		Handlers: map[airun.Kind]chef.Handler{
			airun.KindChefAttributes: func(ctx context.Context, job chef.Job) error { return labelling.Handle(ctx, job.Payload) },
			airun.KindChefFind:       func(ctx context.Context, job chef.Job) error { return findSvc.Handle(ctx, job.Payload) },
		},
		Concurrency: chefJobConcurrency, MaxAttempts: chefJobMaxAttempts, Now: time.Now, Logger: logger,
		OnUnfinished: func(ctx context.Context, job chef.Job, status chef.Status, errText string) {
			if job.Kind == airun.KindChefFind {
				findSvc.Unfinished(ctx, job.Payload, status == chef.StatusSkipped, errText)
			}
		},
	})
	labelling = flavor.NewLabelling(
		flavor.NewLabeler(foodAttributeRepo, foodRepo, nutritionResolver, txRunner, llmResolver, runRecorder),
		foodAttributeRepo, chefRunner, settingsSvc,
		func(ctx context.Context, input, output int64) (*float64, error) {
			ai, err := settingsSvc.EffectiveAI(ctx)
			if err != nil {
				return nil, err
			}
			overrides, err := priceOverrides(ctx)
			if err != nil {
				return nil, err
			}
			return airun.Cost(ai.Model, ai.Provider, airun.Tokens{Input: input, Output: output}, overrides), nil
		},
		chefJobConcurrency,
	)
	// Finds (FIND, C2): requested finds run on the chef runner; fetching is
	// polite (robots, rate limit, backoff) and guarded like discovery.
	houseCuisines := householdCuisines(profileSvc.Get, lovedCuisines(tasteSvc.View),
		stapleCuisines(tasteSvc.Staples, foodAttributeRepo.Many, time.Now), settingsSvc.DiscoveryCuisines, foodAttributeRepo.LibraryCuisines)
	findRepo := sqlite.NewFindRepo(conn)
	reviewRepo := sqlite.NewLibraryReviewRepo(conn)
	politeFetcher := httpfetch.NewPolite(httpfetch.New(httpfetch.WithDialGuard(nil, cfg.DevMode)),
		httpfetch.WithAIGroupsExempt(aiGroupsExempt(settingsSvc.ChefSites)))
	findSvc = finds.NewService(finds.Deps{
		Store: findRepo, LLM: llmResolver, Runs: runRecorder,
		Fetcher: politeFetcher,
		Robots: func(ctx context.Context, domain string) bool {
			return politeFetcher.AllowsRecipes(ctx, "https://"+domain)
		},
		Extractor: importSvc, Matcher: matcher,
		Household: findHousehold(profileSvc.Get, sqlite.NewKitchenRepo(conn).NeverItems, tasteSvc.View, foodRepo.LeafNamesByRecipeUse, houseCuisines, foodRepo.HasCookHistory, foodRepo.RecipeNames),
		Sites:     settingsSvc.ChefSites,
		Enqueue: func(ctx context.Context, id int64, requested bool) error {
			_, err := chefRunner.Enqueue(ctx, airun.KindChefFind, strconv.FormatInt(id, 10), requested)
			return err
		},
		Taste: tasteSvc, Tx: txRunner, Logger: logger,
		Planning: finds.Planning{
			Library: finds.NewLibrary(finds.LibraryDeps{
				Foods: foodSvc, Catalogue: foodRepo, Origins: sqlite.NewFoodOriginRepo(conn),
				Finalizer: importSvc, Plates: plateSvc,
			}),
			Gaps:   reviewRepo,
			ChefOn: settingsSvc.ChefEnabled,
			NewDishes: func(ctx context.Context) (int, error) {
				return settingsdom.NewDishesPerWeek(ctx, settingsSvc)
			},
			Cuisines: houseCuisines,
			Now:      time.Now,
		},
	})
	findRequester.svc = findSvc
	readyFinds.svc = findSvc
	foodSvc.WithSaved(func(ctx context.Context, id int64) {
		if err := labelling.Saved(ctx, id); err != nil {
			logger.WarnContext(ctx, "chef.labelling.queue failed", "food_id", id, "error", err)
		}
	})
	if err := chefRunner.Start(context.Background()); err != nil {
		return err
	}
	if err := findSvc.FailStranded(context.Background()); err != nil {
		logger.Warn("finds.stranded sweep failed", "error", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), chefShutdownWait)
		defer cancel()
		if err := chefRunner.Shutdown(ctx); err != nil {
			logger.Warn("chef.shutdown jobs cancelled", "error", err)
		}
	}()

	// Chef tips (TIP): computed by rules on read of a window, applied as a
	// normal plate change with the plate Undo token.
	tipSvc := tips.NewService(tips.Deps{
		Store: sqlite.NewChefTipRepo(conn), Plates: plateSvc, Taste: tasteSvc,
		State: func(ctx context.Context, w window.Window, today time.Time) (planning.State, error) {
			return planBuilder.Build(ctx, w, today, w.Start, w.End)
		},
		ChefOn: settingsSvc.ChefEnabled,
		ShoppingDay: func(ctx context.Context) (int, error) {
			return plate.IntSetting(ctx, settingsSvc, settingsdom.KeyPlanShoppingDay)
		},
		Never: neverFoods,
		Equipment: func(ctx context.Context) ([]string, error) {
			p, err := profileSvc.Get(ctx)
			if err != nil {
				return nil, err
			}
			return p.Kitchen.Equipment, nil
		},
		Now: clock.Now,
	})
	spendByKind := func(ctx context.Context, t time.Time) ([]handlers.KindSpend, error) {
		out := make([]handlers.KindSpend, 0, len(chef.CappedKinds))
		for _, k := range chef.CappedKinds {
			usd, err := airun.Spend(ctx, aiRunRepo, t, []airun.Kind{k})
			if err != nil {
				return nil, err
			}
			out = append(out, handlers.KindSpend{Kind: k, USD: usd})
		}
		return out, nil
	}

	// Rate limiter for /api/ai/chat. Initial limit comes from effective
	// settings; the SettingsHandler reconfigures it when the user changes
	// ai.rate_limit_per_min from the UI.
	initialRate := cfg.AIRateLimitPerMin
	if effective, err := settingsSvc.EffectiveAI(context.Background()); err == nil && effective.RateLimitPerMin > 0 {
		initialRate = effective.RateLimitPerMin
	}
	aiRateLimiter := plantrymw.NewRateLimiter(initialRate)

	settingsHandler := handlers.NewSettingsHandler(settingsSvc, handlers.SystemInfo{
		Port:        cfg.Port,
		DBPath:      cfg.DBPath,
		LogLevel:    cfg.LogLevel.String(),
		ImagePath:   cfg.ImagePath,
		DevMode:     cfg.DevMode,
		Version:     version,
		BuildCommit: commit,
	}, aiRateLimiter)

	h := transport.Handlers{
		Foods:        handlers.NewFoodHandler(foodSvc, nutritionResolver, imgStore).WithClock(clock.Now),
		Lookup:       handlers.NewLookupHandler(resolver, foodSvc, fdcProvider),
		ImageProxy:   handlers.NewImageProxyHandler(),
		ImageStore:   imgStore,
		Slots:        handlers.NewSlotHandler(slotSvc),
		Plates:       handlers.NewPlateHandler(plateSvc),
		CookSessions: handlers.NewCookSessionHandler(plateSvc),
		PlanWrite:    handlers.NewPlanWriteHandler(plateSvc),
		PlanRead:     handlers.NewPlanReadHandler(planViewSvc).WithClock(clock.Now),
		PlanGenerate: handlers.NewPlanGenerateHandler(planningSvc).WithClock(clock.Now),
		Today:        handlers.NewTodayHandler(planViewSvc).WithClock(clock.Now),
		Profile:      handlers.NewProfileHandler(profileSvc),
		Kitchen:      handlers.NewKitchenHandler(profileSvc, kitchenSvc),
		Taste:        handlers.NewTasteHandler(tasteSvc).WithKitchen(kitchenSvc, foodSvc, profileSvc),
		Themes:       handlers.NewThemeHandler(themeSvc),
		Finds:        handlers.NewFindsHandler(findSvc, settingsSvc.ChefEnabled),
		Sites:        handlers.NewSitesHandler(settingsSvc, findRepo),
		LibraryReview: handlers.NewLibraryReviewHandler(libreview.NewService(libreview.Deps{
			Store: reviewRepo, Tx: txRunner, Now: time.Now,
			Enabled: func(ctx context.Context) (bool, error) {
				on, err := settingsSvc.ChefEnabled(ctx)
				if err != nil || !on {
					return false, err
				}
				return settingsSvc.ChefLibraryReviewEnabled(ctx)
			},
		})),
		ChefPlan: handlers.NewChefPlanHandler(handlers.ChefPlanDeps{
			Finds: findSvc, ChefOn: settingsSvc.ChefEnabled, Plates: plateSvc,
			ShoppingDay: func(ctx context.Context) (int, error) {
				return plate.IntSetting(ctx, settingsSvc, settingsdom.KeyPlanShoppingDay)
			},
			LiveTips: func(ctx context.Context, start time.Time) (int, error) {
				v, err := tipSvc.Sync(ctx, start)
				return len(v.Tips), err
			},
			Suggested: func(ctx context.Context, start time.Time) (bool, error) {
				v, err := themeSvc.Window(ctx, start)
				return v.Suggestion != nil, err
			},
		}),
		NotForMe:      handlers.NewNotForMeHandler(foodSvc, tasteSvc),
		Presets:       handlers.NewPresetHandler(presetSvc),
		AI:            aiHandler,
		AIRuns:        aiRunsHandler,
		AIRateLimiter: aiRateLimiter,
		Feedback:      handlers.NewFeedbackHandler(feedbackSvc),
		Import:        importHandler,
		Discovery:     handlers.NewDiscoveryHandler(discoverySvc).WithClock(clock.Now),
		Chef:          handlers.NewChefHandler(chefGuard.Spent, labelling),
		Tips:          handlers.NewTipHandler(tipSvc, plateSvc, spendByKind).WithClock(clock.Now),
		Settings:      settingsHandler,
		Shopping:      handlers.NewShoppingHandler(shoppingSvc).WithClock(clock.Now),
		Availability:  handlers.NewAvailabilityHandler(availabilitySvc).WithClock(clock.Now),
		FoodEvents:    handlers.NewFoodEventHandler(foodEventSvc),
		Substitutions: handlers.NewSubstitutionHandler(substitutionSvc),
		Help:          handlers.NewHelpHandler(helpSvc),
		RecipeChanges: handlers.NewRecipeChangeHandler(recipeChangeSvc),
		Attributes:    handlers.NewAttributeHandler(flavorSvc),
		Setup:         handlers.NewSetupHandler(setup.NewService(settingsSvc, foodRepo)),
		DevMode:       cfg.DevMode,
		Dev:           devHandler,
		Version:       version,
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

// planningShutdownWait bounds how long shutdown waits for cancelled plan
// generations to record their end; a run still going after it is failed at
// the next start (FailInterruptedRuns).
const planningShutdownWait = 5 * time.Second

// chefJobConcurrency is how many capped jobs run at once; an interrupted
// job fails at boot after chefJobMaxAttempts tries; shutdown waits
// chefShutdownWait for running jobs.
const (
	chefJobConcurrency = 2
	chefJobMaxAttempts = 3
	chefShutdownWait   = 10 * time.Second
)

// discoveryShutdownWait bounds how long shutdown waits for a running
// discovery run before cancelling it.
const discoveryShutdownWait = 10 * time.Second

// loadRecipes loads the full recipes (ingredients and steps) of ids, each once.
func loadRecipes(ctx context.Context, repo *sqlite.FoodRepo, ids []int64) (map[int64]*food.Food, error) {
	out := make(map[int64]*food.Food, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok {
			continue
		}
		f, err := repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = f
	}
	return out, nil
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

// isFreshDB reports whether the database had no schema before migrating: a
// new install, as opposed to an upgrade.
func isFreshDB(conn *sql.DB) (bool, error) {
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return false, err
	}
	v, err := goose.GetDBVersion(conn)
	if err != nil {
		return false, err
	}
	return v == 0, nil
}

// seedFreshInstall starts a new install with the Chef on (ON-3). An upgrade
// keeps its setting; the spec default stays off so fresh test databases do
// not change.
func seedFreshInstall(ctx context.Context, svc *settingsdom.Service, fresh bool) error {
	if !fresh {
		return nil
	}
	return svc.Set(ctx, settingsdom.KeyChefEnabled, "true")
}

// appVersion is what runs record as app_version: the ldflags version and
// commit when set, else the VCS stamp of the build.
func appVersion() string {
	if version != "dev" || commit != "" {
		if commit == "" {
			return version
		}
		return version + "+" + commit
	}
	return airun.BuildVersion()
}

// lovedCuisines lists the cuisines the household loves (theme suggestions,
// THEME-4).
func lovedCuisines(view func(context.Context, time.Time) (taste.View, error)) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		v, err := view(ctx, time.Time{})
		if err != nil {
			return nil, err
		}
		var out []string
		for _, it := range v.Items {
			if it.Level == taste.LevelCuisine && it.Polarity == taste.Love {
				out = append(out, it.Key)
			}
		}
		return out, nil
	}
}
