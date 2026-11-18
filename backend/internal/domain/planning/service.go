package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// HeuristicVersion is the run log's prompt_version of heuristic runs: bump
// it when the heuristic changes what it picks.
const HeuristicVersion = "h18"

// runTimeout bounds one generation (D9). Saving has its own budget, so a
// proposal ready late in the run is still written.
const (
	runTimeout  = 3 * time.Minute
	saveTimeout = 30 * time.Second
)

// keyTimeout is the error key of a run that ran out of time.
const keyTimeout = "error.plan.timeout"

// errShuttingDown refuses a generation once Shutdown began.
var errShuttingDown = errors.New("plan generation is shutting down")

// ScopeKind is what a generation may replace (D6).
type ScopeKind string

// Scopes. The window scope is generate (D7).
const (
	ScopeWindow     ScopeKind = "window"
	ScopeDay        ScopeKind = "day"
	ScopePlate      ScopeKind = "plate"
	ScopeSide       ScopeKind = "side"
	ScopeEmptyCells ScopeKind = "empty_cells"
)

// Scope names a regenerate's target: Date for day, PlateID for plate,
// ComponentID for side. From and To, when set, bound empty_cells to the
// cells dated in [From, To] (the mid-week re-plan, B8).
type Scope struct {
	Kind        ScopeKind
	Date        time.Time
	PlateID     int64
	ComponentID int64
	From, To    time.Time
}

// Request starts a generation of the window that starts on Start.
type Request struct {
	Start       time.Time
	Today       time.Time
	Scope       Scope
	Chooser     airun.Chooser
	Instruction string
	// EatersByDate (YYYY-MM-DD → eaters, dates in the window) is generate's
	// guests option (D16).
	EatersByDate map[string]float64
	// ExploreQuota is the number of explore recipes to include; nil follows
	// plan.explore_per_week (D17).
	ExploreQuota *int
	// Light are the run's light requests (see LightIntent); the heuristic
	// honours them and the LLM chooser is told them.
	Light []LightIntent
}

// Deps are the generation service's dependencies. Go runs a generation; nil
// runs it in a goroutine (tests pass a synchronous one).
type Deps struct {
	Builder   *Builder
	Plates    PlateService
	PlateRows PlateRows
	Cells     Cells
	Sessions  SessionRows
	FoodRows  FoodRows
	Settings  SettingsReader
	RunLog    RunLog
	Runs      Runs
	Overrides Overrides
	// Light stores the light requests a successful run was given.
	Light      LightStore
	Tx         domain.Transactor
	Progress   *Progress
	Choosers   map[airun.Chooser]Chooser
	Go         func(func())
	Logger     *slog.Logger
	RunTimeout time.Duration
}

// Service runs generations (spec §6.1): it validates a request and starts
// the run synchronously, then resolves, chooses, checks and persists in the
// background (D9). One code path for every chooser. Runs outlive their
// request but not the service: Shutdown cancels them.
type Service struct {
	d      Deps
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
	base   context.Context
	cancel context.CancelFunc
}

