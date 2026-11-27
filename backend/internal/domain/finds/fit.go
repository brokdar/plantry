package finds

import (
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// mainStyles vary what the autonomous finds of one window and of
// consecutive weeks look for: every one a main for a lunch or dinner cell.
var mainStyles = []string{
	"main dish",
	"one-pot main",
	"noodle or rice main",
	"vegetable main",
	"oven bake main",
	"skillet main",
}

// planningHead is how many of the household's cuisines, best first, the
// planning finds rotate through.
const planningHead = 3

// planningDishes are the n dishes the autonomous finds search for: each a
// different main style of one of the first planningHead cuisines (ordered
// best first), both rotating by week so that consecutive windows ask for
// different ones; meatFree asks for meat-free mains.
func planningDishes(cuisines []string, n int, day time.Time, meatFree bool) []string {
	head := cuisines[:min(len(cuisines), planningHead)]
	week := day.YearDay() / 7
	out := make([]string, 0, n)
	for i := range n {
		dish := head[(week+i)%len(head)]
		if meatFree {
			dish += " meat-free"
		}
		out = append(out, dish+" "+mainStyles[(week+i)%len(mainStyles)])
	}
	return out
}

// meatFreeShare is the weekly meat-day limit up to which planning finds ask
// for meat-free mains: with at most three meat days most lunch and dinner
// mains of a week are meat-free, and the library's meat mains already fill
// the meat days (H5), so a meat find rarely gets a cell.
const meatFreeShare = 3

// wantsMeatFree reports whether the household's planning finds look for
// meat-free mains: an omnivore household with a tight meat-day limit (a
// vegetarian or vegan diet is in the query already).
func wantsMeatFree(h Household) bool {
	return h.Diet == "" && h.MeatDaysMax != nil && *h.MeatDaysMax <= meatFreeShare
}

// nextDish is the dish after dish in the main styles (GATE-4: an autonomous
// find that failed tries another dish); a dish without a style gets the
// second one.
func nextDish(dish string) string {
	for i, st := range mainStyles {
		if cuisine, ok := strings.CutSuffix(dish, " "+st); ok {
			return cuisine + " " + mainStyles[(i+1)%len(mainStyles)]
		}
	}
	return dish + " " + mainStyles[1]
}

// Non-main and main category tags of extracted recipes (schema.org
// recipeCategory, lower case, EN and DE).
var (
	mainTags = []string{"main", "main course", "main dish", "mains", "entree", "entrée", "hauptgericht", "hauptgerichte", "hauptspeise"}
	sideTags = []string{
		"side", "side dish", "sides", "dessert", "desserts", "appetizer", "appetizers", "starter", "snack", "snacks",
		"drink", "drinks", "beverage", "sauce", "sauces", "condiment", "dip", "dips", "bread", "baking",
		"beilage", "beilagen", "nachspeise", "nachtisch", "vorspeise", "vorspeisen", "getränk", "getränke", "soße", "saucen", "brot", "backen",
	}
)

// mainDish reports whether the draft is a main: its role when the check set
// one, else its category tags (a main tag wins over a side tag; no tag
// counts as a main).
func mainDish(d *importer.Draft) bool {
	if d.Role != "" {
		return d.Role == "main"
	}
	tags := make([]string, 0, len(d.Tags))
	for _, t := range d.Tags {
		tags = append(tags, strings.ToLower(strings.TrimSpace(t)))
	}
	if slices.ContainsFunc(tags, func(t string) bool { return slices.Contains(mainTags, t) }) {
		return true
	}
	return !slices.ContainsFunc(tags, func(t string) bool { return slices.Contains(sideTags, t) })
}

// draftMinutes is the draft's total time: prep plus cook, or the stated
// total when that is longer (0: unknown).
func draftMinutes(d *importer.Draft) int {
	n := 0
	for _, m := range []*int{d.PrepMinutes, d.CookMinutes} {
		if m != nil {
			n += *m
		}
	}
	if d.TotalMinutes != nil {
		n = max(n, *d.TotalMinutes)
	}
	return n
}

// Why a ready find cannot fill a lunch or dinner main this window.
const (
	unfitNotMain = "it is not a main dish"
	unfitTooLong = "it takes longer than any meal this week has time for"
)

// cellUnfit says why a draft cannot fill a lunch or dinner main cell (""
// when it can): not a main, or longer than the kitchen's longest time
// allowance (GATE-2 the way the planner reads it: no weekday allowance, no
// limit; unknown minutes and a missing draft fit).
func cellUnfit(d *importer.Draft, k profile.Kitchen) string {
	if d == nil {
		return ""
	}
	if !mainDish(d) {
		return unfitNotMain
	}
	if k.MinutesWeekday > 0 && draftMinutes(d) > max(k.MinutesWeekday, k.MinutesWeekend) {
		return unfitTooLong
	}
	return ""
}

// sameDish normalises a dish name for the "never the same dish" check.
func sameDish(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}
