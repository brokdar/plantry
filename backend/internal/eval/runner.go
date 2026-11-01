package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// DefaultRuns is how often each scenario runs (spec §8.4).
const DefaultRuns = 3

// Config is an eval run: which chooser plans the measured step, how often
// each scenario runs and the labels the report is keyed by.
type Config struct {
	Chooser airun.Chooser
	Runs    int
	// Model is the LLM's model; the label of scripted runs ("fake" when
	// empty).
	Model string
	// PromptVersion labels the report; empty takes the chooser's version.
	PromptVersion string
	GitSHA        string
	// Live plans with a real provider (PLANTRY_EVAL_LIVE=1); nil scripts the
	// fake client per choice.
	Live *Live
	// Now times the measured step; nil is time.Now.
	Now    func() time.Time
	Logger *slog.Logger
}

// Live is a real LLM provider's client.
type Live struct {
	Provider string
	Client   llm.Client
}

func (c Config) model() string {
	if c.Chooser == airun.ChooserHeuristic {
		return ""
	}
	if c.Model == "" && c.Live == nil {
		return fakeProvider
	}
	return c.Model
}

func (c Config) promptVersion() string {
	switch {
	case c.PromptVersion != "":
		return c.PromptVersion
	case c.Chooser == airun.ChooserLLM:
		return planning.PromptVersion
	}
	return planning.HeuristicVersion
}

// Expectation names.
const (
	ExpectNoViolations    = "no_hard_violations"
	ExpectPromptDates     = "prompt_dates"
	ExpectKeptConfirmed   = "kept_confirmed"
	ExpectNoPlate         = "no_plate"
	ExpectUnchangedBefore = "unchanged_before"
	ExpectMinPlates       = "min_plates"
	ExpectMinRepeats      = "min_repeats"
	ExpectFrozenSpare     = "frozen_spare"
)

// Run runs every scenario cfg.Runs times, each on a fresh database, and
// returns one record per run. An error means the eval itself broke (a
// scenario that does not seed); a failing generation is a record.
func Run(ctx context.Context, cfg Config, scenarios []Scenario) ([]Record, error) {
	switch cfg.Chooser {
	case airun.ChooserHeuristic, airun.ChooserLLM:
	default:
		return nil, fmt.Errorf("unknown chooser %q", cfg.Chooser)
	}
	if cfg.Runs <= 0 {
		cfg.Runs = DefaultRuns
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	lib, err := LoadLibrary()
	if err != nil {
		return nil, err
	}
	d, err := newDBs()
	if err != nil {
		return nil, err
	}
	defer func() { _ = d.close() }()
	var out []Record
	for _, sc := range scenarios {
		for n := 1; n <= cfg.Runs; n++ {
			rec, err := runOnce(ctx, cfg, d, lib, sc, n)
			if err != nil {
				return nil, fmt.Errorf("%s run %d: %w", sc.Name, n, err)
			}
			out = append(out, rec)
		}
	}
	return out, nil
}

func runOnce(ctx context.Context, cfg Config, d *dbs, lib Library, sc Scenario, n int) (Record, error) {
	conn, err := d.open()
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = conn.Close() }()
	e := newEnv(conn, cfg)
	if err := e.seed(ctx, sc, lib); err != nil {
		return Record{}, fmt.Errorf("seed: %w", err)
	}
	rec := Record{
		GitSHA: cfg.GitSHA, PromptVersion: cfg.promptVersion(), Model: cfg.model(), Chooser: string(cfg.Chooser),
		Scenario: sc.Name, Run: n, Violations: []planning.Issue{}, Expectations: []Expectation{},
	}
	measured := -1
	for i, st := range sc.Steps {
		if st.startsRun() {
			measured = i
		}
	}
	var before rows
	for i, st := range sc.Steps {
		if i != measured {
			// Setup steps plan with the heuristic: the scenario's state is
			// the same for every chooser.
			if _, err := e.step(ctx, sc, st, airun.ChooserHeuristic); err != nil {
				return Record{}, fmt.Errorf("step %d (%s): %w", i+1, st.Action, err)
			}
			continue
		}
		if before, err = e.rows(ctx); err != nil {
			return Record{}, err
		}
		e.takeRequests()
		t0 := cfg.Now()
		runID, err := e.step(ctx, sc, st, cfg.Chooser)
		rec.LatencyMS = cfg.Now().Sub(t0).Milliseconds()
		if err != nil {
			rec.Status = string(airun.StatusError)
			rec.ErrorKey, _ = domain.MessageKey(err, "plan")
			rec.Expectations = append(rec.Expectations, Expectation{Name: ExpectNoViolations, Detail: err.Error()})
			return rec, nil
		}
		if err := e.measure(ctx, runID, &rec); err != nil {
			return Record{}, err
		}
		// A run with nothing to choose sends no request, and has no prompt
		// to check.
		if reqs := e.takeRequests(); len(reqs) > 0 && cfg.Chooser == airun.ChooserLLM {
			rec.Expectations = append(rec.Expectations, promptDates(reqs, stepToday(sc, st), e))
		}
	}
	after, err := e.rows(ctx)
	if err != nil {
		return Record{}, err
	}
	rec.Expectations = append(rec.Expectations, e.expect(sc, sc.Steps[measured], before, after, rec)...)
	return rec, nil
}

