package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// Mid-week re-plan (spec §3.5, UX §5.10): the plates in No-plate cells from
// From on go, the batches they ate from shrink or freeze their spare, and an
// optional empty_cells run fills the rest of the window.

// SpareMode is what a re-plan does with an uncooked batch's spare portions.
type SpareMode string

// Spare modes: freeze them (when freezing is on and the recipe freezable),
// or keep the batch to what is still eaten.
const (
	SpareFreeze SpareMode = "freeze"
	SpareKeep   SpareMode = "keep"
)

// ReplanQuery names a re-plan's range [From, To] (inside one window, From ≥
// Today) and the cells it marks No plate on top of those that already are.
type ReplanQuery struct {
	From, To  time.Time
	Today     time.Time
	NoneCells []plate.CellKey
}

// ReplanRequest is a re-plan: confirmed plates go only with
// UnlockConfirmed; Regenerate starts an empty_cells run over [From, To]
// with Chooser (empty: the LLM when a provider resolves, else the
// heuristic, as for Regenerate; D11).
type ReplanRequest struct {
	ReplanQuery
	UnlockConfirmed bool
	Spare           SpareMode
	Regenerate      bool
	Chooser         airun.Chooser
	Instruction     string
}

// AffectedPlate is a confirmed plate a re-plan removes. FoodName is its
// first component's.
type AffectedPlate struct {
	PlateID  int64
	Date     time.Time
	SlotID   int64
	FoodName string
}

// Spare is a batch that would cook more than is still eaten: Portions spare.
// Freezable: the batch is not cooked and cooks on or after From, freezing is
// on and the recipe not marked unfreezable.
type Spare struct {
	SessionID int64
	FoodName  string
	Portions  float64
	Freezable bool
}

// ReplanPreview is what a re-plan would change: the No-plate cells that hold
// plates, the confirmed plates among them and the batches left with spare.
type ReplanPreview struct {
	NoneCells         []plate.CellKey
	ConfirmedAffected []AffectedPlate
	Spare             []Spare
}

// ReplanResult is a re-plan's outcome: the snapshot that restores the
// removed plates and the run it started (nil without Regenerate).
type ReplanResult struct {
	Removed plate.PlanSnapshot
	RunID   *int64
}

// ConfirmedAffectedError is ErrConfirmedAffected with the confirmed plates
// the re-plan would remove.
type ConfirmedAffectedError struct {
	Plates []AffectedPlate
}

func (e *ConfirmedAffectedError) Error() string {
	return fmt.Sprintf("%s: %d plates", domain.ErrConfirmedAffected, len(e.Plates))
}

func (e *ConfirmedAffectedError) Unwrap() error { return domain.ErrConfirmedAffected }

// replanScan is a re-plan worked out against the current plan.
type replanScan struct {
	preview ReplanPreview
	plates  []int64
	// spare holds every batch with spare (and every cooked or earlier batch
	// that loses all its links), keyed by session.
	spare map[int64]spareBatch
	// write are the requested cells not already No plate.
	write []plate.CellKey
}

// spareBatch is a batch losing links: its row, what stays eaten and whether
// the re-plan may change it (not cooked, cooking on or after From).
type spareBatch struct {
	sess      plate.CookSession
	after     float64
	freezable bool
	mutable   bool
}

// PreviewReplan lists what Replan would change.
func (s *Service) PreviewReplan(ctx context.Context, q ReplanQuery) (*ReplanPreview, error) {
	var sc *replanScan
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		sc, err = s.scanReplan(ctx, q)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &sc.preview, nil
}

