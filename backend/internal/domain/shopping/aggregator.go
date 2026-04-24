// Package shopping aggregates planned meals into a weekly shopping list by
// walking the food tree from each plate's top-level foods down to their leaf
// descendants.
package shopping

import (
	"context"
	"sort"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Item is one line on the shopping list.
type Item struct {
	FoodID     int64   `json:"food_id"`
	Name       string  `json:"name"`
	TotalGrams float64 `json:"total_grams"`
}

// Resolver walks the food tree to produce weekly shopping totals.
type Resolver struct {
	repo food.Repository
}

// NewResolver constructs a Resolver backed by the given food repo.
func NewResolver(r food.Repository) *Resolver {
	return &Resolver{repo: r}
}

// FromPlates aggregates total grams per leaf food across every plate in the
// week. For a leaf food on a plate we count its serving portions directly;
// for a composed food we walk children recursively, scaling by
// portions / reference_portions at the top level.
func (r *Resolver) FromPlates(ctx context.Context, plates []plate.Plate) ([]Item, error) {
	totals := map[int64]*Item{}
	// Cache the (leaf-aggregate) grams per composed food id, weighted by
	// 1 / reference_portions so multiplying by plate.Portions yields the
	// correct per-plate contribution for each leaf descendant.
	perServing := map[int64]map[int64]float64{} // foodID → leafID → grams/serving
	names := map[int64]string{}                 // leafID → name

	for _, pl := range plates {
		for _, pc := range pl.Components {
			leaves, err := r.perServing(ctx, pc.FoodID, perServing, names)
			if err != nil {
				return nil, err
			}
			for leafID, grams := range leaves {
				e, ok := totals[leafID]
				if !ok {
					e = &Item{FoodID: leafID, Name: names[leafID]}
					totals[leafID] = e
				}
				e.TotalGrams += grams * pc.Portions
			}
		}
	}

	items := make([]Item, 0, len(totals))
	for _, e := range totals {
		items = append(items, *e)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// perServing returns the leaf-aggregated grams for one plated portion of food
// `id`. The caller multiplies by plate_component.Portions.
func (r *Resolver) perServing(ctx context.Context, id int64, cache map[int64]map[int64]float64, names map[int64]string) (map[int64]float64, error) {
	if m, ok := cache[id]; ok {
		return m, nil
	}
	f, err := r.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	result := map[int64]float64{}
	if f.Kind == food.KindLeaf {
		// A leaf food plated at portions=1 contributes 100 g of itself. This
		// keeps the unit consistent with the leaf's per-100g nutrition columns,
		// so Shopping and Nutrition share the same scaling: 1 portion = 100 g.
		names[id] = f.Name
		result[id] = 100
		cache[id] = result
		return result, nil
	}
	// Composed: scale per-reference-portion totals by 1/ReferencePortions.
	ref := float64(1)
	if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
		ref = *f.ReferencePortions
	}
	for _, ch := range f.Children {
		sub, err := r.perServingChild(ctx, ch, cache, names)
		if err != nil {
			return nil, err
		}
		for leafID, g := range sub {
			result[leafID] += g / ref
		}
	}
	cache[id] = result
	return result, nil
}

// perServingChild resolves one child link of a composed parent. Leaf children
// contribute their child.Grams directly; composed children recurse and scale
// by (child.Grams / childRecipeTotalGrams) so we count only the child's share.
func (r *Resolver) perServingChild(ctx context.Context, ch food.FoodComponent, cache map[int64]map[int64]float64, names map[int64]string) (map[int64]float64, error) {
	child, err := r.repo.Get(ctx, ch.ChildID)
	if err != nil {
		return nil, err
	}
	if child.Kind == food.KindLeaf {
		names[child.ID] = child.Name
		return map[int64]float64{child.ID: ch.Grams}, nil
	}
	// Composed child: get its leaves per reference portion, multiply by
	// (ch.Grams / totalChildGrams) to take only the fraction the parent uses.
	childLeaves, err := r.perServing(ctx, child.ID, cache, names)
	if err != nil {
		return nil, err
	}
	totalGrams := float64(0)
	for _, g := range childLeaves {
		totalGrams += g
	}
	if totalGrams <= 0 {
		return childLeaves, nil
	}
	factor := ch.Grams / totalGrams
	scaled := map[int64]float64{}
	for leafID, g := range childLeaves {
		scaled[leafID] = g * factor
	}
	return scaled, nil
}
