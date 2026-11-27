package main

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// knownLeavesLimit bounds the leaves read for FIND-17's "new to the
// household".
const knownLeavesLimit = 500

// libraryNamesLimit bounds the recipe names an autonomous find checks a
// dish against (newest first).
const libraryNamesLimit = 2000

// findHousehold builds what the find pipeline knows about the household:
// language and diet (the only fields a search may carry), the gates'
// household, the kitchen, never items, leaning taste items and the leaves
// the household cooks with.
func findHousehold(
	prof func(context.Context) (*profile.Profile, error),
	never func(context.Context) ([]kitchen.Food, error),
	tasteView func(context.Context, time.Time) (taste.View, error),
	leaves func(context.Context, int) ([]string, error),
	cuisines func(context.Context) ([]string, error),
	history func(context.Context) (bool, error),
	recipes func(context.Context, int) ([]string, error),
) func(context.Context) (finds.Household, error) {
	return func(ctx context.Context) (finds.Household, error) {
		p, err := prof(ctx)
		if err != nil {
			return finds.Household{}, err
		}
		h := finds.Household{Lang: "en", Kitchen: p.Kitchen, MeatDaysMax: p.MeatDaysPerWeekMax, Portions: max(1, int(p.HouseholdPortions+0.5))}
		if strings.HasPrefix(p.Locale, "de") {
			h.Lang = "de"
		}
		switch p.DietType {
		case profile.DietVegan, profile.DietVegetarian, profile.DietPescetarian:
			h.Diet = string(p.DietType)
			h.Gates.Diet = diet.Class(p.DietType)
		}
		for _, code := range gates.AllergenCodes {
			if slices.ContainsFunc(p.DietaryRestrictions, func(r string) bool { return strings.Contains(strings.ToLower(r), code) }) {
				h.Gates.Allergens = append(h.Gates.Allergens, code)
			}
		}
		items, err := never(ctx)
		if err != nil {
			return finds.Household{}, err
		}
		for _, f := range items {
			h.Gates.NeverFoodIDs = append(h.Gates.NeverFoodIDs, f.ID)
			h.Gates.NeverNames = append(h.Gates.NeverNames, f.Name)
		}
		h.Never = h.Gates.NeverNames
		v, err := tasteView(ctx, time.Time{})
		if err != nil {
			return finds.Household{}, err
		}
		for _, it := range v.Items {
			switch it.Polarity {
			case taste.Love:
				h.Loves = append(h.Loves, it.Key)
			case taste.RatherNot:
				h.RatherNot = append(h.RatherNot, it.Key)
			}
		}
		if h.KnownLeaves, err = leaves(ctx, knownLeavesLimit); err != nil {
			return finds.Household{}, err
		}
		if h.Cuisines, err = cuisines(ctx); err != nil {
			return finds.Household{}, err
		}
		cooked, err := history(ctx)
		if err != nil {
			return finds.Household{}, err
		}
		h.FirstWeek = !cooked
		if h.Recipes, err = recipes(ctx, libraryNamesLimit); err != nil {
			return finds.Household{}, err
		}
		return h, nil
	}
}

// householdCuisines lists the cuisines the household eats (FIND-15), best
// first (TASTE): the ones it loves (learned, stated or the cold-start
// answer), its staples' cuisines, Cuisines to explore, its library's
// cuisines (most recipes first), and the locale's home cuisine last.
func householdCuisines(
	prof func(context.Context) (*profile.Profile, error),
	loved, staples, explore, library func(context.Context) ([]string, error),
) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		var out []string
		for _, src := range []func(context.Context) ([]string, error){loved, staples, explore, library} {
			more, err := src(ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, more...)
		}
		p, err := prof(ctx)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(p.Locale, "de") {
			out = append(out, "german")
		}
		seen := map[string]bool{}
		return slices.DeleteFunc(out, func(c string) bool {
			dup := seen[c]
			seen[c] = true
			return dup
		}), nil
	}
}

// stapleCuisines lists the cuisines of the household's staples (THEME-1),
// the most staples first; neutral is left out.
func stapleCuisines(
	staples func(context.Context, time.Time) (map[int64]bool, error),
	attrs func(context.Context, []int64) (map[int64]flavor.Record, error),
	now func() time.Time,
) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		set, err := staples(ctx, now())
		if err != nil || len(set) == 0 {
			return nil, err
		}
		recs, err := attrs(ctx, slices.Sorted(maps.Keys(set)))
		if err != nil {
			return nil, err
		}
		n := map[string]int{}
		for _, r := range recs {
			if c := r.Values.Cuisine; c != "" && c != "neutral" {
				n[c]++
			}
		}
		out := slices.Collect(maps.Keys(n))
		slices.SortFunc(out, func(a, b string) int {
			return cmp.Or(cmp.Compare(n[b], n[a]), cmp.Compare(a, b))
		})
		return out, nil
	}
}

// aiGroupsExempt reports whether a host belongs to a site whose AI
// user-agent groups are skipped (FIND-6).
func aiGroupsExempt(sites func(context.Context) ([]settings.ChefSite, error)) func(string) bool {
	return func(host string) bool {
		list, err := sites(context.Background())
		if err != nil {
			return false
		}
		host = strings.TrimPrefix(strings.ToLower(host), "www.")
		return slices.ContainsFunc(list, func(s settings.ChefSite) bool {
			return s.AIGroupsExempt && (host == s.Domain || strings.HasSuffix(host, "."+s.Domain))
		})
	}
}

// lazyFinds lets the chat tool set, built before the chef runner, reach the
// find service once it exists.
type lazyFinds struct{ svc *finds.Service }

func (l *lazyFinds) Request(ctx context.Context, dish string, trigger finds.Trigger) (*finds.Find, error) {
	return l.svc.Request(ctx, dish, trigger)
}

// Touch, Stage and Settle make lazyFinds the planner's planning.ReadyFinds
// (C3); the find service is built after the planner.
func (l *lazyFinds) Touch(ctx context.Context) error { return l.svc.Touch(ctx) }

func (l *lazyFinds) Stage(ctx context.Context, n int, windowStart time.Time) ([]int64, []string, error) {
	return l.svc.Stage(ctx, n, windowStart)
}

func (l *lazyFinds) Settle(ctx context.Context, windowStart time.Time, staged []int64, placed func(int64) bool) error {
	return l.svc.Settle(ctx, windowStart, staged, placed)
}