// NewService returns a generation service.
func NewService(d Deps) *Service {
	if d.Go == nil {
		d.Go = func(f func()) { go f() }
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.RunTimeout == 0 {
		d.RunTimeout = runTimeout
	}
	base, cancel := context.WithCancel(context.Background())
	return &Service{d: d, base: base, cancel: cancel}
}

// Wait blocks until every started generation finished.
func (s *Service) Wait() { s.wg.Wait() }

// Shutdown refuses new generations, cancels the running ones and waits until
// they finished their runs or ctx ends. A cancelled run ends as an error
// (error.server, like a run a restart interrupted).
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// track counts a generation about to start, unless Shutdown began.
func (s *Service) track() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

// Generate starts a window generation (D7: on a window with drafts it
// replaces them all) and returns its run id.
func (s *Service) Generate(ctx context.Context, req Request) (int64, error) {
	req.Scope = Scope{Kind: ScopeWindow}
	return s.start(ctx, airun.KindGenerate, req, false)
}

// Regenerate starts a day, plate, side or empty_cells generation and
// returns its run id. It keeps each day's eaters as the plates it replaces
// (or, for empty cells, the day's other plates) have them, and asks for no
// new recipe.
func (s *Service) Regenerate(ctx context.Context, req Request) (int64, error) {
	return s.regenerate(ctx, req, false)
}

// regenerate is Regenerate; held says the caller already reserved the
// window (the re-plan), and the reservation is released only when no run
// starts.
func (s *Service) regenerate(ctx context.Context, req Request, held bool) (int64, error) {
	switch req.Scope.Kind {
	case ScopeDay, ScopePlate, ScopeSide, ScopeEmptyCells:
	default:
		return 0, fmt.Errorf("%w: kind %q", domain.ErrInvalidScope, req.Scope.Kind)
	}
	none := 0
	req.EatersByDate, req.ExploreQuota = nil, &none
	return s.start(ctx, airun.KindRegenerate, req, held)
}

// job is a started generation's input.
type job struct {
	kind         airun.Kind
	w            window.Window
	today        time.Time
	chooser      Chooser
	name         airun.Chooser
	scope        Scope
	target       Target
	rows         plate.RowSet
	side         *SideSwap
	avoid        []int64
	avoidBatches []int64
	exclude      []int64
	from, to     time.Time
	eaters       map[string]float64
	exploreQuota int
	instruction  string
	// light are the request's light intents, then (after choosing) the
	// chooser's too.
	light []LightIntent
	kept  int
	// replacesBatch: a plate scope on a batch's cook plate replaces the batch.
	replacesBatch bool
}

func (s *Service) start(ctx context.Context, kind airun.Kind, req Request, held bool) (int64, error) {
	started := false
	if held {
		defer func() {
			if !started {
				s.d.Progress.release(dateOnly(req.Start))
			}
		}()
	}
	if !s.track() {
		return 0, errShuttingDown
	}
	defer func() {
		if !started {
			s.wg.Done()
		}
	}()
	sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return 0, err
	}
	start := dateOnly(req.Start)
	if !window.IsStart(start, sd) {
		return 0, fmt.Errorf("%w: %s", domain.ErrInvalidPlanStart, start.Format(time.DateOnly))
	}
	w := window.Containing(start, sd)
	if err := validateEaters(req.EatersByDate, w); err != nil {
		return 0, err
	}
	n, err := s.d.FoodRows.CountComposedFoods(ctx)
	if err != nil {
		return 0, fmt.Errorf("count recipes: %w", err)
	}
	if n < setup.RecipeTarget {
		return 0, fmt.Errorf("%w: %d recipes", domain.ErrNotEnoughRecipes, n)
	}
	name, chooser, err := s.chooser(ctx, req.Chooser)
	if err != nil {
		return 0, err
	}
	quota, err := s.exploreQuota(ctx, req.ExploreQuota)
	if err != nil {
		return 0, err
	}

	if !held {
		if err := s.d.Progress.reserve(w.Start); err != nil {
			return 0, err
		}
		defer func() {
			if !started {
				s.d.Progress.release(w.Start)
			}
		}()
	}
	j, err := s.resolve(ctx, w, dateOnly(req.Today), req.Scope)
	if err != nil {
		return 0, err
	}
	j.kind, j.chooser, j.name = kind, chooser, name
	if kind == airun.KindGenerate {
		j.eaters = req.EatersByDate
	}
	j.exploreQuota, j.instruction = quota, req.Instruction
	if j.light, err = ValidateLight(req.Light); err != nil {
		return 0, fmt.Errorf("%w: %w", domain.ErrInvalidInput, err)
	}
	scopeJSON, err := json.Marshal(j.record())
	if err != nil {
		return 0, err
	}
	version := HeuristicVersion
	if p, ok := chooser.(interface{ PromptVersion() string }); ok {
		version = p.PromptVersion()
	}
	runCtx, run, err := s.d.RunLog.Start(ctx, RunStart{
		Kind: kind, PromptVersion: version, WindowStart: w.Start, WindowEnd: w.End,
		Instruction: req.Instruction, Chooser: name, ScopeJSON: string(scopeJSON),
	})
	if err != nil {
		return 0, fmt.Errorf("start run: %w", err)
	}
	s.d.Progress.begin(w.Start, run.ID())
	started = true
	// The run keeps the request's values but not its cancellation (it
	// outlives the client), and ends when the service shuts down.
	bg, stop := context.WithCancel(context.WithoutCancel(runCtx))
	unhook := context.AfterFunc(s.base, stop)
	s.d.Go(func() {
		defer s.wg.Done()
		defer unhook()
		defer stop()
		s.execute(bg, run, j)
	})
	return run.ID(), nil
}

// chooser resolves the requested chooser (D11). Without a name it is the
// LLM when one is wired and its provider resolves, and the heuristic when no
// provider is configured. A configured provider that fails to resolve is an
// error, never a silent fallback: the user asks for the heuristic instead.
func (s *Service) chooser(ctx context.Context, name airun.Chooser) (airun.Chooser, Chooser, error) {
	if name == "" {
		name = airun.ChooserHeuristic
		if c, ok := s.d.Choosers[airun.ChooserLLM]; ok {
			switch err := ready(ctx, c); {
			case err == nil:
				name = airun.ChooserLLM
			case errors.Is(err, domain.ErrAIProviderMissing):
			default:
				s.d.Logger.WarnContext(ctx, "plan.chooser provider not ready", "error", err)
				return "", nil, err
			}
		}
	}
	switch name {
	case airun.ChooserLLM, airun.ChooserHeuristic:
	default:
		return "", nil, fmt.Errorf("%w: chooser %q", domain.ErrInvalidInput, name)
	}
	c, ok := s.d.Choosers[name]
	if !ok {
		return "", nil, fmt.Errorf("%w: no %s chooser", domain.ErrAIProviderMissing, name)
	}
	if err := ready(ctx, c); err != nil {
		return "", nil, err
	}
	return name, c, nil
}

