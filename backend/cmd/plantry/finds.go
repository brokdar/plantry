package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// knownLeavesLimit bounds the leaves read for FIND-17's "new to the
// household".
const knownLeavesLimit = 500

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
) func(context.Context) (finds.Household, error) {
	return func(ctx context.Context) (finds.Household, error) {
		p, err := prof(ctx)
		if err != nil {
			return finds.Household{}, err
		}
		h := finds.Household{Lang: "en", Kitchen: p.Kitchen, Portions: max(1, int(p.HouseholdPortions+0.5))}
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
		return h, nil
	}
}

// householdCuisines lists the cuisines the household eats (FIND-15): the ones
// its library's recipes carry, the ones it loves (the cold-start answer
// included), Cuisines to explore, and the locale's home cuisine.
func householdCuisines(
	prof func(context.Context) (*profile.Profile, error),
	library func(context.Context) ([]string, error),
	loved func(context.Context) ([]string, error),
	explore func(context.Context) ([]string, error),
) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		var out []string
		for _, src := range []func(context.Context) ([]string, error){library, loved, explore} {
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
		slices.Sort(out)
		return slices.Compact(out), nil
	}
}

// planningDish picks the cuisine a planning find searches for on day: once
// the household has cooked, today's of Cuisines to explore; in week one
// (COLD-4) or without any, today's of the household's cuisines (FIND-15).
// An empty scope gives "", and the find service then starts none.
func planningDish(
	history func(context.Context) (bool, error),
	house func(context.Context) ([]string, error),
	explore func(context.Context) ([]string, error),
) func(context.Context, time.Time) (string, error) {
	return func(ctx context.Context, day time.Time) (string, error) {
		cooked, err := history(ctx)
		if err != nil {
			return "", err
		}
		var cuisines []string
		if cooked {
			if cuisines, err = explore(ctx); err != nil {
				return "", err
			}
		}
		if len(cuisines) == 0 {
			if cuisines, err = house(ctx); err != nil {
				return "", err
			}
		}
		return discovery.DailyCuisine(cuisines, day), nil
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

func (l *lazyFinds) Stage(ctx context.Context, n int, windowStart time.Time) ([]int64, error) {
	return l.svc.Stage(ctx, n, windowStart)
}

func (l *lazyFinds) Settle(ctx context.Context, windowStart time.Time, staged []int64, placed func(int64) bool) error {
	return l.svc.Settle(ctx, windowStart, staged, placed)
}
