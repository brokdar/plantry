package planview

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// checkInput is what the plan read model already loaded for its window.
type checkInput struct {
	w        window.Window
	today    time.Time
	env      env
	cells    []availability.Cell
	plates   []plate.Plate
	sessions []plate.SessionView
	// foods holds the sessions' foods by id.
	foods   map[int64]*food.Food
	profile *profile.Profile
}

// issues checks the window as persisted with planning.Check over all its
// cells and keeps the rules the board shows (D21): H1, H2, H5 and H8. H3/H4
// already appear as session warnings; H6/H7 cannot occur in persisted data.
// An empty plan cell (missing_plate) needs attention only while the window
// is a draft, and only from today on: a hand-planned or accepted window may
// leave cells empty, and past cells are history.
func (s *Service) issues(ctx context.Context, in checkInput) ([]planning.Issue, error) {
	st, err := s.checkState(ctx, in)
	if err != nil {
		return nil, err
	}
	t := planning.Target{Cells: make([]plate.CellKey, len(st.Cells))}
	for i, c := range st.Cells {
		t.Cells[i] = c.Key()
	}
	drafts := false
	for _, p := range st.Plates {
		drafts = drafts || !p.Confirmed
	}
	for _, ss := range st.Sessions {
		drafts = drafts || !ss.Confirmed
	}
	today := in.today.Format(time.DateOnly)
	out := []planning.Issue{}
	for _, is := range planning.Check(st, t) {
		switch is.Rule {
		case "H1":
			if is.Code == planning.CodeMissingPlate && (!drafts || is.Date < today) {
				continue
			}
		case "H2", "H5", "H8":
		default:
			continue
		}
		out = append(out, is)
	}
	return out, nil
}

// checkState builds the planning state of the window from the loaded rows.
// Foods load in one lookup; when the window holds drafts every recipe is
// loaded too, so H8 knows how many unrepeated recipes there were to pick.
func (s *Service) checkState(ctx context.Context, in checkInput) (planning.State, error) {
	maxCook, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanMaxCookOccasionsPerDay)
	if err != nil {
		return planning.State{}, err
	}
	variety, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanVarietyWeeks)
	if err != nil {
		return planning.State{}, err
	}
	st := planning.State{
		Window: in.w, Today: in.today, SlotOrder: in.env.order,
		Cells:    make([]planning.Cell, len(in.cells)),
		Plates:   make([]planning.Plate, len(in.plates)),
		Sessions: make([]planning.Session, len(in.sessions)),
		Settings: planning.Settings{BatchSettings: in.env.batch, MaxCookOccasionsPerDay: maxCook, VarietyWeeks: variety},
		Profile: planning.Profile{
			KcalTarget: in.profile.KcalTarget, ProteinPct: in.profile.ProteinPct, FatPct: in.profile.FatPct,
			CarbsPct: in.profile.CarbsPct, DietType: in.profile.DietType,
			MeatDaysPerWeekMax: in.profile.MeatDaysPerWeekMax, HouseholdPortions: in.profile.HouseholdPortions,
		},
	}
	for i, c := range in.cells {
		st.Cells[i] = planning.Cell{Date: c.Date, SlotID: c.SlotID, Mode: c.Mode, PresetID: c.PresetID}
	}

	foods := make(map[int64]*food.Food, len(in.foods))
	for id, f := range in.foods {
		foods[id] = f
	}
	var missing []int64
	need := func(id int64) {
		if _, ok := foods[id]; !ok {
			foods[id] = nil
			missing = append(missing, id)
		}
	}
	drafts := false
	for i, p := range in.plates {
		drafts = drafts || p.State == plate.StateDraft
		pp := planning.Plate{
			ID: p.ID, Date: p.Date, SlotID: p.SlotID, Eaters: p.Eaters, Confirmed: p.State != plate.StateDraft,
			Components: make([]planning.Component, len(p.Components)),
		}
		for j, pc := range p.Components {
			need(pc.FoodID)
			c := planning.Component{FoodID: pc.FoodID, Portions: pc.Portions, Grams: pc.Grams}
			if pc.CookSessionID != nil {
				c.SessionID = *pc.CookSessionID
			}
			pp.Components[j] = c
		}
		st.Plates[i] = pp
	}
	for i, v := range in.sessions {
		drafts = drafts || v.State == plate.StateDraft
		st.Sessions[i] = planning.Session{CookSession: v.CookSession, Confirmed: v.State != plate.StateDraft}
	}
	if drafts {
		ids, err := s.d.Recipes.ComposedFoodIDs(ctx)
		if err != nil {
			return planning.State{}, fmt.Errorf("list recipes: %w", err)
		}
		for _, id := range ids {
			need(id)
		}
	}
	if len(missing) > 0 {
		found, err := s.d.Foods.LookupByIDs(ctx, missing)
		if err != nil {
			return planning.State{}, fmt.Errorf("lookup plan foods: %w", err)
		}
		for _, id := range missing {
			foods[id] = found[id]
		}
	}

	ids := make([]int64, 0, len(foods))
	loaded := make([]*food.Food, 0, len(foods))
	var composed []int64
	for id, f := range foods {
		if f == nil {
			continue
		}
		ids = append(ids, id)
		loaded = append(loaded, f)
		if f.Kind == food.KindComposed {
			composed = append(composed, id)
		}
	}
	var hist map[int64]food.History
	if len(composed) > 0 {
		if hist, err = s.d.Foods.History(ctx, composed, in.today); err != nil {
			return planning.State{}, fmt.Errorf("food history: %w", err)
		}
	}
	if err := s.d.Diets.DeriveDiet(ctx, loaded...); err != nil {
		return planning.State{}, fmt.Errorf("diet classes: %w", err)
	}
	st.Foods = make(map[int64]planning.FoodFacts, len(ids))
	for _, id := range ids {
		f := foods[id]
		ff := planning.FoodFacts{
			ID: id, Name: f.Name, Kind: f.Kind, DietClass: f.DietClass,
			KeepDays: f.KeepDays, Favorite: f.Favorite,
			LastEaten: hist[id].LastEaten,
		}
		if f.Role != nil {
			ff.Role = *f.Role
		}
		st.Foods[id] = ff
	}
	return st, nil
}

// draft summarises the draft plates of the window and the draft sessions it
// shows, nil without any. The newest run (highest id) that wrote one of them
// supplies run_id, chooser and notes.
func (s *Service) draft(ctx context.Context, plates []plate.Plate, sessions []plate.SessionView) (*Draft, error) {
	d := Draft{}
	var newest int64
	note := func(run *int64) {
		if run != nil && *run > newest {
			newest = *run
		}
	}
	for _, p := range plates {
		if p.State == plate.StateDraft {
			d.Plates++
			note(p.AIRunID)
		}
	}
	for _, v := range sessions {
		if v.State == plate.StateDraft {
			d.Sessions++
			note(v.AIRunID)
		}
	}
	if d.Plates == 0 && d.Sessions == 0 {
		return nil, nil
	}
	if newest > 0 {
		n, err := s.d.Runs.RunNote(ctx, newest)
		if err != nil {
			return nil, fmt.Errorf("read run %d: %w", newest, err)
		}
		d.RunID, d.Chooser, d.Notes = &newest, n.Chooser, n.Notes
	}
	return &d, nil
}