// ready reports whether a chooser can run now: one with a Ready method (the
// LLM's provider must resolve) says so, any other always can.
func ready(ctx context.Context, c Chooser) error {
	if r, ok := c.(interface{ Ready(context.Context) error }); ok {
		return r.Ready(ctx)
	}
	return nil
}

// exploreQuota is the asked quota, else plan.explore_per_week (D17).
func (s *Service) exploreQuota(ctx context.Context, asked *int) (int, error) {
	if asked != nil {
		return *asked, nil
	}
	return plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanExplorePerWeek)
}

// WizardQuota maps the plan wizard's "include a new recipe" toggle to a
// quota: on is the setting, at least 1; off is 0 (D17).
func (s *Service) WizardQuota(ctx context.Context, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	n, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanExplorePerWeek)
	return max(n, 1), err
}

func validateEaters(m map[string]float64, w window.Window) error {
	for d, n := range m {
		date, err := time.Parse(time.DateOnly, d)
		if err != nil || !w.Contains(date) {
			return fmt.Errorf("%w: eaters_by_date %q is not a date of the window", domain.ErrInvalidInput, d)
		}
		if !(n > 0) || n > plate.MaxEaters {
			return fmt.Errorf("%w: eaters_by_date %q", domain.ErrInvalidInput, d)
		}
	}
	return nil
}

// resolve turns a scope into the job's target (D6): the cells it fills and
// the draft rows it replaces, expanded to their batch closures. Cells
// before today are never filled.
func (s *Service) resolve(ctx context.Context, w window.Window, today time.Time, sc Scope) (job, error) {
	j := job{w: w, today: today, scope: sc, from: w.Start, to: w.End}
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		cells, err := s.d.Cells.Resolve(ctx, w.Start, w.End)
		if err != nil {
			return err
		}
		plates, err := s.d.PlateRows.ListByDateRange(ctx, w.Start, w.End)
		if err != nil {
			return err
		}
		occupied := map[cellID]plate.Plate{}
		for _, p := range plates {
			occupied[idOf(p.Cell())] = p
		}
		var closure []plate.Plate
		var cellsOf func(availability.Cell) bool
		switch sc.Kind {
		case ScopeWindow:
			j.rows, closure, err = s.d.Plates.DraftClosure(ctx, maxDate(w.Start, today), w.End)
			cellsOf = func(availability.Cell) bool { return true }
		case ScopeDay:
			// The day stays inside the day: only its own drafts go, through
			// RemoveMeals at save, so other days keep their dishes (R1).
			date, derr := dayInWindow(w, today, sc.Date)
			if derr != nil {
				return derr
			}
			closure = dayDrafts(plates, date)
			for _, p := range closure {
				j.rows.PlateIDs = append(j.rows.PlateIDs, p.ID)
			}
			cellsOf = func(c availability.Cell) bool { return c.Date.Equal(date) }
		case ScopePlate:
			p, err := s.scopePlate(ctx, w, today, sc.PlateID)
			if err != nil {
				return err
			}
			leftover, lerr := s.eatsOnlyLeftovers(ctx, p)
			if lerr != nil {
				return lerr
			}
			// "Replace plate" means a different dish: the plate's own
			// foods may not come back on its cell, fresh or leftover.
			for _, c := range p.Components {
				if c.IsComposed() {
					j.exclude = append(j.exclude, c.FoodID)
				}
			}
			if leftover {
				// A leftover plate goes alone; the batch it ate from shrinks
				// by D11 when the plate is removed (D6, F6).
				j.rows, closure = plate.RowSet{PlateIDs: []int64{p.ID}}, []plate.Plate{*p}
				for _, c := range p.Components {
					if c.IsComposed() {
						j.avoidBatches = append(j.avoidBatches, c.FoodID)
					}
				}
			} else {
				j.rows, closure, err = s.d.Plates.PlateClosure(ctx, p.ID)
				if err != nil {
					return err
				}
				j.replacesBatch = len(closure) > 1
			}
		case ScopeSide:
			return s.resolveSide(ctx, w, today, sc.ComponentID, &j)
		case ScopeEmptyCells:
			cellsOf = func(c availability.Cell) bool {
				_, taken := occupied[idOf(plate.CellKey{Date: c.Date, SlotID: c.SlotID})]
				inRange := (sc.From.IsZero() || !c.Date.Before(dateOnly(sc.From))) && (sc.To.IsZero() || !c.Date.After(dateOnly(sc.To)))
				return !taken && inRange && c.Mode != availability.ModeNone
			}
		default:
			return fmt.Errorf("%w: kind %q", domain.ErrInvalidScope, sc.Kind)
		}
		if err != nil {
			return err
		}
		seen := map[cellID]bool{}
		add := func(c plate.CellKey) {
			if c.Date.Before(today) || seen[idOf(c)] {
				return
			}
			seen[idOf(c)] = true
			j.target.Cells = append(j.target.Cells, c)
			j.from, j.to = minDate(j.from, c.Date), maxDate(j.to, c.Date)
		}
		for _, c := range cells {
			if cellsOf != nil && cellsOf(c) {
				add(plate.CellKey{Date: c.Date, SlotID: c.SlotID})
			}
		}
		for _, p := range closure {
			add(p.Cell())
			for _, c := range p.Components {
				// D3: only a plate or day regenerate asks for something different; a
				// window run excludes nothing and the variety rules handle repeats.
				if (sc.Kind == ScopePlate || sc.Kind == ScopeDay) && c.IsComposed() && !slices.Contains(j.avoid, c.FoodID) {
					j.avoid = append(j.avoid, c.FoodID)
				}
			}
		}
		// A regenerate keeps each day's eaters (D16): the replaced plates'
		// or, for empty cells, the day's other plates'.
		if sc.Kind != ScopeWindow {
			eatersFrom := closure
			if sc.Kind == ScopeEmptyCells {
				eatersFrom = plates
			}
			j.eaters = dayEaters(eatersFrom, j.target.Cells)
		}
		j.target.PlateIDs, j.target.SessionIDs = j.rows.PlateIDs, j.rows.SessionIDs
		for _, c := range j.target.Cells {
			if p, ok := occupied[idOf(c)]; ok && p.State == plate.StateConfirmed {
				j.kept++
			}
		}
		return nil
	})
	return j, err
}

