package finds

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// LibraryDeps wire the Library of the app (food service, catalogue, import
// finalize and plates).
type LibraryDeps struct {
	Foods interface {
		Create(ctx context.Context, f *food.Food) error
		Delete(ctx context.Context, id int64) error
		StartTrial(ctx context.Context, id int64) error
	}
	Catalogue food.Repository
	Origins   food.OriginStore
	Finalizer interface {
		Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedFood, error)
	}
	// Plates is plate.Service.
	Plates interface {
		Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
		RemoveMeals(ctx context.Context, ids []int64) (plate.MealResult, error)
	}
}

type library struct{ d LibraryDeps }

// NewLibrary returns the app's Library.
func NewLibrary(d LibraryDeps) Library { return library{d: d} }

func (l library) Leaf(ctx context.Context, c food.Candidate, o food.Origin) (int64, error) {
	id, _, err := food.EnsureLeaf(ctx, l.d.Catalogue, l.d.Origins, c, o)
	return id, err
}

func (l library) Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedFood, error) {
	return l.d.Finalizer.Finalize(ctx, in)
}

func (l library) Create(ctx context.Context, f *food.Food) error { return l.d.Foods.Create(ctx, f) }

func (l library) Delete(ctx context.Context, id int64) error { return l.d.Foods.Delete(ctx, id) }

func (l library) StartTrial(ctx context.Context, id int64) error {
	return l.d.Foods.StartTrial(ctx, id)
}

// HasPortion reports whether leaf foodID stores a portion for the unit.
func (l library) HasPortion(ctx context.Context, foodID int64, unit string) (bool, error) {
	ps, err := l.d.Catalogue.ListPortions(ctx, foodID)
	if err != nil {
		return false, err
	}
	u := units.Normalize(unit)
	return slices.ContainsFunc(ps, func(p food.Portion) bool { return units.Normalize(p.Unit) == u }), nil
}

// OnDraft reports whether a draft plate of the window eats foodID.
func (l library) OnDraft(ctx context.Context, foodID int64, windowStart time.Time) (bool, error) {
	plates, err := l.d.Plates.Range(ctx, windowStart, windowStart.AddDate(0, 0, 6))
	if err != nil {
		return false, err
	}
	for _, p := range plates {
		if p.State != plate.StateDraft {
			continue
		}
		for _, c := range p.Components {
			if c.FoodID == foodID {
				return true, nil
			}
		}
	}
	return false, nil
}

// Unplan removes the draft plates of the window that eat foodID.
func (l library) Unplan(ctx context.Context, foodID int64, windowStart time.Time) error {
	plates, err := l.d.Plates.Range(ctx, windowStart, windowStart.AddDate(0, 0, 6))
	if err != nil {
		return err
	}
	var ids []int64
	for _, p := range plates {
		for _, c := range p.Components {
			if c.FoodID != foodID {
				continue
			}
			if p.State != plate.StateDraft {
				return fmt.Errorf("%w: the recipe is on an accepted plate", domain.ErrInUse)
			}
			ids = append(ids, p.ID)
			break
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = l.d.Plates.RemoveMeals(plate.WithoutTaste(ctx), ids) // an Undo is no taste
	return err
}
