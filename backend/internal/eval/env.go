package eval

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// env is one scenario run: a fresh database with the app's services wired
// over it the way cmd/plantry/main.go does, and generations run
// synchronously.
type env struct {
	db       *sql.DB
	settings *settings.Service
	slots    *slot.Service
	foods    *food.Service
	presets  *preset.Service
	profile  *profile.Service
	avail    *availability.Service
	plates   *plate.Service
	plateRow *sqlite.PlateRepo
	sessions *sqlite.CookSessionRepo
	runRepo  *sqlite.AIRunRepo
	runs     *capturingLog
	svc      *planning.Service
	scripted *scriptedLLM

	slotIDs   map[string]int64
	foodIDs   map[string]int64
	presetIDs map[string]int64
	window    window.Window
}

func newEnv(conn *sql.DB, cfg Config) *env {
	settingsSvc := settings.NewService(sqlite.NewAppSettingsRepo(conn), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	foodRepo := sqlite.NewFoodRepo(conn)
	slotRepo := sqlite.NewSlotRepo(conn)
	tx := sqlite.NewTxRunner(conn)
	presetRepo := sqlite.NewPresetRepo(conn)
	avail := availability.NewService(sqlite.NewAvailabilityRepo(conn), tx, presetRepo)
	profileSvc := profile.NewService(sqlite.NewProfileRepo(conn))
	plateRepo := sqlite.NewPlateRepo(conn)
	sessionRepo := sqlite.NewCookSessionRepo(conn)
	runRepo := sqlite.NewAIRunRepo(conn)
	plateSvc := plate.NewService(plate.Deps{
		Plates: plateRepo, Sessions: sessionRepo, Slots: slotRepo, Foods: foodRepo, Cells: avail,
		Profile: profileSvc, Settings: settingsSvc, Events: sqlite.NewPlateEventRepo(conn), Outcomes: runRepo, Tx: tx,
	})
	avail.WithPlates(plateSvc)
	foodSvc := food.NewService(foodRepo)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	recorder := airun.NewRecorder(runRepo, logger, time.Now)
	e := &env{
		db: conn, settings: settingsSvc, slots: slot.NewService(slotRepo), foods: foodSvc,
		presets: preset.NewService(presetRepo, foodRepo, plateSvc, tx, foodRepo, foodRepo),
		profile: profileSvc, avail: avail, plates: plateSvc, plateRow: plateRepo, sessions: sessionRepo,
		runRepo: runRepo, runs: &capturingLog{inner: planning.RecorderLog{Recorder: recorder}, ends: map[int64]planning.RunEnd{}},
	}
	var llmChooser planning.Chooser
	if cfg.Live != nil {
		llmChooser = planning.NewLLMChooser(llm.StaticResolver(recorder.Meter(cfg.Live.Client, cfg.Live.Provider), cfg.model()))
	} else {
		e.scripted = newScriptedLLM(func(c llm.Client) llm.Client { return recorder.Meter(c, fakeProvider) }, cfg.model())
		llmChooser = e.scripted
	}
	e.svc = planning.NewService(planning.Deps{
		Builder: planning.NewBuilder(planning.BuilderDeps{
			Foods: foodSvc, FoodRows: foodRepo, Nutrition: food.NewNutritionResolver(foodRepo), Cells: avail,
			Plates: plateRepo, Sessions: sessionRepo, Presets: presetRepo, Slots: slotRepo,
			Settings: settingsSvc, Profile: profileSvc,
		}),
		Plates: plateSvc, PlateRows: plateRepo, Cells: avail, Overrides: avail, FoodRows: foodRepo, Settings: settingsSvc,
		RunLog: e.runs, Runs: runRepo, Tx: tx, Progress: planning.NewProgress(),
		Choosers: map[airun.Chooser]planning.Chooser{
			airun.ChooserHeuristic: planning.Heuristic{},
			airun.ChooserLLM:       llmChooser,
		},
		Go:     func(f func()) { f() },
		Logger: logger,
	})
	return e
}

// capturingLog is the run log that also keeps how each run ended, for the
// report (the run row has no read of repair attempts).
type capturingLog struct {
	inner planning.RunLog
	ends  map[int64]planning.RunEnd
}

func (l *capturingLog) Start(ctx context.Context, s planning.RunStart) (context.Context, planning.Run, error) {
	ctx, run, err := l.inner.Start(ctx, s)
	if err != nil {
		return nil, nil, err
	}
	return ctx, capturedRun{Run: run, log: l}, nil
}

type capturedRun struct {
	planning.Run
	log *capturingLog
}

func (r capturedRun) Finish(ctx context.Context, end planning.RunEnd) {
	r.log.ends[r.ID()] = end
	r.Run.Finish(ctx, end)
}

var slotIcons = map[string]string{"breakfast": "coffee", "lunch": "sun", "dinner": "moon"}

// seed writes a scenario's library, settings, profile, slots, presets,
// availability and plates through the services.
func (e *env) seed(ctx context.Context, sc Scenario, lib Library) error {
	for k, v := range sc.Settings {
		if err := e.settings.Set(ctx, k, v); err != nil {
			return fmt.Errorf("setting %s: %w", k, err)
		}
	}
	if err := e.seedProfile(ctx, sc.Profile); err != nil {
		return err
	}
	e.slotIDs = map[string]int64{}
	for i, name := range sc.Slots {
		icon := slotIcons[name]
		if icon == "" {
			icon = "utensils"
		}
		s := &slot.TimeSlot{NameKey: "slot." + name, Icon: icon, SortOrder: i, Active: true}
		if err := e.slots.Create(ctx, s); err != nil {
			return fmt.Errorf("slot %s: %w", name, err)
		}
		e.slotIDs[name] = s.ID
	}
	if err := e.seedLibrary(ctx, lib, sc.Recipes); err != nil {
		return err
	}
	e.presetIDs = map[string]int64{}
	for _, ps := range sc.Presets {
		sid, err := e.slot(ps.Slot)
		if err != nil {
			return err
		}
		fid, err := e.recipe(ps.Recipe)
		if err != nil {
			return err
		}
		one := 1.0
		p, err := e.presets.Create(ctx, preset.CreateInput{Name: ps.Name, Plates: []preset.Plate{{
			SlotID: sid, Components: []preset.Component{{FoodID: fid, Portions: &one}},
		}}})
		if err != nil {
			return fmt.Errorf("preset %s: %w", ps.Name, err)
		}
		e.presetIDs[ps.Name] = p.ID
	}
	if err := e.seedAvailability(ctx, sc.Availability); err != nil {
		return err
	}
	sd, err := plate.IntSetting(ctx, e.settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return err
	}
	if !window.IsStart(sc.Start.Time, sd) {
		return fmt.Errorf("start %s is not a shopping day", sc.Start.Format(time.DateOnly))
	}
	e.window = window.Containing(sc.Start.Time, sd)
	for _, ps := range sc.Plates {
		c, err := e.cell(ps.CellSpec)
		if err != nil {
			return err
		}
		fid, err := e.recipe(ps.Recipe)
		if err != nil {
			return err
		}
		one := 1.0
		if _, err := e.plates.Create(ctx, plate.PlateInput{
			Date: c.Date, SlotID: c.SlotID,
			Components: []plate.PlateComponent{{FoodID: fid, Portions: &one}},
		}); err != nil {
			return fmt.Errorf("plate %s %s: %w", ps.Date.Format(time.DateOnly), ps.Slot, err)
		}
	}
	return nil
}

func (e *env) seedProfile(ctx context.Context, ps ProfileSpec) error {
	p, err := e.profile.Get(ctx)
	if err != nil {
		return err
	}
	if ps.DietType != "" {
		p.DietType = profile.DietType(ps.DietType)
	}
	if ps.HouseholdPortions > 0 {
		p.HouseholdPortions = ps.HouseholdPortions
	}
	p.KcalTarget, p.ProteinPct, p.FatPct, p.CarbsPct = ps.KcalTarget, ps.ProteinPct, ps.FatPct, ps.CarbsPct
	p.MeatDaysPerWeekMax = ps.MeatDaysPerWeekMax
	if ps.Notes != "" {
		p.SystemPrompt = &ps.Notes
	}
	if _, err := e.profile.Update(ctx, p); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	return nil
}

func (e *env) seedLibrary(ctx context.Context, lib Library, only []string) error {
	e.foodIDs = map[string]int64{}
	leaves := map[string]int64{}
	for _, in := range lib.Ingredients {
		f := &food.Food{
			Kind: food.KindLeaf, Name: in.Name,
			Kcal100g: ptr(in.Kcal), Protein100g: ptr(in.Protein), Fat100g: ptr(in.Fat), Carbs100g: ptr(in.Carbs),
		}
		if err := e.foods.Create(ctx, f); err != nil {
			return fmt.Errorf("ingredient %s: %w", in.Name, err)
		}
		leaves[in.Name] = f.ID
	}
	want := map[string]bool{}
	for _, n := range only {
		want[n] = true
	}
	for _, r := range lib.Recipes {
		if len(want) > 0 && !want[r.Name] {
			continue
		}
		delete(want, r.Name)
		role := food.Role(r.Role)
		f := &food.Food{
			Kind: food.KindComposed, Name: r.Name, Role: &role, ReferencePortions: ptr(r.Portions),
			KeepDays: r.KeepDays, Freezable: r.Freezable, Favorite: r.Favorite, Tags: r.Tags,
		}
		for i, ing := range r.Ingredients {
			id, ok := leaves[ing.Name]
			if !ok {
				return fmt.Errorf("recipe %s: unknown ingredient %q", r.Name, ing.Name)
			}
			f.Children = append(f.Children, food.FoodComponent{ChildID: id, Amount: ing.Grams, Unit: "g", SortOrder: i})
		}
		if err := e.foods.Create(ctx, f); err != nil {
			return fmt.Errorf("recipe %s: %w", r.Name, err)
		}
		e.foodIDs[r.Name] = f.ID
	}
	for n := range want {
		return fmt.Errorf("unknown recipe %q", n)
	}
	return nil
}

func (e *env) seedAvailability(ctx context.Context, a AvailabilitySpec) error {
	var defaults []availability.Default
	for _, d := range a.Defaults {
		sid, err := e.slot(d.Slot)
		if err != nil {
			return err
		}
		var pid *int64
		if d.Preset != "" {
			id, ok := e.presetIDs[d.Preset]
			if !ok {
				return fmt.Errorf("unknown preset %q", d.Preset)
			}
			pid = &id
		}
		for _, wd := range d.Weekdays {
			defaults = append(defaults, availability.Default{Weekday: wd, SlotID: sid, Mode: availability.Mode(d.Mode), PresetID: pid})
		}
	}
	if len(defaults) > 0 {
		if err := e.avail.SetDefaults(ctx, defaults); err != nil {
			return fmt.Errorf("availability defaults: %w", err)
		}
	}
	for _, o := range a.Overrides {
		c, err := e.cell(CellSpec{Date: o.Date, Slot: o.Slot})
		if err != nil {
			return err
		}
		if _, _, err := e.avail.SetOverride(ctx, availability.OverrideInput{Date: c.Date, SlotID: c.SlotID, Mode: availability.Mode(o.Mode)}); err != nil {
			return fmt.Errorf("override %s %s: %w", o.Date.Format(time.DateOnly), o.Slot, err)
		}
	}
	return nil
}

func (e *env) slot(name string) (int64, error) {
	id, ok := e.slotIDs[name]
	if !ok {
		return 0, fmt.Errorf("unknown slot %q", name)
	}
	return id, nil
}

func (e *env) recipe(name string) (int64, error) {
	id, ok := e.foodIDs[name]
	if !ok {
		return 0, fmt.Errorf("unknown recipe %q", name)
	}
	return id, nil
}

func (e *env) cell(c CellSpec) (plate.CellKey, error) {
	id, err := e.slot(c.Slot)
	return plate.CellKey{Date: c.Date.Time, SlotID: id}, err
}

func ptr[T any](v T) *T { return &v }