// errDryRun ends a unit of work that only looks: what it wrote is undone.
var errDryRun = errors.New("dry run")

// dryRun runs fn in a unit of work that is always rolled back. It must not
// run inside another unit of work: a nested InTx joins the outer one, which
// would keep fn's writes.
func (s *Service) dryRun(ctx context.Context, fn func(ctx context.Context) error) error {
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return errDryRun
	})
	if errors.Is(err, errDryRun) {
		return nil
	}
	return err
}

// removeDay removes a day scope's draft plates through RemoveMeals, so the
// batches they leave re-anchor (R1) and shrink (D11). A plate that is gone
// or no longer a draft means the plan changed since the scope resolved.
func (s *Service) removeDay(ctx context.Context, ids []int64) (plate.MealResult, error) {
	if len(ids) == 0 {
		return plate.MealResult{}, nil
	}
	for _, id := range ids {
		p, err := s.d.Plates.Get(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			return plate.MealResult{}, fmt.Errorf("%w: plate %d is gone", domain.ErrStateMismatch, id)
		}
		if err != nil {
			return plate.MealResult{}, err
		}
		if p.State != plate.StateDraft {
			return plate.MealResult{}, fmt.Errorf("%w: plate %d is confirmed", domain.ErrStateMismatch, id)
		}
	}
	return s.d.Plates.RemoveMeals(ctx, ids)
}

// movedNotes is a run note per batch the save moved, in plain words
// ("Lasagne now cooked Thursday lunch"), from the state the chooser saw.
func movedNotes(st State, moved []plate.AlsoChanged) string {
	lang := "en"
	if st.Profile.Locale == "de" {
		lang = "de"
	}
	notes := make([]string, 0, len(moved))
	for _, m := range moved {
		name := fmt.Sprintf("#%d", m.SessionID)
		if i := slices.IndexFunc(st.Sessions, func(ss Session) bool { return ss.ID == m.SessionID }); i >= 0 {
			if f, ok := st.Foods[st.Sessions[i].FoodID]; ok {
				name = f.Name
			}
		}
		d, err := time.Parse(time.DateOnly, m.Date)
		if err != nil {
			continue
		}
		notes = append(notes, fmt.Sprintf(texts[lang][textCookMoved], name, weekdays[lang][d.Weekday()], slotName(st, m.SlotID)))
	}
	return joinNotes(notes...)
}

// dayInWindow is a day scope's date: in w and not before today.
func dayInWindow(w window.Window, today, d time.Time) (time.Time, error) {
	date := dateOnly(d)
	if !w.Contains(date) || date.Before(today) {
		return time.Time{}, fmt.Errorf("%w: day %s", domain.ErrInvalidScope, date.Format(time.DateOnly))
	}
	return date, nil
}

// dayDrafts are the draft plates of plates dated date.
func dayDrafts(plates []plate.Plate, date time.Time) []plate.Plate {
	var out []plate.Plate
	for _, p := range plates {
		if p.Date.Equal(date) && p.State == plate.StateDraft {
			out = append(out, p)
		}
	}
	return out
}

// dayEaters maps each date of cells to the most eaters a plate of that date
// has; nil without such plates.
func dayEaters(plates []plate.Plate, cells []plate.CellKey) map[string]float64 {
	var out map[string]float64
	for _, p := range plates {
		if !slices.ContainsFunc(cells, func(c plate.CellKey) bool { return c.Date.Equal(p.Date) }) {
			continue
		}
		if out == nil {
			out = map[string]float64{}
		}
		d := p.DateString()
		out[d] = max(out[d], p.Eaters)
	}
	return out
}