// Replan removes the plates in the No-plate cells of [From, To] and settles
// their batches in one unit of work, then, with Regenerate, starts an
// empty_cells run over [From, To]. Confirmed plates need UnlockConfirmed,
// else it fails with a *ConfirmedAffectedError. Days before From are never
// touched, nor are cooked batches' portions.
func (s *Service) Replan(ctx context.Context, req ReplanRequest) (*ReplanResult, error) {
	switch req.Spare {
	case SpareFreeze, SpareKeep:
	default:
		return nil, fmt.Errorf("%w: spare %q", domain.ErrInvalidInput, req.Spare)
	}
	if req.Regenerate {
		if err := s.startable(ctx, req.Chooser); err != nil {
			return nil, err
		}
	}
	sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return nil, err
	}
	// No generation may write the window while its plates go.
	w := window.Containing(dateOnly(req.From), sd)
	if err := s.d.Progress.reserve(w.Start); err != nil {
		return nil, err
	}
	res := &ReplanResult{}
	err = s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		sc, err := s.scanReplan(ctx, req.ReplanQuery)
		if err != nil {
			return err
		}
		if len(sc.preview.ConfirmedAffected) > 0 && !req.UnlockConfirmed {
			return &ConfirmedAffectedError{Plates: sc.preview.ConfirmedAffected}
		}
		res.Removed, err = s.d.Plates.RemovePlates(ctx, sc.plates, sc.keep(req.Spare))
		if err != nil {
			return err
		}
		for _, c := range sc.write {
			if _, _, err := s.d.Overrides.SetOverride(ctx, availability.OverrideInput{Date: c.Date, SlotID: c.SlotID, Mode: availability.ModeNone}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || !req.Regenerate {
		s.d.Progress.release(w.Start)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	// The run starts on the reservation the re-plan holds, so no other
	// generation slips in between; the removal is committed either way, so a
	// run that fails to start is logged and leaves run_id nil.
	id, err := s.regenerate(ctx, Request{
		Start: w.Start, Today: req.Today, Chooser: req.Chooser, Instruction: req.Instruction,
		Scope: Scope{Kind: ScopeEmptyCells, From: req.From, To: req.To},
	}, true)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "plan.replan regenerate did not start", "window", w.Start.Format(time.DateOnly), "error", err)
		return res, nil
	}
	res.RunID = &id
	return res, nil
}

// startable checks up front what would stop Regenerate from starting, so a
// re-plan does not commit and then fail to fill the week.
func (s *Service) startable(ctx context.Context, chooser airun.Chooser) error {
	n, err := s.d.FoodRows.CountComposedFoods(ctx)
	if err != nil {
		return fmt.Errorf("count recipes: %w", err)
	}
	if n < setup.RecipeTarget {
		return fmt.Errorf("%w: %d recipes", domain.ErrNotEnoughRecipes, n)
	}
	_, _, err = s.chooser(ctx, chooser)
	return err
}

// keep is what RemovePlates sets each spare batch to: an uncooked one cooking
// from From on freezes its spare (freeze, when freezable) or shrinks to what
// is still eaten; a cooked or earlier one keeps its portions. A batch that
// loses all its links is not here, so D11 removes it.
func (sc *replanScan) keep(mode SpareMode) map[int64]plate.SessionPortions {
	out := make(map[int64]plate.SessionPortions, len(sc.spare))
	for id, b := range sc.spare {
		p := plate.SessionPortions{Portions: b.sess.Portions, FrozenPortions: b.sess.FrozenPortions}
		if b.mutable {
			if mode == SpareFreeze && b.freezable {
				p.FrozenPortions = min(p.Portions, p.FrozenPortions+b.sess.Portions-b.after)
			} else {
				p.Portions = b.after
				p.FrozenPortions = min(p.FrozenPortions, b.after)
			}
		}
		out[id] = p
	}
	return out
}

