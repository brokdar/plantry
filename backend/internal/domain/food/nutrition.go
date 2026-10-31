package food

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

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
// incomplete leaves, unique by ID and sorted case-insensitively by name (then
// ID); it is empty (never nil) when Complete is true.
type Detailed struct {
	Macros   nutrition.Macros
	Complete bool
	Missing  []MissingLeaf
}

// MissingLeaf identifies a leaf food that lacks core macros.
type MissingLeaf struct {
	ID   int64
	Name string
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
	out, err := r.PerPortionDetailedMany(ctx, []int64{foodID})
	if err != nil {
		return Detailed{}, err
	}
	return out[foodID], nil
}

// PerPortionDetailedMany resolves PerPortionDetailed for several foods at
// once. The food trees load level by level, with one LookupByIDs and one
// ListChildrenByParents per level however many foods there are, and every
// food is loaded once even when several trees share it.
func (r *NutritionResolver) PerPortionDetailedMany(ctx context.Context, ids []int64) (map[int64]Detailed, error) {
	out := make(map[int64]Detailed, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	nodes, err := LoadTrees(ctx, r.repo, ids)
	if err != nil {
		return nil, err
	}
	w := &walk{nodes: nodes, memo: map[int64]subtree{}}
	for _, id := range ids {
		f := nodes[id]
		total, missing, err := w.total(f, 0)
		if err != nil {
			return nil, err
		}
		d := Detailed{Macros: total, Missing: sortedMissing(missing)}
		d.Complete = len(d.Missing) == 0
		if f.Kind == KindComposed {
			portions := float64(1)
			if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
				portions = *f.ReferencePortions
			}
			d.Macros = scale(total, 1/portions)
		}
		out[id] = d
	}
	return out, nil
}

// TreeReader loads food headers and child links in batches.
type TreeReader interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*Food, error)
	ListChildrenByParents(ctx context.Context, parentIDs []int64) (map[int64][]FoodComponent, error)
}

// LoadTrees loads the given foods and everything below them, one tree level
// per round trip (one LookupByIDs and one ListChildrenByParents per level).
// Composed foods get their Children set; every food loads once even when
// several trees share it.
func LoadTrees(ctx context.Context, r TreeReader, ids []int64) (map[int64]*Food, error) {
	nodes := map[int64]*Food{}
	frontier := slices.Compact(slices.Sorted(slices.Values(ids)))
	for len(frontier) > 0 {
		foods, err := r.LookupByIDs(ctx, frontier)
		if err != nil {
			return nil, err
		}
		var composed []int64
		for _, id := range frontier {
			f, ok := foods[id]
			if !ok {
				return nil, fmt.Errorf("%w: food %d not found during nutrition resolve", domain.ErrNotFound, id)
			}
			nodes[id] = f
			if f.Kind == KindComposed {
				composed = append(composed, id)
			}
		}
		frontier = nil
		if len(composed) == 0 {
			break
		}
		children, err := r.ListChildrenByParents(ctx, composed)
		if err != nil {
			return nil, err
		}
		queued := map[int64]bool{}
		for _, id := range composed {
			nodes[id].Children = children[id]
			for _, ch := range children[id] {
				if _, loaded := nodes[ch.ChildID]; !loaded && !queued[ch.ChildID] {
					queued[ch.ChildID] = true
					frontier = append(frontier, ch.ChildID)
				}
			}
		}
	}
	return nodes, nil
}

// walk computes macros over loaded trees, memoising each subtree.
type walk struct {
	nodes map[int64]*Food
	memo  map[int64]subtree
}

// subtree is a food's per-100g macros and the leaves under it (itself, for
// a leaf) that lack core macros, id → name.
type subtree struct {
	per100g nutrition.Macros
	missing map[int64]string
}

// total returns absolute macros for a whole food (all ingredients summed)
// and its incomplete leaves. For a leaf that means "macros for 100 g"; for a
// composed food "macros for the total recipe at reference_portions servings".
func (w *walk) total(f *Food, depth int) (nutrition.Macros, map[int64]string, error) {
	if depth > maxResolveDepth {
		return nutrition.Macros{}, nil, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	missing := map[int64]string{}
	if f.Kind == KindLeaf {
		if f.Kcal100g == nil || f.Protein100g == nil || f.Fat100g == nil || f.Carbs100g == nil {
			missing[f.ID] = f.Name
		}
		return leafPer100g(f), missing, nil
	}
	var total nutrition.Macros
	for _, ch := range f.Children {
		sub, err := w.per100g(ch.ChildID, depth+1)
		if err != nil {
			return nutrition.Macros{}, nil, err
		}
		total = add(total, scale(sub.per100g, ch.Grams/100))
		maps.Copy(missing, sub.missing)
	}
	return total, missing, nil
}

// per100g returns the per-100g macros for any food (leaf or composed).
// Composed foods normalise their total by the total child grams so callers
// can use them as if they were a leaf.
func (w *walk) per100g(id int64, depth int) (subtree, error) {
	if s, ok := w.memo[id]; ok {
		return s, nil
	}
	if depth > maxResolveDepth {
		return subtree{}, fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxResolveDepth)
	}
	f, ok := w.nodes[id]
	if !ok {
		return subtree{}, fmt.Errorf("%w: food %d not found during nutrition resolve", domain.ErrNotFound, id)
	}
	total, missing, err := w.total(f, depth)
	if err != nil {
		return subtree{}, err
	}
	s := subtree{per100g: total, missing: missing}
	if f.Kind == KindComposed {
		totalGrams := float64(0)
		for _, ch := range f.Children {
			totalGrams += ch.Grams
		}
		s.per100g = nutrition.Macros{}
		if totalGrams > 0 {
			s.per100g = scale(total, 100/totalGrams)
		}
	}
	w.memo[id] = s
	return s, nil
}

// sortedMissing lists the leaves sorted case-insensitively by name, then id;
// empty, never nil.
func sortedMissing(missing map[int64]string) []MissingLeaf {
	out := make([]MissingLeaf, 0, len(missing))
	for id, name := range missing {
		out = append(out, MissingLeaf{ID: id, Name: name})
	}
	slices.SortFunc(out, func(a, b MissingLeaf) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out
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
