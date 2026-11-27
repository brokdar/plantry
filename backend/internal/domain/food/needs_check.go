package food

import "context"

// NeedsCheckLine is one recipe line whose leaf has no nutrition.
type NeedsCheckLine struct {
	RecipeID   int64
	RecipeName string
	LeafID     int64
	LeafName   string
	Amount     float64
	Unit       string
}

// NeedsCheckRecipe is a recipe with its problem lines.
type NeedsCheckRecipe struct {
	ID    int64
	Name  string
	Lines []NeedsCheckLine
}

// NeedsCheckLeaf is a leaf without nutrition and the recipes using it.
type NeedsCheckLeaf struct {
	ID      int64
	Name    string
	Recipes []NeedsCheckRecipe
}

// NeedsCheck is the legacy "Needs your check" list (spec §7): leaves without
// nutrition that recipes use, and the recipes with such a line. The two are
// the same lines grouped two ways; fixing a leaf clears all its recipes.
type NeedsCheck struct {
	Leaves  []NeedsCheckLeaf
	Recipes []NeedsCheckRecipe
}

// NeedsCheck groups the repository's problem lines, in recipe-name order.
func (s *Service) NeedsCheck(ctx context.Context) (*NeedsCheck, error) {
	lines, err := s.repo.NeedsCheckLines(ctx)
	if err != nil {
		return nil, err
	}
	out := &NeedsCheck{Leaves: []NeedsCheckLeaf{}, Recipes: []NeedsCheckRecipe{}}
	recipeAt := map[int64]int{}
	leafAt := map[int64]int{}
	for _, l := range lines {
		i, ok := recipeAt[l.RecipeID]
		if !ok {
			i = len(out.Recipes)
			recipeAt[l.RecipeID] = i
			out.Recipes = append(out.Recipes, NeedsCheckRecipe{ID: l.RecipeID, Name: l.RecipeName})
		}
		out.Recipes[i].Lines = append(out.Recipes[i].Lines, l)

		j, ok := leafAt[l.LeafID]
		if !ok {
			j = len(out.Leaves)
			leafAt[l.LeafID] = j
			out.Leaves = append(out.Leaves, NeedsCheckLeaf{ID: l.LeafID, Name: l.LeafName})
		}
		leaf := &out.Leaves[j]
		if n := len(leaf.Recipes); n == 0 || leaf.Recipes[n-1].ID != l.RecipeID {
			leaf.Recipes = append(leaf.Recipes, NeedsCheckRecipe{ID: l.RecipeID, Name: l.RecipeName})
		}
	}
	return out, nil
}
