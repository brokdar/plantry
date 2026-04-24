package food

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

// maxResolveDepth guards against pathological recursion. Real food trees are
// shallow (≤ 4 in practice).
const maxResolveDepth = 16

// NutritionResolver walks a food tree to compute macros. Leaves read direct
// per-100g columns; composed foods aggregate children recursively and divide
// by reference_portions for per-portion output.
type NutritionResolver struct {
	repo Repository
}

// NewNutritionResolver constructs a resolver backed by the given food repo.
func NewNutritionResolver(repo Repository) *NutritionResolver {
	return &NutritionResolver{repo: repo}
}

// PerPortion returns the per-portion macros for a composed food. For a leaf
// food it returns macros for 100 g (the unit the leaf reports its nutrition
// in).
func (r *NutritionResolver) PerPortion(ctx context.Context, foodID int64) (nutrition.Macros, error) {
	f, err := r.repo.Get(ctx, foodID)
	if err != nil {
		return nutrition.Macros{}, err
	}
	cache := map[int64]nutrition.Macros{} // per-100g macros, per food id
	total, err := r.totalMacros(ctx, f, cache, 0)
	if err != nil {
		return nutrition.Macros{}, err
	}
	if f.Kind == KindLeaf {
		return total, nil
	}
	portions := float64(1)
	if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
		portions = *f.ReferencePortions
	}
	return scale(total, 1/portions), nil
}

// totalMacros returns absolute macros for a whole food (all ingredients
// summed). For a leaf that means "macros for 100 g"; for a composed food
// that means "macros for the total recipe at reference_portions servings".
func (r *NutritionResolver) totalMacros(ctx context.Context, f *Food, cache map[int64]nutrition.Macros, depth int) (nutrition.Macros, error) {
	if depth > maxResolveDepth {
		return nutrition.Macros{}, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	if f.Kind == KindLeaf {
		return leafPer100g(f), nil
	}
	var total nutrition.Macros
	for _, ch := range f.Children {
		per100g, err := r.per100gFor(ctx, ch.ChildID, cache, depth+1)
		if err != nil {
			return nutrition.Macros{}, err
		}
		factor := ch.Grams / 100
		total = add(total, scale(per100g, factor))
	}
	return total, nil
}

// per100gFor returns the per-100g macros for any food (leaf or composed).
// Composed foods normalise their total by the computed total child grams so
// callers can use them as if they were a leaf.
func (r *NutritionResolver) per100gFor(ctx context.Context, id int64, cache map[int64]nutrition.Macros, depth int) (nutrition.Macros, error) {
	if m, ok := cache[id]; ok {
		return m, nil
	}
	if depth > maxResolveDepth {
		return nutrition.Macros{}, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	f, err := r.repo.Get(ctx, id)
	if err != nil {
		return nutrition.Macros{}, err
	}
	if f.Kind == KindLeaf {
		m := leafPer100g(f)
		cache[id] = m
		return m, nil
	}
	total, err := r.totalMacros(ctx, f, cache, depth)
	if err != nil {
		return nutrition.Macros{}, err
	}
	totalGrams := float64(0)
	for _, ch := range f.Children {
		totalGrams += ch.Grams
	}
	if totalGrams <= 0 {
		cache[id] = nutrition.Macros{}
		return nutrition.Macros{}, nil
	}
	per100g := scale(total, 100/totalGrams)
	cache[id] = per100g
	return per100g, nil
}

func leafPer100g(f *Food) nutrition.Macros {
	return nutrition.Macros{
		Kcal:    deref(f.Kcal100g),
		Protein: deref(f.Protein100g),
		Fat:     deref(f.Fat100g),
		Carbs:   deref(f.Carbs100g),
		Fiber:   deref(f.Fiber100g),
		Sodium:  deref(f.Sodium100g),
	}
}

func add(a, b nutrition.Macros) nutrition.Macros {
	return nutrition.Macros{
		Kcal:    a.Kcal + b.Kcal,
		Protein: a.Protein + b.Protein,
		Fat:     a.Fat + b.Fat,
		Carbs:   a.Carbs + b.Carbs,
		Fiber:   a.Fiber + b.Fiber,
		Sodium:  a.Sodium + b.Sodium,
	}
}

func scale(m nutrition.Macros, k float64) nutrition.Macros {
	return nutrition.Macros{
		Kcal:    m.Kcal * k,
		Protein: m.Protein * k,
		Fat:     m.Fat * k,
		Carbs:   m.Carbs * k,
		Fiber:   m.Fiber * k,
		Sodium:  m.Sodium * k,
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
