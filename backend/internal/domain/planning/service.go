package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
const HeuristicVersion = "h1"

// runTimeout bounds one generation (D9).
const runTimeout = 3 * time.Minute

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
// ComponentID for side.
type Scope struct {
	Kind        ScopeKind
	Date        time.Time
	PlateID     int64
	ComponentID int64
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
	// IncludeNewRecipe nil follows plan.explore_per_week > 0 (D15).
	IncludeNewRecipe *bool
}

// Deps are the generation service's dependencies. Go runs a generation; nil
// runs it in a goroutine (tests pass a synchronous one).
type Deps struct {
	Builder    *Builder
	Plates     PlateService
	PlateRows  PlateRows
	Cells      Cells
	FoodRows   FoodRows
	Settings   SettingsReader
	RunLog     RunLog
	Runs       Runs
	Tx         domain.Transactor
	Progress   *Progress
	Choosers   map[airun.Chooser]Chooser
	Go         func(func())
	Logger     *slog.Logger
	RunTimeout time.Duration
}

// Service runs generations (spec §6.1): it validates a request and starts
// the run synchronously, then resolves, chooses, checks and persists in the
// background (D9). One code path for every chooser.
type Service struct {
	d  Deps
	wg sync.WaitGroup
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
	return &Service{d: d}
}

// Wait blocks until every started generation finished (shutdown).
func (s *Service) Wait() { s.wg.Wait() }

// Generate starts a window generation (D7: on a window with drafts it
// replaces them all) and returns its run id.
func (s *Service) Generate(ctx context.Context, req Request) (int64, error) {
	req.Scope = Scope{Kind: ScopeWindow}
	return s.start(ctx, airun.KindGenerate, req)
}

// Regenerate starts a day, plate, side or empty_cells generation and
// returns its run id. It keeps each day's eaters as the plates it replaces
// (or, for empty cells, the day's other plates) have them, and asks for no
// new recipe.
func (s *Service) Regenerate(ctx context.Context, req Request) (int64, error) {
	switch req.Scope.Kind {
	case ScopeDay, ScopePlate, ScopeSide, ScopeEmptyCells:
	default:
		return 0, fmt.Errorf("%w: kind %q", domain.ErrInvalidScope, req.Scope.Kind)
	}
	noNew := false
	req.EatersByDate, req.IncludeNewRecipe = nil, &noNew
	return s.start(ctx, airun.KindRegenerate, req)
}

// job is a started generation's input.
type job struct {
	kind        airun.Kind
	w           window.Window
	today       time.Time
	chooser     Chooser
	name        airun.Chooser
	scope       Scope
	target      Target
	rows        plate.RowSet
	side        *SideSwap
	avoid       []int64
	from, to    time.Time
	eaters      map[string]float64
	includeNew  bool
	instruction string
	kept        int
}

func (s *Service) start(ctx context.Context, kind airun.Kind, req Request) (int64, error) {
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
	includeNew, err := s.includeNew(ctx, req.IncludeNewRecipe)
	if err != nil {
		return 0, err
	}

	if err := s.d.Progress.reserve(w.Start); err != nil {
		return 0, err
	}
	started := false
	defer func() {
		if !started {
			s.d.Progress.release(w.Start)
		}
	}()
	j, err := s.resolve(ctx, w, dateOnly(req.Today), req.Scope)
	if err != nil {
		return 0, err
	}
	j.kind, j.chooser, j.name = kind, chooser, name
	if kind == airun.KindGenerate {
		j.eaters = req.EatersByDate
	}
	j.includeNew, j.instruction = includeNew, req.Instruction
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
	bg := context.WithoutCancel(runCtx)
	s.wg.Add(1)
	s.d.Go(func() {
		defer s.wg.Done()
		s.execute(bg, run, j)
	})
	return run.ID(), nil
}

