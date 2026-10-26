package food

import (
	"context"
	"fmt"
	"slices"

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

// Detailed is the per-portion macros plus a completeness flag. Complete is
// false when any leaf in the tree lacks kcal, protein, fat or carbs per 100 g;
// those leaves still contribute 0 for the missing values. Missing holds the
// unique names of the incomplete leaves, sorted; it is empty (never nil) when
// Complete is true.
type Detailed struct {
	Macros   nutrition.Macros
	Complete bool
	Missing  []string
}

// PerPortion returns the per-portion macros for a composed food. For a leaf
// food it returns macros for 100 g (the unit the leaf reports its nutrition
// in).
func (r *NutritionResolver) PerPortion(ctx context.Context, foodID int64) (nutrition.Macros, error) {
	d, err := r.PerPortionDetailed(ctx, foodID)
	if err != nil {
		return nutrition.Macros{}, err
	}
	return d.Macros, nil
}

// PerPortionDetailed is PerPortion plus which leaves lack core macros.
func (r *NutritionResolver) PerPortionDetailed(ctx context.Context, foodID int64) (Detailed, error) {
	// LookupByIDs fetches only the food row — no children/instructions/tags — which
	// is all we need here. Children are loaded separately via ListChildren so that
	// recursive nodes also avoid full hydration.
	foods, err := r.repo.LookupByIDs(ctx, []int64{foodID})
	if err != nil {
		return Detailed{}, err
	}
	f, ok := foods[foodID]
	if !ok {
		return Detailed{}, fmt.Errorf("%w: id %d", domain.ErrNotFound, foodID)
	}
	if f.Kind == KindComposed {
		f.Children, err = r.repo.ListChildren(ctx, foodID)
		if err != nil {
			return Detailed{}, err
		}
	}
	// cache memoises per-100g macros per food id. A cached subtree has already
	// recorded its incomplete leaves in missing, so cache hits need not re-add.
	w := &walk{cache: map[int64]nutrition.Macros{}, missing: map[string]struct{}{}}
	total, err := r.totalMacros(ctx, f, w, 0)
	if err != nil {
		return Detailed{}, err
	}
	d := Detailed{Macros: total, Missing: make([]string, 0, len(w.missing))}
	for name := range w.missing {
		d.Missing = append(d.Missing, name)
	}
	slices.Sort(d.Missing)
	d.Complete = len(d.Missing) == 0
	if f.Kind == KindLeaf {
		return d, nil
	}
	portions := float64(1)
	if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
		portions = *f.ReferencePortions
	}
	d.Macros = scale(total, 1/portions)
	return d, nil
}

// walk is the per-call state of one resolve.
type walk struct {
	cache   map[int64]nutrition.Macros // per-100g macros by food id
	missing map[string]struct{}        // names of leaves lacking core macros
}

// leaf returns a leaf's per-100g macros and records it if incomplete.
func (w *walk) leaf(f *Food) nutrition.Macros {
	if f.Kcal100g == nil || f.Protein100g == nil || f.Fat100g == nil || f.Carbs100g == nil {
		w.missing[f.Name] = struct{}{}
	}
	return leafPer100g(f)
}

// totalMacros returns absolute macros for a whole food (all ingredients
// summed). For a leaf that means "macros for 100 g"; for a composed food
// that means "macros for the total recipe at reference_portions servings".
func (r *NutritionResolver) totalMacros(ctx context.Context, f *Food, w *walk, depth int) (nutrition.Macros, error) {
	if depth > maxResolveDepth {
		return nutrition.Macros{}, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	if f.Kind == KindLeaf {
		return w.leaf(f), nil
	}
	var total nutrition.Macros
	for _, ch := range f.Children {
		per100g, err := r.per100gFor(ctx, ch.ChildID, w, depth+1)
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
func (r *NutritionResolver) per100gFor(ctx context.Context, id int64, w *walk, depth int) (nutrition.Macros, error) {
	if m, ok := w.cache[id]; ok {
		return m, nil
	}
	if depth > maxResolveDepth {
		return nutrition.Macros{}, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	foods, err := r.repo.LookupByIDs(ctx, []int64{id})
	if err != nil {
		return nutrition.Macros{}, err
	}
	f, ok := foods[id]
	if !ok {
		return nutrition.Macros{}, fmt.Errorf("%w: food %d not found during nutrition resolve", domain.ErrNotFound, id)
	}
	if f.Kind == KindLeaf {
		m := w.leaf(f)
		w.cache[id] = m
		return m, nil
	}
	f.Children, err = r.repo.ListChildren(ctx, id)
	if err != nil {
		return nutrition.Macros{}, err
	}
	total, err := r.totalMacros(ctx, f, w, depth)
	if err != nil {
		return nutrition.Macros{}, err
	}
	totalGrams := float64(0)
	for _, ch := range f.Children {
		totalGrams += ch.Grams
	}
	if totalGrams <= 0 {
		w.cache[id] = nutrition.Macros{}
		return nutrition.Macros{}, nil
	}
	per100g := scale(total, 100/totalGrams)
	w.cache[id] = per100g
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