// scopePlate loads a plate a scope names: it must be a draft dated in the
// window from today on.
func (s *Service) scopePlate(ctx context.Context, w window.Window, today time.Time, id int64) (*plate.Plate, error) {
	p, err := s.d.Plates.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.State == plate.StateConfirmed {
		return nil, fmt.Errorf("%w: plate %d", domain.ErrScopeConfirmed, id)
	}
	if !w.Contains(p.Date) || p.Date.Before(today) {
		return nil, fmt.Errorf("%w: plate %d is outside the window or past", domain.ErrInvalidScope, id)
	}
	return p, nil
}

// eatsOnlyLeftovers reports whether p eats only batches cooked elsewhere:
// it has composed components and none of their sessions cooks at p's cell.
func (s *Service) eatsOnlyLeftovers(ctx context.Context, p *plate.Plate) (bool, error) {
	any := false
	for _, c := range p.Components {
		if c.CookSessionID == nil {
			continue
		}
		v, err := s.d.Plates.Session(ctx, *c.CookSessionID)
		if err != nil {
			return false, err
		}
		if v.CookCell().Equal(p.Cell()) {
			return false, nil
		}
		any = true
	}
	return any, nil
}

// resolveSide targets a side component's session and every plate eating it;
// the plates stay (D6 "different side").
func (s *Service) resolveSide(ctx context.Context, w window.Window, today time.Time, componentID int64, j *job) error {
	row, err := s.d.Plates.GetComponent(ctx, componentID)
	if err != nil {
		return err
	}
	p, err := s.scopePlate(ctx, w, today, row.PlateID)
	if err != nil {
		return err
	}
	// The plate's components carry the food's role.
	i := slices.IndexFunc(p.Components, func(c plate.PlateComponent) bool { return c.ID == componentID })
	if i < 0 {
		return fmt.Errorf("%w: component %d", domain.ErrNotFound, componentID)
	}
	pc := p.Components[i]
	if pc.CookSessionID == nil || pc.FoodRole == nil || !sideRole(food.Role(*pc.FoodRole)) {
		return fmt.Errorf("%w: component %d is not a side", domain.ErrInvalidScope, componentID)
	}
	v, err := s.d.Plates.Session(ctx, *pc.CookSessionID)
	if err != nil {
		return err
	}
	sw := &SideSwap{FoodID: v.FoodID, Role: food.Role(*pc.FoodRole), CookDate: v.CookDate, CookSlotID: v.CookSlotID, Portions: v.Portions, FrozenPortions: v.FrozenPortions}
	// The new side cooks on the old one's cell, so that cell is in the
	// target (H6) even when no plate eats there.
	cells := []plate.CellKey{v.CookCell()}
	for _, l := range v.Links {
		sw.Plates = append(sw.Plates, SidePlate{PlateID: l.PlateID, Date: l.Cell.Date, SlotID: l.Cell.SlotID, Eaters: l.Eaters, PerEater: l.PerEater})
		if !l.Cell.Equal(v.CookCell()) {
			cells = append(cells, l.Cell)
		}
	}
	for _, c := range cells {
		j.target.Cells = append(j.target.Cells, c)
		j.from, j.to = minDate(j.from, c.Date), maxDate(j.to, c.Date)
	}
	j.side = sw
	j.rows = plate.RowSet{SessionIDs: []int64{v.ID}}
	j.target.SessionIDs = j.rows.SessionIDs
	j.avoid = []int64{v.FoodID}
	return nil
}

// scopeRecord is a run's scope_json: the request's scope, its closure
// expansion and the confirmed plates on its cells (D6).
type scopeRecord struct {
	Kind          ScopeKind          `json:"kind"`
	Date          string             `json:"date,omitempty"`
	PlateID       int64              `json:"plate_id,omitempty"`
	ComponentID   int64              `json:"component_id,omitempty"`
	From          string             `json:"from,omitempty"`
	To            string             `json:"to,omitempty"`
	PlateIDs      []int64            `json:"plate_ids"`
	SessionIDs    []int64            `json:"session_ids"`
	KeptConfirmed int                `json:"kept_confirmed"`
	EatersByDate  map[string]float64 `json:"eaters_by_date,omitempty"`
	ExploreQuota  int                `json:"explore_quota,omitempty"`
	// Light are the light requests the run was started with.
	Light []LightIntent `json:"light,omitempty"`
	// AlsoChanged are the batches outside a day scope its removal moved
	// (R1), and Undo the save's revert token; both are set when the run
	// saved.
	AlsoChanged []plate.AlsoChanged `json:"also_changed,omitempty"`
	Undo        *plate.Undo         `json:"undo,omitempty"`
}

