package planview

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
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

// issues checks the window as persisted and keeps the rules the board shows
// (planning.BoardIssues).
func (s *Service) issues(ctx context.Context, in checkInput) ([]planning.Issue, error) {
	st, err := s.checkState(ctx, in)
	if err != nil {
		return nil, err
	}
	return planning.BoardIssues(st), nil
}

// checkState builds the planning state of the window from the loaded rows
// with the row mappers the context builder uses. The foods the rows name
// load in one lookup. While the window holds drafts H8 also needs the
// recipes it could have picked instead: only the uneaten ones load (one
// query), and their diet classes only when the diet type can rule one out.
func (s *Service) checkState(ctx context.Context, in checkInput) (planning.State, error) {
	set, err := planning.ReadSettings(ctx, s.d.Settings)
	if err != nil {
		return planning.State{}, err
	}
	st := planning.State{
		Window: in.w, Today: in.today, SlotOrder: in.env.order,
		Cells:    make([]planning.Cell, len(in.cells)),
		Plates:   make([]planning.Plate, len(in.plates)),
		Sessions: make([]planning.Session, len(in.sessions)),
		Settings: set,
		Profile:  planning.StateProfile(in.profile),
	}
	for i, c := range in.cells {
		st.Cells[i] = planning.StateCell(c)
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
		for _, pc := range p.Components {
			need(pc.FoodID)
		}
		st.Plates[i] = planning.StatePlate(p)
	}
	for i, v := range in.sessions {
		drafts = drafts || v.State == plate.StateDraft
		st.Sessions[i] = planning.StateSession(v.CookSession)
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
		ff := planning.BaseFacts(foods[id])
		ff.LastEaten = hist[id].LastEaten
		st.Foods[id] = ff
	}
	if drafts && set.VarietyWeeks > 0 {
		if err := s.addUneatenRecipes(ctx, &st); err != nil {
			return planning.State{}, err
		}
	}
	return st, nil
}

// addUneatenRecipes adds the recipes H8 counts as unrepeated that the rows
// do not name. Only that count reads them: they carry kind, role and, when
// the diet type can rule one out, the diet class; no history (none was
// eaten since the variety bound).
func (s *Service) addUneatenRecipes(ctx context.Context, st *planning.State) error {
	since := planning.VarietySince(st.Window.Start, st.Settings.VarietyWeeks)
	rows, err := s.d.Recipes.UneatenRecipes(ctx, since, st.Today)
	if err != nil {
		return fmt.Errorf("list uneaten recipes: %w", err)
	}
	var ids []int64
	for _, r := range rows {
		if _, known := st.Foods[r.ID]; known {
			continue
		}
		ff := planning.FoodFacts{ID: r.ID, Kind: food.KindComposed, ReferencePortions: 1}
		if r.Role != nil {
			ff.Role = *r.Role
		}
		st.Foods[r.ID] = ff
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 || !planning.DietRestricts(st.Profile.DietType) {
		return nil
	}
	found, err := s.d.Foods.LookupByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("lookup uneaten recipes: %w", err)
	}
	loaded := slices.Collect(maps.Values(found))
	if err := s.d.Diets.DeriveDiet(ctx, loaded...); err != nil {
		return fmt.Errorf("diet classes: %w", err)
	}
	for _, f := range loaded {
		ff := st.Foods[f.ID]
		ff.DietClass = f.DietClass
		st.Foods[f.ID] = ff
	}
	return nil
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
