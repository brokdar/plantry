package food

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// MaxScalePortions caps the portions a recipe can be scaled to.
const MaxScalePortions = 100

// Scaled is a composed food's ingredient list for a number of portions.
type Scaled struct {
	FoodID            int64
	Portions          float64
	ReferencePortions float64
	Factor            float64
	Ingredients       []ScaledIngredient
}

// ScaledIngredient is one child line scaled by the factor, in the child's own
// unit. Display is the rounded amount to show (units.Display); Buy is set for
// count units only (units.Buy).
type ScaledIngredient struct {
	ChildID int64
	Name    string
	Kind    Kind
	Amount  float64
	Unit    string
	Grams   float64
	Display units.Quantity
	Buy     *units.Quantity
}

// Scale scales the children of composed food f to portions, with
// factor = portions / reference_portions (a missing reference counts as 1).
// A composed child is scaled as a single line. portions must be in
// (0, MaxScalePortions].
func Scale(f *Food, portions float64) (*Scaled, error) {
	if err := validScalePortions(portions); err != nil {
		return nil, err
	}
	if f.Kind != KindComposed {
		return nil, fmt.Errorf("%w: only composed foods scale", domain.ErrInvalidInput)
	}
	ref := 1.0
	if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
		ref = *f.ReferencePortions
	}
	factor := portions / ref
	out := &Scaled{
		FoodID:            f.ID,
		Portions:          portions,
		ReferencePortions: ref,
		Factor:            factor,
		Ingredients:       make([]ScaledIngredient, 0, len(f.Children)),
	}
	for _, ch := range f.Children {
		amount := ch.Amount * factor
		line := ScaledIngredient{
			ChildID: ch.ChildID,
			Name:    ch.ChildName,
			Kind:    ch.ChildKind,
			Amount:  amount,
			Unit:    ch.Unit,
			Grams:   ch.Grams * factor,
			Display: units.Display(amount, ch.Unit),
		}
		if buy, ok := units.Buy(amount, ch.Unit); ok {
			line.Buy = &buy
		}
		out.Ingredients = append(out.Ingredients, line)
	}
	return out, nil
}

// Scaled loads food id and scales it to portions. Invalid portions fail
// before the lookup.
func (s *Service) Scaled(ctx context.Context, id int64, portions float64) (*Scaled, error) {
	if err := validScalePortions(portions); err != nil {
		return nil, err
	}
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return Scale(f, portions)
}

func validScalePortions(portions float64) error {
	if portions > 0 && portions <= MaxScalePortions {
		return nil
	}
	return fmt.Errorf("%w: portions must be > 0 and <= %d", domain.ErrInvalidPortions, MaxScalePortions)
}