func stepToday(sc Scenario, st Step) time.Time {
	if st.Today != nil {
		return st.Today.Time
	}
	return sc.Today.Time
}

// step runs one scenario step and returns the run it started (0 for none).
func (e *env) step(ctx context.Context, sc Scenario, st Step, chooser airun.Chooser) (int64, error) {
	today := stepToday(sc, st)
	req := planning.Request{Start: e.window.Start, Today: today, Chooser: chooser, Instruction: st.Instruction}
	switch st.Action {
	case ActionGenerate:
		return e.svc.Generate(ctx, req)
	case ActionRegenerate:
		if st.Date == nil {
			return 0, errors.New("regenerate_day needs a date")
		}
		req.Scope = planning.Scope{Kind: planning.ScopeDay, Date: st.Date.Time}
		return e.svc.Regenerate(ctx, req)
	case ActionAccept:
		_, err := e.plates.Accept(ctx, e.window.Start, e.window.End)
		return 0, err
	case ActionReplan:
		if st.From == nil || st.To == nil {
			return 0, errors.New("replan needs from and to")
		}
		q := planning.ReplanQuery{From: st.From.Time, To: st.To.Time, Today: today}
		for _, c := range st.None {
			k, err := e.cell(c)
			if err != nil {
				return 0, err
			}
			q.NoneCells = append(q.NoneCells, k)
		}
		res, err := e.svc.Replan(ctx, planning.ReplanRequest{
			ReplanQuery: q, UnlockConfirmed: st.UnlockConfirmed, Spare: planning.SpareMode(st.Spare),
			Regenerate: st.Regenerate, Chooser: chooser, Instruction: st.Instruction,
		})
		if err != nil {
			return 0, err
		}
		if st.Regenerate && res.RunID == nil {
			return 0, errors.New("replan started no run")
		}
		if res.RunID == nil {
			return 0, nil
		}
		return *res.RunID, nil
	}
	return 0, fmt.Errorf("unknown action %q", st.Action)
}

