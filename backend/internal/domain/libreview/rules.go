package libreview

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// maxDataRecipes bounds the recipes one data finding lists.
const maxDataRecipes = 5

// quickMinutes is the longest a quick weekday main takes (prep + cook).
const quickMinutes = 30

// minGapLibrary is the library size from which gaps are queued; a smaller
// library is the cold start (Fill my library).
const minGapLibrary = 5

func tokens(name string) []string {
	f := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	slices.Sort(f)
	return slices.Compact(f)
}

func jaccard[T comparable](a, b []T) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	in := 0
	for _, x := range a {
		if slices.Contains(b, x) {
			in++
		}
	}
	return float64(in) / float64(len(a)+len(b)-in)
}

// similar scores how alike two recipes are, 0 when they are not near
// duplicates: the same words in the name, or nearly the same ingredients.
func similar(a, b Recipe) float64 {
	ta, tb := tokens(a.Name), tokens(b.Name)
	if slices.Equal(ta, tb) {
		return 1
	}
	best := 0.0
	if len(ta) >= 2 && len(tb) >= 2 {
		if j := jaccard(ta, tb); j >= 0.75 {
			best = j
		}
	}
	if len(a.Ingredients) >= 3 && len(b.Ingredients) >= 3 {
		if j := jaccard(a.Ingredients, b.Ingredients); j >= 0.8 {
			best = max(best, j)
		}
	}
	return best
}

// duplicates finds pairs of near-identical recipes that are not variants of
// each other yet, most alike first; a recipe is in one pair.
func duplicates(rs []Recipe) []Finding {
	type pair struct {
		a, b  Recipe
		score float64
	}
	var pairs []pair
	for i, a := range rs {
		for _, b := range rs[i+1:] {
			if a.VariantGroup != 0 && a.VariantGroup == b.VariantGroup {
				continue
			}
			if s := similar(a, b); s > 0 {
				pairs = append(pairs, pair{a, b, s})
			}
		}
	}
	slices.SortStableFunc(pairs, func(x, y pair) int { return cmp.Compare(y.score, x.score) })
	used := map[int64]bool{}
	var out []Finding
	for _, p := range pairs {
		if used[p.a.ID] || used[p.b.ID] {
			continue
		}
		used[p.a.ID], used[p.b.ID] = true, true
		out = append(out, Finding{
			Key: fmt.Sprintf("dup:%d-%d", p.a.ID, p.b.ID), Kind: KindDuplicates,
			Recipes: []RecipeRef{{ID: p.a.ID, Name: p.a.Name}, {ID: p.b.ID, Name: p.b.Name}},
		})
	}
	return out
}

// replaced finds the recipes the user took off a plan ReplacedMin times or
// more, the most often first.
func replaced(rs []Recipe, counts map[int64]int) []Finding {
	var out []Finding
	for _, r := range rs {
		if n := counts[r.ID]; n >= ReplacedMin {
			out = append(out, Finding{Key: fmt.Sprintf("replaced:%d", r.ID), Kind: KindReplaced, Recipes: []RecipeRef{{ID: r.ID, Name: r.Name}}, Count: n})
		}
	}
	slices.SortStableFunc(out, func(x, y Finding) int { return cmp.Compare(y.Count, x.Count) })
	return out
}

// data is one finding listing the recipes that lack times or steps, the
// ones lacking most first.
func data(rs []Recipe) []Finding {
	var list []RecipeRef
	for _, r := range rs {
		var missing []string
		if r.Prep+r.Cook == 0 {
			missing = append(missing, "times")
		}
		if r.Steps == 0 {
			missing = append(missing, "steps")
		}
		if len(missing) > 0 {
			list = append(list, RecipeRef{ID: r.ID, Name: r.Name, Missing: missing})
		}
	}
	if len(list) == 0 {
		return nil
	}
	slices.SortStableFunc(list, func(x, y RecipeRef) int { return cmp.Compare(len(y.Missing), len(x.Missing)) })
	n := len(list)
	list = list[:min(n, maxDataRecipes)]
	return []Finding{{Key: "data", Kind: KindData, Recipes: list, Count: n}}
}

// Pick takes at most MaxFindings findings, one of each list in turn.
func Pick(lists ...[]Finding) []Finding {
	var out []Finding
	for round := 0; len(out) < MaxFindings; round++ {
		added := false
		for _, l := range lists {
			if round < len(l) && len(out) < MaxFindings {
				out = append(out, l[round])
				added = true
			}
		}
		if !added {
			break
		}
	}
	return out
}

// isMain reports whether a recipe is a lunch or dinner main.
func isMain(r Recipe) bool { return r.Role == "main" || r.Role == "standalone" }

// gaps are what the assistant can fill by finding recipes (LIB-3): no quick
// weekday main, and fewer than two mains.
func gaps(rs []Recipe) []Gap {
	if len(rs) < minGapLibrary {
		return nil
	}
	mains, quick := 0, 0
	for _, r := range rs {
		if !isMain(r) {
			continue
		}
		mains++
		if t := r.Prep + r.Cook; t > 0 && t <= quickMinutes {
			quick++
		}
	}
	var out []Gap
	if mains < 2 {
		out = append(out, Gap{Kind: "few_mains", Dish: "a family dinner main course"})
	}
	if quick == 0 {
		out = append(out, Gap{Kind: "quick_main", Dish: "a quick weeknight dinner main course ready in 30 minutes"})
	}
	return out
}