// chooser resolves the requested chooser (D11): the LLM when one is wired,
// its provider resolves and nothing else is asked for, else the heuristic.
func (s *Service) chooser(ctx context.Context, name airun.Chooser) (airun.Chooser, Chooser, error) {
	if name == "" {
		name = airun.ChooserHeuristic
		if c, ok := s.d.Choosers[airun.ChooserLLM]; ok && ready(ctx, c) == nil {
			name = airun.ChooserLLM
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

func (s *Service) includeNew(ctx context.Context, asked *bool) (bool, error) {
	if asked != nil {
		return *asked, nil
	}
	n, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanExplorePerWeek)
	return n > 0, err
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
			date := dateOnly(sc.Date)
			if !w.Contains(date) || date.Before(today) {
				return fmt.Errorf("%w: day %s", domain.ErrInvalidScope, date.Format(time.DateOnly))
			}
			j.rows, closure, err = s.d.Plates.DraftClosure(ctx, date, date)
			cellsOf = func(c availability.Cell) bool { return c.Date.Equal(date) }
		case ScopePlate:
			p, err := s.scopePlate(ctx, w, today, sc.PlateID)
			if err != nil {
				return err
			}
			j.rows, closure, err = s.d.Plates.PlateClosure(ctx, p.ID)
			if err != nil {
				return err
			}
		case ScopeSide:
			return s.resolveSide(ctx, w, today, sc.ComponentID, &j)
		case ScopeEmptyCells:
			cellsOf = func(c availability.Cell) bool {
				_, taken := occupied[idOf(plate.CellKey{Date: c.Date, SlotID: c.SlotID})]
				return !taken && c.Mode != availability.ModeNone
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
				if c.IsComposed() && !slices.Contains(j.avoid, c.FoodID) {
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
	sw := &SideSwap{FoodID: v.FoodID, CookDate: v.CookDate, CookSlotID: v.CookSlotID, Portions: v.Portions, FrozenPortions: v.FrozenPortions}
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
	Kind             ScopeKind          `json:"kind"`
	Date             string             `json:"date,omitempty"`
	PlateID          int64              `json:"plate_id,omitempty"`
	ComponentID      int64              `json:"component_id,omitempty"`
	PlateIDs         []int64            `json:"plate_ids"`
	SessionIDs       []int64            `json:"session_ids"`
	KeptConfirmed    int                `json:"kept_confirmed"`
	EatersByDate     map[string]float64 `json:"eaters_by_date,omitempty"`
	IncludeNewRecipe bool               `json:"include_new_recipe,omitempty"`
}

func (j job) record() scopeRecord {
	r := scopeRecord{
		Kind: j.scope.Kind, PlateID: j.scope.PlateID, ComponentID: j.scope.ComponentID,
		PlateIDs: nonNil(j.rows.PlateIDs), SessionIDs: nonNil(j.rows.SessionIDs), KeptConfirmed: j.kept,
		EatersByDate: j.eaters, IncludeNewRecipe: j.includeNew,
	}
	if j.scope.Kind == ScopeDay {
		r.Date = j.scope.Date.Format(time.DateOnly)
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
// panic ends the run as an error instead of the process.
func (s *Service) execute(ctx context.Context, run Run, j job) {
	ctx, cancel := context.WithTimeout(ctx, s.d.RunTimeout)
	defer cancel()
	end := func() (end RunEnd) {
		defer func() {
			if r := recover(); r != nil {
				s.d.Logger.ErrorContext(ctx, "plan.generate panic", "run_id", run.ID(), "panic", r)
				end = RunEnd{Status: airun.StatusError, ErrorKey: "error.server"}
			}
		}()
		return s.generate(ctx, run.ID(), j)
	}()
	run.Finish(ctx, end)
	s.d.Progress.finish(run.ID())
}

// generate is the pipeline of spec §6.1 after the run started: resolve
// cells and apply routines, build the context, choose, check, persist
// atomically (D10) and score.
func (s *Service) generate(ctx context.Context, runID int64, j job) RunEnd {
	fail := func(stage string, err error) RunEnd {
		key, _ := domain.MessageKey(err, "plan")
		s.d.Logger.WarnContext(ctx, "plan.generate failed", "run_id", runID, "stage", stage, "error", err)
		return RunEnd{Status: airun.StatusError, ErrorKey: key}
	}
	var base State
	var routine []plate.SnapshotPlate
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		st, err := s.d.Builder.Build(ctx, j.w, j.today, j.from, j.to)
		if err != nil {
			return err
		}
		base = Merge(st, j.target, Proposal{})
		routine, err = s.d.Builder.ApplyRoutines(ctx, &base, j.target, func(d time.Time) float64 {
			if n := j.eaters[d.Format(time.DateOnly)]; n > 0 {
				return n
			}
			return base.Profile.HouseholdPortions
		})
		return err
	})
	if err != nil {
		return fail("resolving", err)
	}

	s.d.Progress.setStage(runID, StageChoosing)
	prop, err := j.chooser.Choose(ctx, ChooseInput{
		State: base, Target: j.target, EatersByDate: j.eaters, IncludeNewRecipe: j.includeNew,
		Avoid: j.avoid, Side: j.side, Instruction: j.instruction, Locale: base.Profile.Locale,
		Stage: func(st Stage) { s.d.Progress.setStage(runID, st) },
	})
	if err != nil {
		return fail("choosing", err)
	}

	s.d.Progress.setStage(runID, StageChecking)
	merged := Merge(base, j.target, prop)
	issues := Check(merged, j.target)

	s.d.Progress.setStage(runID, StageSaving)
	snap := ToSnapshot(prop, base)
	snap.Plates = append(routine, snap.Plates...)
	gen := actor.With(ctx, actor.Generator, runID)
	err = s.d.Tx.InTx(gen, func(ctx context.Context) error {
		if !j.rows.Empty() {
			if _, err := s.d.Plates.RemoveDrafts(ctx, j.rows); err != nil {
				return err
			}
		}
		_, err := s.d.Plates.Apply(ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
		return err
	})
	if err != nil {
		return fail("saving", err)
	}

	score := ComputeScore(merged, j.target, issues)
	violations := []Issue{}
	for _, is := range issues {
		if is.Severity == SeverityViolation {
			violations = append(violations, is)
		}
	}
	end := RunEnd{Status: airun.StatusOK, Notes: prop.Notes, RepairAttempts: prop.RepairAttempts}
	if len(violations) > 0 {
		end.Status = airun.StatusPartial
	}
	vj, err := json.Marshal(violations)
	if err != nil {
		return fail("scoring", err)
	}
	sj, err := json.Marshal(score)
	if err != nil {
		return fail("scoring", err)
	}
	end.ViolationsJSON, end.ScoreJSON = string(vj), string(sj)
	return end
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
}

// Counts are what a run wrote and the confirmed plates its scope kept.
type Counts struct {
	RunCounts
	KeptConfirmed int
}

// Run returns a generation run, or domain.ErrNotFound.
func (s *Service) Run(ctx context.Context, id int64) (*RunView, error) {
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
		v.Stage, _ = s.d.Progress.Stage(id)
	}
	if rec.ScopeJSON != "" {
		var sc scopeRecord
		if err := json.Unmarshal([]byte(rec.ScopeJSON), &sc); err != nil {
			return nil, fmt.Errorf("run %d scope: %w", id, err)
		}
		v.Counts.KeptConfirmed = sc.KeptConfirmed
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

// Running implements planview's Progress port.
func (s *Service) Running(start time.Time) (int64, bool) { return s.d.Progress.Running(start) }

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