// measure fills a record from the measured run: how it ended, its score,
// violations, plates, tokens and cost.
func (e *env) measure(ctx context.Context, runID int64, rec *Record) error {
	end, ok := e.runs.ends[runID]
	if !ok {
		return fmt.Errorf("run %d did not finish", runID)
	}
	rec.Status, rec.ErrorKey, rec.RepairAttempts = string(end.Status), end.ErrorKey, end.RepairAttempts
	if end.ScoreJSON != "" {
		rec.Score = &planning.Score{}
		if err := json.Unmarshal([]byte(end.ScoreJSON), rec.Score); err != nil {
			return fmt.Errorf("run %d score: %w", runID, err)
		}
	}
	if end.ViolationsJSON != "" {
		if err := json.Unmarshal([]byte(end.ViolationsJSON), &rec.Violations); err != nil {
			return fmt.Errorf("run %d violations: %w", runID, err)
		}
	}
	counts, err := e.runRepo.PlanRunCounts(ctx, runID)
	if err != nil {
		return err
	}
	rec.Plates = counts.Plates
	row, err := e.runRepo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	rec.Tokens = Tokens{Input: row.Tokens.Input, Output: row.Tokens.Output, CacheRead: row.Tokens.CacheRead, CacheWrite: row.Tokens.CacheWrite}
	rec.CostUSD = airun.Cost(row.Model, row.Provider, row.Tokens, nil)
	ok = end.Status == airun.StatusOK && len(rec.Violations) == 0
	detail := ""
	if !ok {
		detail = fmt.Sprintf("status %s %s, %d violations", end.Status, end.ErrorKey, len(rec.Violations))
	}
	rec.Expectations = append(rec.Expectations, Expectation{Name: ExpectNoViolations, OK: ok, Detail: strings.TrimSpace(detail)})
	return nil
}

// takeRequests returns the scripted client's requests since the last call;
// none in live mode.
func (e *env) takeRequests() []llm.Request {
	if e.scripted == nil {
		return nil
	}
	return e.scripted.takeRequests()
}

// promptDates checks the "nil dates" regression on the recorded requests:
// every system prompt names today and each date of the window with its
// weekday.
func promptDates(reqs []llm.Request, today time.Time, e *env) Expectation {
	x := Expectation{Name: ExpectPromptDates}
	if len(reqs) == 0 {
		x.Detail = "no LLM request recorded"
		return x
	}
	want := []string{weekdayDate(today)}
	for _, d := range e.window.Days() {
		want = append(want, weekdayDate(d))
	}
	for i, r := range reqs {
		for _, w := range want {
			if !strings.Contains(r.System, w) {
				x.Detail = fmt.Sprintf("request %d: the prompt lacks %q", i+1, w)
				return x
			}
		}
	}
	x.OK = true
	return x
}

func weekdayDate(d time.Time) string { return d.Weekday().String() + " " + d.Format(time.DateOnly) }

// rows is the persisted plan around the window, for the expectations.
type rows struct {
	plates   map[int64]plateRow
	sessions map[int64]sessionRow
}

type plateRow struct {
	Date       string
	SlotID     int64
	Eaters     float64
	State      plate.State
	Components []componentRow
}

type componentRow struct {
	FoodID    int64
	Portions  *float64
	SessionID *int64
}

type sessionRow struct {
	FoodID         int64
	CookDate       string
	CookSlotID     int64
	Portions       float64
	FrozenPortions float64
	Cooked         bool
	State          plate.State
}

// rowsLookback reaches the scenario's history before the window.
const rowsLookback = 42

func (e *env) rows(ctx context.Context) (rows, error) {
	from, to := e.window.Start.AddDate(0, 0, -rowsLookback), e.window.End.AddDate(0, 0, 7)
	ps, err := e.plateRow.ListByDateRange(ctx, from, to)
	if err != nil {
		return rows{}, err
	}
	ss, _, err := e.sessions.ListInRange(ctx, from, to)
	if err != nil {
		return rows{}, err
	}
	r := rows{plates: map[int64]plateRow{}, sessions: map[int64]sessionRow{}}
	for _, p := range ps {
		pr := plateRow{Date: p.DateString(), SlotID: p.SlotID, Eaters: p.Eaters, State: p.State}
		for _, c := range p.Components {
			pr.Components = append(pr.Components, componentRow{FoodID: c.FoodID, Portions: c.Portions, SessionID: c.CookSessionID})
		}
		r.plates[p.ID] = pr
	}
	for _, s := range ss {
		r.sessions[s.ID] = sessionRow{
			FoodID: s.FoodID, CookDate: s.CookDate.Format(time.DateOnly), CookSlotID: s.CookSlotID,
			Portions: s.Portions, FrozenPortions: s.FrozenPortions, Cooked: s.CookedAt != nil, State: s.State,
		}
	}
	return r, nil
}