func (j job) record() scopeRecord {
	r := scopeRecord{
		Kind: j.scope.Kind, PlateID: j.scope.PlateID, ComponentID: j.scope.ComponentID,
		PlateIDs: nonNil(j.rows.PlateIDs), SessionIDs: nonNil(j.rows.SessionIDs), KeptConfirmed: j.kept,
		EatersByDate: j.eaters, ExploreQuota: j.exploreQuota, Light: j.light,
	}
	if j.scope.Kind == ScopeDay {
		r.Date = j.scope.Date.Format(time.DateOnly)
	}
	if !j.scope.From.IsZero() {
		r.From = j.scope.From.Format(time.DateOnly)
	}
	if !j.scope.To.IsZero() {
		r.To = j.scope.To.Format(time.DateOnly)
	}
	return r
}

func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// execute runs a started generation to its end and finishes its run. A
// panic ends the run as an error instead of the process. The window stays
// reserved until the run row is finished, and is freed even if that fails.
func (s *Service) execute(ctx context.Context, run Run, j job) {
	defer s.d.Progress.finish(run.ID())
	tctx, cancel := context.WithTimeout(ctx, s.d.RunTimeout)
	defer cancel()
	end := func() (end RunEnd) {
		defer func() {
			if r := recover(); r != nil {
				s.d.Logger.ErrorContext(ctx, "plan.generate panic", "run_id", run.ID(), "panic", r, "stack", string(debug.Stack()))
				end = RunEnd{Status: airun.StatusError, ErrorKey: "error.server"}
			}
		}()
		return s.generate(tctx, run.ID(), j)
	}()
	if end.Status == airun.StatusError && s.base.Err() != nil {
		end.ErrorKey = "error.server" // interrupted by Shutdown
	}
	run.Finish(ctx, end)
}

