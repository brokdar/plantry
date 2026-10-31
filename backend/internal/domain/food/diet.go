package food

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// DietSource says where a food's effective diet class comes from (D28).
type DietSource string

// The diet class sources.
const (
	// DietSourceAuto: a leaf classified from its name (diet.Classify).
	DietSourceAuto DietSource = "auto"
	// DietSourceUser: the food's override.
	DietSourceUser DietSource = "user"
	// DietSourceDerived: a recipe's max over its children.
	DietSourceDerived DietSource = "derived"
)

// ownDiet is the class a food has without looking at its children: its
// override, or a leaf's classification. ok is false for a recipe without an
// override.
func ownDiet(f *Food) (diet.Class, DietSource, bool) {
	switch {
	case f.DietOverride != nil:
		return *f.DietOverride, DietSourceUser, true
	case f.Kind == KindLeaf:
		return diet.Classify(f.Name), DietSourceAuto, true
	}
	return diet.Unknown, DietSourceDerived, false
}

// SetDietClasses sets DietClass and DietClassSource on every food of nodes,
// a LoadTrees result. A recipe without an override takes the max of its
// children (diet.Max), so any unknown child makes it unknown; a child missing
// from nodes, and a cycle, count as unknown.
func SetDietClasses(nodes map[int64]*Food) {
	memo := make(map[int64]diet.Class, len(nodes))
	for id := range nodes {
		dietOf(nodes, id, memo)
	}
}

func dietOf(nodes map[int64]*Food, id int64, memo map[int64]diet.Class) diet.Class {
	if c, ok := memo[id]; ok {
		return c
	}
	f, ok := nodes[id]
	if !ok {
		return diet.Unknown
	}
	memo[id] = diet.Unknown // a cycle reads unknown
	c, src, ok := ownDiet(f)
	if !ok {
		classes := make([]diet.Class, len(f.Children))
		for i, ch := range f.Children {
			classes[i] = dietOf(nodes, ch.ChildID, memo)
		}
		c = diet.Max(classes...)
	}
	f.DietClass, f.DietClassSource = c, src
	memo[id] = c
	return c
}

// DeriveDiet sets the effective diet class on foods. The recipes without an
// override load their trees once, together (LoadTrees).
func (s *Service) DeriveDiet(ctx context.Context, foods ...*Food) error {
	var recipes []int64
	for _, f := range foods {
		if _, _, ok := ownDiet(f); !ok {
			recipes = append(recipes, f.ID)
		}
	}
	nodes := map[int64]*Food{}
	if len(recipes) > 0 {
		var err error
		if nodes, err = LoadTrees(ctx, s.repo, recipes); err != nil {
			return err
		}
		SetDietClasses(nodes)
	}
	for _, f := range foods {
		if c, src, ok := ownDiet(f); ok {
			f.DietClass, f.DietClassSource = c, src
			continue
		}
		f.DietClass, f.DietClassSource = nodes[f.ID].DietClass, DietSourceDerived
	}
	return nil
}

// SetDietClass sets a food's diet class override; nil clears it so the class
// is derived again.
func (s *Service) SetDietClass(ctx context.Context, id int64, class *diet.Class) (*Food, error) {
	if class != nil && !diet.Valid(*class) {
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidDietClass, *class)
	}
	return s.repo.SetDietClass(ctx, id, class)
}

// listByDiet is List with the diet filter: the class is derived, not stored,
// so every other filter runs in the repository, the diet filter here, and
// the page is cut afterwards.
func (s *Service) listByDiet(ctx context.Context, q ListQuery) (*ListResult, error) {
	if !diet.Valid(q.Diet) {
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidDietClass, q.Diet)
	}
	limit, offset := q.Limit, max(q.Offset, 0)
	q.Limit, q.Offset = maxListLimit, 0
	all, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	foods := make([]*Food, len(all.Items))
	for i := range all.Items {
		foods[i] = &all.Items[i]
	}
	if err := s.DeriveDiet(ctx, foods...); err != nil {
		return nil, err
	}
	kept := make([]Food, 0, len(all.Items))
	for _, f := range all.Items {
		if diet.Compatible(f.DietClass, q.Diet) {
			kept = append(kept, f)
		}
	}
	out := &ListResult{Total: len(kept), Items: []Food{}}
	if offset < len(kept) {
		out.Items = kept[offset:min(len(kept), offset+limit)]
	}
	return out, nil
}