// expect checks a scenario's own expectations.
func (e *env) expect(sc Scenario, st Step, before, after rows, rec Record) []Expectation {
	var out []Expectation
	x := sc.Expect
	if x.KeptConfirmed {
		var removable []string
		for _, c := range st.None {
			id, _ := e.slot(c.Slot)
			removable = append(removable, cellKey(c.Date.Format(time.DateOnly), id))
		}
		out = append(out, compare(ExpectKeptConfirmed, before, after, func(p plateRow) bool {
			return p.State == plate.StateConfirmed && !slices.Contains(removable, cellKey(p.Date, p.SlotID))
		}, nil))
	}
	if x.UnchangedBefore != nil {
		d := x.UnchangedBefore.Format(time.DateOnly)
		out = append(out, compare(ExpectUnchangedBefore, before, after,
			func(p plateRow) bool { return p.Date < d },
			func(s sessionRow) bool { return s.CookDate < d }))
	}
	if len(x.NoPlate) > 0 {
		ex := Expectation{Name: ExpectNoPlate, OK: true}
		for _, c := range x.NoPlate {
			id, _ := e.slot(c.Slot)
			for _, p := range after.plates {
				if p.Date == c.Date.Format(time.DateOnly) && p.SlotID == id {
					ex.OK, ex.Detail = false, fmt.Sprintf("%s %s holds a plate", p.Date, c.Slot)
				}
			}
		}
		out = append(out, ex)
	}
	if x.MinRepeats > 0 {
		repeats := 0
		if rec.Score != nil {
			repeats = rec.Score.Variety.Repeats
		}
		ex := Expectation{Name: ExpectMinRepeats, OK: repeats >= x.MinRepeats}
		if !ex.OK {
			ex.Detail = fmt.Sprintf("%d repeats, want ≥ %d", repeats, x.MinRepeats)
		}
		out = append(out, ex)
	}
	if x.FrozenSpare {
		out = append(out, frozenSpare(st, before, after))
	}
	if x.MinPlates > 0 {
		ex := Expectation{Name: ExpectMinPlates, OK: rec.Plates >= x.MinPlates}
		if !ex.OK {
			ex.Detail = fmt.Sprintf("%d plates, want ≥ %d", rec.Plates, x.MinPlates)
		}
		out = append(out, ex)
	}
	return out
}

// frozenSpare checks that an uncooked batch cooking on or after the
// re-plan's From froze more portions than it had before.
func frozenSpare(st Step, before, after rows) Expectation {
	x := Expectation{Name: ExpectFrozenSpare}
	if st.From == nil {
		x.Detail = "the step has no from"
		return x
	}
	from := st.From.Format(time.DateOnly)
	for id, s := range before.sessions {
		a, ok := after.sessions[id]
		if ok && !a.Cooked && a.CookDate >= from && a.FrozenPortions > s.FrozenPortions {
			x.OK = true
			return x
		}
	}
	x.Detail = "no batch from " + from + " on froze spare portions"
	return x
}

func cellKey(date string, slotID int64) string { return fmt.Sprintf("%s/%d", date, slotID) }

// compare checks that the plates (and sessions) keep selects in before are
// unchanged in after; a nil selector compares none.
func compare(name string, before, after rows, plates func(plateRow) bool, sessions func(sessionRow) bool) Expectation {
	x := Expectation{Name: name, OK: true}
	ids := make([]int64, 0, len(before.plates))
	for id := range before.plates {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		p := before.plates[id]
		if plates == nil || !plates(p) {
			continue
		}
		if q, ok := after.plates[id]; !ok || !reflect.DeepEqual(p, q) {
			x.OK, x.Detail = false, fmt.Sprintf("plate %d (%s) changed", id, p.Date)
			return x
		}
	}
	if sessions == nil {
		return x
	}
	ids = ids[:0]
	for id := range before.sessions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		s := before.sessions[id]
		if !sessions(s) {
			continue
		}
		if q, ok := after.sessions[id]; !ok || q != s {
			x.OK, x.Detail = false, fmt.Sprintf("session %d (%s) changed", id, s.CookDate)
			return x
		}
	}
	return x
}