// generate is the pipeline of spec §6.1 after the run started: resolve
// cells and apply routines, build the context, choose, check, persist
// atomically (D10) and score.
func (s *Service) generate(ctx context.Context, runID int64, j job) RunEnd {
	// Every run that got as far as the base plan ends with one closing
	// plan.evaluation: `after` once saved, else the last proposal (or the
	// unchanged plan) as `final_proposal` with the run's status, also when
	// the run panics.
	var base State
	var haveBase, closed bool
	var last Proposal // the chooser's last proposal, partial on its error
	closeStatus, closeStage := "panic", ""
	defer func() {
		if haveBase && !closed {
			RecordFinalProposal(ctx, closeStatus, closeStage, func() Evaluation {
				return Evaluate(Merge(base, j.target, last), j.target)
			})
		}
	}()
	fail := func(stage string, err error) RunEnd {
		key, _ := domain.MessageKey(err, "plan")
		closeStatus, closeStage = "error", stage
		if errors.Is(err, context.DeadlineExceeded) {
			key, closeStatus = keyTimeout, "timeout"
		}
		s.d.Logger.WarnContext(ctx, "plan.generate failed", "run_id", runID, "stage", stage, "error", err)
		return RunEnd{Status: airun.StatusError, ErrorKey: key}
	}
	started := time.Now()
	stage := func(st Stage, extra ...any) {
		s.d.Progress.setStage(runID, st)
		s.logStage(ctx, runID, started, st, extra...)
	}
	s.logStage(ctx, runID, started, StageResolving)
	var routine []plate.SnapshotPlate
	var stored []LightIntent
	build := func(ctx context.Context) error {
		st, err := s.d.Builder.Build(ctx, j.w, j.today, j.from, j.to)
		if err != nil {
			return err
		}
		stored = st.Light
		st.Light = MergeLight(stored, j.light)
		base = Merge(st, j.target, Proposal{})
		routine, err = s.d.Builder.ApplyRoutines(ctx, &base, j.target, func(d time.Time) float64 {
			if n := j.eaters[d.Format(time.DateOnly)]; n > 0 {
				return n
			}
			return base.Profile.HouseholdPortions
		})
		return err
	}
	var err error
	var dryRemoval plate.MealResult
	if j.scope.Kind == ScopeDay {
		// The chooser plans on the plan as it will be once the day's drafts
		// are gone: batches re-anchored, shrunk or deleted. The removal is
		// rolled back; the save repeats it, so a failed run changes nothing.
		err = s.dryRun(ctx, func(ctx context.Context) error {
			res, err := s.removeDay(ctx, j.rows.PlateIDs)
			if err != nil {
				return changedDuringRun(err)
			}
			dryRemoval = res
			return build(ctx)
		})
	} else {
		err = s.d.Tx.InTx(ctx, build)
	}
	if err != nil {
		return fail("resolving", err)
	}
	haveBase = true

	// Without an empty plan cell (a side swap aside) there is nothing to
	// choose, so no chooser runs: an LLM call would plan nothing at a cost.
	if j.side != nil || hasOpenCell(base, j.target) {
		stage(StageChoosing)
		last, err = j.chooser.Choose(ctx, ChooseInput{
			State: base, Target: j.target, EatersByDate: j.eaters, ExploreQuota: j.exploreQuota,
			Avoid: j.avoid, AvoidBatches: j.avoidBatches, Exclude: j.exclude, Side: j.side, Instruction: j.instruction, Locale: base.Profile.Locale,
			Stage: func(st Stage) { stage(st) }, Logger: s.d.Logger,
		})
		if err != nil {
			return fail("choosing", err)
		}
	}

	// No saved plate holds a dish above the portion cap, whatever the chooser
	// left (the checker flagged it and the repairs had their turns).
	prop, capNotes := capPortions(base, last)
	if len(capNotes) > 0 {
		prop.Notes = joinNotes(append([]string{prop.Notes}, capNotes...)...)
	}
	// The week's meat stays within its budget and holds a fish meal whenever a
	// verified swap exists, whatever the chooser left (runs that span the week only).
	if j.side == nil && distinctDays(j.target) >= minDGEDays {
		var weekNotes []string
		prop, weekNotes = guardWeek(base, j.target, prop, idSet(j.exclude), j.instruction)
		if len(weekNotes) > 0 {
			prop.Notes = joinNotes(append([]string{prop.Notes}, weekNotes...)...)
		}
	}
	// A model's plan gets its quantities finished in Go (portions, a side,
	// add-ons), the way the heuristic sets its own.
	if _, heuristic := j.chooser.(Heuristic); !heuristic && j.side == nil {
		var balanceNotes []string
		prop, balanceNotes = balancePlan(ChooseInput{State: base, Target: j.target, Exclude: j.exclude, Avoid: j.avoid, Locale: base.Profile.Locale}, prop)
		if len(balanceNotes) > 0 {
			prop.Notes = joinNotes(append([]string{prop.Notes}, balanceNotes...)...)
		}
	}
	merged := Merge(base, j.target, prop)
	issues := Check(merged, j.target)
	stage(StageChecking, issueAttrs(issues)...)

	snap := ToSnapshot(prop, base)
	// proposal_sessions are the chooser's; the run API's counts.sessions are the
	// persisted ones, routine plates' sessions included.
	stage(StageSaving, "plates", len(snap.Plates)+len(routine), "routine_plates", len(routine), "proposal_sessions", len(snap.Sessions))
	snap.Plates = append(routine, snap.Plates...)
	// Saving has its own budget: a proposal ready at the run's deadline is
	// still written.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer cancelSave()
	gen := actor.With(saveCtx, actor.Generator, runID)
	var moved []plate.AlsoChanged
	undo, err := s.d.Plates.Recorded(gen, func(ctx context.Context) error {
		switch {
		case j.scope.Kind == ScopeDay:
			res, err := s.removeDay(ctx, j.rows.PlateIDs)
			if err != nil {
				return err
			}
			// The chooser planned on the dry run's plan. A removal that
			// shrinks other batches or moves other cooks differently means a
			// plate of an affected batch changed since.
			if !reflect.DeepEqual(res.Removed, dryRemoval.Removed) || !reflect.DeepEqual(res.AlsoChanged, dryRemoval.AlsoChanged) {
				return fmt.Errorf("%w: the day's batches changed", domain.ErrPlanChanged)
			}
			moved = res.AlsoChanged
		case !j.rows.Empty():
			if _, err := s.d.Plates.RemoveDrafts(ctx, j.rows); err != nil {
				return err
			}
		}
		if _, err := s.d.Plates.Apply(ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail}); err != nil {
			return err
		}
		// The light requests the plan now stands under are kept for the
		// board, the chat and later runs.
		return s.d.Light.Replace(ctx, j.today, LightDates(UnionLight(stored, merged.Light)), merged.Light)
	})
	if err != nil {
		return fail("saving", changedDuringRun(err))
	}

	// The closing evaluation reads the saved plan back, as the board will.
	closed = true
	s.recordAfter(ctx, j)
	score := ComputeScore(merged, j.target, issues, j.exploreQuota)
	violations := violationsOf(issues)
	end := RunEnd{Status: airun.StatusOK, Notes: joinNotes(prop.Notes, movedNotes(base, moved)), RepairAttempts: prop.RepairAttempts}
	if len(violations) > 0 {
		end.Status = airun.StatusPartial
	}
	// The run's scope now also says what moved outside it and how to take
	// the save back (day and plate scopes).
	rec := j.record()
	rec.AlsoChanged = moved
	if (j.scope.Kind == ScopeDay || j.scope.Kind == ScopePlate) && undo.FirstEventID > 0 {
		rec.Undo = &undo
	}
	rj, err := json.Marshal(rec)
	if err != nil {
		return fail("scoring", err)
	}
	end.ScopeJSON = string(rj)
	vj, err := json.Marshal(violations)
	if err != nil {
		return fail("scoring", err)
	}
	sj, err := json.Marshal(score)
	if err != nil {
		return fail("scoring", err)
	}
	end.ViolationsJSON, end.ScoreJSON = string(vj), string(sj)
	s.d.Logger.InfoContext(ctx, "plan.run.stage", append(runAttrs(ctx, runID),
		"stage", "done", "elapsed_ms", time.Since(started).Milliseconds(),
		"status", end.Status, "repair_attempts", end.RepairAttempts,
		"violations", len(violations), "score", end.ScoreJSON)...)
	return end
}

