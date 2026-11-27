package planning

import "github.com/jaltszeimer/plantry/backend/internal/domain/food"

// A recipe and its variants (one variant group) are one dish for H8 (VAR-3):
// the group is not eaten again within the variety weeks, and its members count
// once in the eligible recipes. Hidden (archived) substitution variants are
// never candidates, but what was eaten of them counts for the group.

func groupOf(f *food.Food) int64 {
	if f.VariantGroupID == nil {
		return 0
	}
	return *f.VariantGroupID
}

// dish is the identity of id as a dish: its food id, or for a variant group
// the negated group id (never a food id).
func (s State) dish(id int64) int64 {
	if g := s.Foods[id].VariantGroup; g != 0 {
		return -g
	}
	return id
}

// ShareGroupHistory gives every member of a variant group the newest
// LastEaten of the group. Members missing from foods contribute nothing.
func ShareGroupHistory(foods map[int64]FoodFacts) {
	newest := map[int64]*FoodFacts{}
	for _, f := range foods {
		if f.VariantGroup == 0 || f.LastEaten == nil {
			continue
		}
		if n := newest[f.VariantGroup]; n == nil || f.LastEaten.After(*n.LastEaten) {
			f := f
			newest[f.VariantGroup] = &f
		}
	}
	for id, f := range foods {
		if n := newest[f.VariantGroup]; f.VariantGroup != 0 && n != nil {
			f.LastEaten = n.LastEaten
			foods[id] = f
		}
	}
}