func (s *Service) scanReplan(ctx context.Context, q ReplanQuery) (*replanScan, error) {
	from, to, today := dateOnly(q.From), dateOnly(q.To), dateOnly(q.Today)
	if from.Before(today) {
		return nil, fmt.Errorf("%w: from %s", domain.ErrReplanPast, from.Format(time.DateOnly))
	}
	sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return nil, err
	}
	sc := &replanScan{spare: map[int64]spareBatch{}}
	if to.Before(from) || !window.Containing(from, sd).Contains(to) {
		return nil, fmt.Errorf("%w: from and to must lie in one window, from first", domain.ErrInvalidInput)
	}
	cells, err := s.d.Cells.Resolve(ctx, from, to)
	if err != nil {
		return nil, err
	}
	asked := map[cellID]bool{}
	for _, c := range q.NoneCells {
		asked[idOf(plate.CellKey{Date: dateOnly(c.Date), SlotID: c.SlotID})] = true
	}
	plates, err := s.d.PlateRows.ListByDateRange(ctx, from, to)
	if err != nil {
		return nil, err
	}
	occupied := map[cellID]plate.Plate{}
	for _, p := range plates {
		occupied[idOf(p.Cell())] = p
	}
	sc.preview = ReplanPreview{NoneCells: []plate.CellKey{}, ConfirmedAffected: []AffectedPlate{}, Spare: []Spare{}}
	var removed []plate.Plate
	for _, c := range cells {
		k := plate.CellKey{Date: c.Date, SlotID: c.SlotID}
		if asked[idOf(k)] {
			delete(asked, idOf(k))
			if c.Mode != availability.ModeNone {
				sc.write = append(sc.write, k)
			}
		} else if c.Mode != availability.ModeNone {
			continue
		}
		p, ok := occupied[idOf(k)]
		if !ok {
			continue
		}
		sc.preview.NoneCells = append(sc.preview.NoneCells, k)
		sc.plates = append(sc.plates, p.ID)
		removed = append(removed, p)
		if p.State == plate.StateConfirmed {
			a := AffectedPlate{PlateID: p.ID, Date: p.Date, SlotID: p.SlotID}
			if len(p.Components) > 0 {
				a.FoodName = p.Components[0].FoodName
			}
			sc.preview.ConfirmedAffected = append(sc.preview.ConfirmedAffected, a)
		}
	}
	if len(asked) > 0 {
		return nil, fmt.Errorf("%w: a No-plate cell is outside [from, to] or its slot inactive", domain.ErrInvalidInput)
	}
	return sc, s.scanSpare(ctx, sc, removed, from)
}

// scanSpare finds the batches the removed plates ate from that keep links
// and would cook more than those links eat.
func (s *Service) scanSpare(ctx context.Context, sc *replanScan, removed []plate.Plate, from time.Time) error {
	gone := map[int64]float64{}
	links := map[int64]int{}
	var order []int64
	for _, p := range removed {
		for _, c := range p.Components {
			if c.CookSessionID == nil || c.Portions == nil {
				continue
			}
			id := *c.CookSessionID
			if _, seen := links[id]; !seen {
				order = append(order, id)
			}
			gone[id] += *c.Portions * p.Eaters
			links[id]++
		}
	}
	if len(order) == 0 {
		return nil
	}
	set, err := plate.ReadBatchSettings(ctx, s.d.Settings)
	if err != nil {
		return err
	}
	var views []*plate.SessionView
	var foodIDs []int64
	for _, id := range order {
		v, err := s.d.Plates.Session(ctx, id)
		if err != nil {
			return err
		}
		if len(v.Links) == links[id] && mutable(v, from) {
			continue // nothing left to eat it and not cooked yet: D11 removes it
		}
		views = append(views, v)
		foodIDs = append(foodIDs, v.FoodID)
	}
	foods, err := s.d.FoodRows.LookupByIDs(ctx, foodIDs)
	if err != nil {
		return err
	}
	for _, v := range views {
		after := v.Consumed - gone[v.ID]
		spare := v.Portions - after
		if spare < 1e-6 {
			continue
		}
		f := foods[v.FoodID]
		// Only a batch the re-plan may change can freeze its spare.
		freezable := mutable(v, from) && set.FreezingEnabled && (f == nil || f.Freezable == nil || *f.Freezable)
		sc.spare[v.ID] = spareBatch{
			sess: v.CookSession, after: after, freezable: freezable,
			mutable: mutable(v, from),
		}
		sc.preview.Spare = append(sc.preview.Spare, Spare{SessionID: v.ID, FoodName: v.FoodName, Portions: spare, Freezable: freezable})
	}
	return nil
}

// mutable reports whether a re-plan from from may change a batch: not cooked
// and cooking on or after from.
func mutable(v *plate.SessionView, from time.Time) bool {
	return v.CookedAt == nil && !v.CookDate.Before(from)
}