// recordAfter stores the `after` evaluation of the plan as saved: the window
// built again from the database, not the merged proposal.
func (s *Service) recordAfter(ctx context.Context, j job) {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer cancel()
	st, err := s.d.Builder.Build(dctx, j.w, j.today, j.from, j.to)
	if err != nil {
		RecordEvaluationError(ctx, PhaseAfter, err)
		return
	}
	RecordEvaluation(ctx, PhaseAfter, func() Evaluation { return Evaluate(st, j.target) })
}

// changedDuringRun marks a save that failed because the plan changed while
// the run chose (D10): a replaced draft was confirmed or removed, a target
// cell got a plate or went No plate, or a session the proposal eats from is
// gone. The unit of work rolled back, so the user can simply run it again.
func changedDuringRun(err error) error {
	for _, e := range []error{domain.ErrStateMismatch, domain.ErrCellOccupied, domain.ErrCellUnavailable, domain.ErrSessionNotFound} {
		if errors.Is(err, e) {
			return fmt.Errorf("%w: %w", domain.ErrPlanChanged, err)
		}
	}
	return err
}

// hasOpenCell reports whether a target cell is a plan cell without a plate
// in st: what a chooser fills.
func hasOpenCell(st State, t Target) bool {
	x := newIndex(st, t)
	for _, c := range st.Cells {
		if c.Mode == availability.ModePlan && x.inTarget(c.Key()) && len(x.byCell[idOf(c.Key())]) == 0 {
			return true
		}
	}
	return false
}

// RunView is a generation run as GET /api/plan/runs/{id} shows it: the run
// row, merged with the registry's stage while it runs (D9).
type RunView struct {
	ID       int64
	Kind     airun.Kind
	Status   airun.Status
	Stage    Stage
	Chooser  airun.Chooser
	Notes    string
	ErrorKey string
	Counts   Counts
	Issues   []Issue
	Score    *Score
	Window   window.Window
	// Undo reverts the save of a finished (ok or partial) day or plate
	// regenerate; nil otherwise.
	Undo *plate.Undo
}

// Counts are what a run wrote and the confirmed plates its scope kept.
type Counts struct {
	RunCounts
	KeptConfirmed int
}

// Run returns a generation run, or domain.ErrNotFound.
func (s *Service) Run(ctx context.Context, id int64) (*RunView, error) {
	// The registry first: a run writes its row before it leaves the registry,
	// so a row read afterwards that still says running with the run absent
	// here has lost its finish write, never one that is finishing.
	st, registered := s.d.Progress.Stage(id)
	rec, err := s.d.Runs.PlanRun(ctx, id)
	if err != nil {
		return nil, err
	}
	counts, err := s.d.Runs.PlanRunCounts(ctx, id)
	if err != nil {
		return nil, err
	}
	v := &RunView{
		ID: rec.ID, Kind: rec.Kind, Status: rec.Status, Chooser: rec.Chooser, Notes: rec.Notes, ErrorKey: rec.ErrorKey,
		Counts: Counts{RunCounts: counts}, Issues: []Issue{}, Window: window.Window{Start: rec.WindowStart, End: rec.WindowEnd},
	}
	if v.Status == airun.StatusRunning {
		if !registered {
			// The registry holds every run this process runs: a running row
			// it lacks lost its finish write (or a restart interrupted it).
			v.Status, v.ErrorKey = airun.StatusError, "error.server"
		}
		v.Stage = st
	}
	if rec.ScopeJSON != "" {
		var sc scopeRecord
		if err := json.Unmarshal([]byte(rec.ScopeJSON), &sc); err != nil {
			return nil, fmt.Errorf("run %d scope: %w", id, err)
		}
		v.Counts.KeptConfirmed = sc.KeptConfirmed
		if v.Status == airun.StatusOK || v.Status == airun.StatusPartial {
			v.Undo = sc.Undo
		}
	}
	if rec.ViolationsJSON != "" {
		if err := json.Unmarshal([]byte(rec.ViolationsJSON), &v.Issues); err != nil {
			return nil, fmt.Errorf("run %d violations: %w", id, err)
		}
	}
	if rec.ScoreJSON != "" {
		v.Score = &Score{}
		if err := json.Unmarshal([]byte(rec.ScoreJSON), v.Score); err != nil {
			return nil, fmt.Errorf("run %d score: %w", id, err)
		}
	}
	return v, nil
}

// Await blocks until run id finished, then returns it as Run does. It
// returns ctx's error when ctx ends first; the run goes on (D8).
func (s *Service) Await(ctx context.Context, id int64) (*RunView, error) {
	if done, ok := s.d.Progress.doneCh(id); ok {
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Run(ctx, id)
}

// Running implements planview's Progress port.
func (s *Service) Running(start time.Time) (int64, bool) { return s.d.Progress.Running(start) }

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
