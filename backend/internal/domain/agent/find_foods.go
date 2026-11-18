package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

const findFoodsSchema = `{
  "type":"object",
  "properties":{
    "ids":{"type":"array","items":{"type":"integer","minimum":1},"maxItems":100,"description":"Look these foods up by id (the other filters apply to them too; unknown ids are listed in missing_ids)."},
    "roles":{"type":"array","items":{"type":"string","enum":["main","side_starch","side_veg","side_protein","sauce","drink","dessert","standalone"]}},
    "kinds":{"type":"array","items":{"type":"string","enum":["leaf","composed"]}},
    "no_cook":{"type":"boolean","description":"true: only foods that need no cooking (leaves eaten raw such as fruit, vegetables, yoghurt; recipes whose cook minutes are 0)."},
    "diet":{"type":"string","enum":["vegan","vegetarian","pescetarian","omnivore"],"description":"Keep foods whose diet class fits this diet (vegetarian keeps vegan and vegetarian); unknown classes are left out."},
    "tags":{"type":"array","items":{"type":"string"},"description":"Foods with every one of these tags."},
    "search":{"type":"string","description":"Words matched against names and tags, ignoring case and umlauts, with German/English synonyms (potato finds Kartoffelgratin)."},
    "max_kcal":{"type":"number","minimum":0,"description":"At most this many kcal per portion (leaf: per typical portion)."},
    "min_protein":{"type":"number","minimum":0,"description":"At least this many g protein per portion (leaf: per typical portion)."},
    "max_minutes":{"type":"integer","minimum":1,"description":"Only recipes whose total minutes (prep + cook) are at most this. Recipes with unknown minutes and all leaves are excluded."},
    "sort":{"type":"string","enum":["name","last_eaten","kcal","protein","minutes"],"description":"last_eaten lists never-eaten recipes first, then the longest uneaten. kcal and protein sort descending unless sort_desc is false. minutes sorts ascending by total minutes (unknown last) unless sort_desc is true."},
    "sort_desc":{"type":"boolean"},
    "limit":{"type":"integer","minimum":1,"maximum":200,"description":"Default 50."}
  },
  "additionalProperties":false
}`

const findFoodsDescription = `Find foods in the library with filters, one call for any number of lookups (roles, kinds, ids, tags, no_cook, diet, search, kcal and protein bounds, max_minutes). A food is a LEAF (single item with direct nutrition: apple, rice, chicken) or COMPOSED (a recipe built from child foods). Returns JSON {items:[{id, name, kind, role (a leaf's role is its effective role: the user's override, else derived from its store section: produce is side_veg, bakery side_starch, meat and fish side_protein, otherwise empty), diet_class (vegan|vegetarian|pescetarian|omnivore|unknown), counts_as_cooking, no_cook, prep_minutes?, cook_minutes?, total_minutes?, per_portion:{kcal,protein,fat,carbs}, basis, portion_g?, per_100g?:{kcal,protein,fat,carbs}, nutrition_complete, missing_nutrition?:[names], keep_days?, freezable?, last_eaten?, times_cooked?, tags, favorite}], total, truncated, missing_ids?:[ids not found]}. A food whose nutrition cannot be resolved is still returned with nutrition_complete false and zero values. Composed: per_portion is per recipe portion, keep_days (days leftovers keep), last_eaten (YYYY-MM-DD or null) and times_cooked are set. Leaf: per_portion is for a typical portion of portion_g grams, per_100g the nutrition per 100 g, and a leaf is added to a plate by amount + unit. counts_as_cooking is true for a recipe (adding it to a plate makes or joins a cooking occasion) and false for a leaf; no_cook is true for foods that need no cooking (the user's setting, else derived). Recipes carry prep_minutes, cook_minutes and total_minutes; a zero or unknown value is left out, and max_minutes and sort minutes ignore recipes without a total. roles[] matches a leaf by its effective role. total counts the matches before limit.`

type foodsFilter struct {
	IDs        []int64  `json:"ids"`
	Roles      []string `json:"roles"`
	Kinds      []string `json:"kinds"`
	NoCook     *bool    `json:"no_cook"`
	Diet       string   `json:"diet"`
	Tags       []string `json:"tags"`
	Search     string   `json:"search"`
	MaxKcal    *float64 `json:"max_kcal"`
	MinProtein *float64 `json:"min_protein"`
	MaxMinutes *int     `json:"max_minutes"`
	Sort       string   `json:"sort"`
	SortDesc   *bool    `json:"sort_desc"`
	Limit      int      `json:"limit"`
}

func toolFindFoods(svc Services) Tool {
	return Tool{
		Name:        "find_foods",
		Description: findFoodsDescription,
		Schema:      json.RawMessage(findFoodsSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in foodsFilter
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			today := todayFrom(ctx)
			foods, missing, err := candidateFoods(ctx, svc, in, today)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := svc.Foods.Derive(ctx, today, foods...); err != nil {
				return nil, ToolEffectNone, err
			}
			keep, err := defaultKeepDays(ctx, svc)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			foods = slices.DeleteFunc(foods, func(f *food.Food) bool { return !matchesFilter(f, in) })
			// Macros decide the filter or the order only for kcal/protein bounds
			// and sorts; otherwise they are resolved for the returned page alone.
			early := in.MaxKcal != nil || in.MinProtein != nil || in.Sort == "kcal" || in.Sort == "protein"
			var macros map[int64]food.Detailed
			if early {
				if macros, err = resolveMacros(ctx, svc, foods); err != nil {
					return nil, ToolEffectNone, err
				}
			}
			items := make([]foodItem, 0, len(foods))
			byID := make(map[int64]*food.Food, len(foods))
			for _, f := range foods {
				byID[f.ID] = f
				it := newFoodItem(f, macros[f.ID], keep)
				if early && ((in.MaxKcal != nil && it.PerPortion.Kcal > *in.MaxKcal) || (in.MinProtein != nil && it.PerPortion.Protein < *in.MinProtein)) {
					continue
				}
				items = append(items, it)
			}
			sortItems(items, in)
			limit := in.Limit
			if limit <= 0 {
				limit = 50
			}
			total := len(items)
			if len(items) > limit {
				items = items[:limit]
			}
			if !early {
				page := make([]*food.Food, len(items))
				for i, it := range items {
					page[i] = byID[it.ID]
				}
				if macros, err = resolveMacros(ctx, svc, page); err != nil {
					return nil, ToolEffectNone, err
				}
				for i, f := range page {
					items[i] = newFoodItem(f, macros[f.ID], keep)
				}
			}
			out := map[string]any{"items": items, "total": total, "truncated": total > len(items)}
			if len(missing) > 0 {
				out["missing_ids"] = missing
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

// resolveMacros resolves per-portion nutrition for foods. A food whose
// nutrition cannot be resolved (cyclic or too deep recipe) comes back as an
// incomplete zero entry so one bad recipe never sinks the call; only a
// cancelled context fails.
func resolveMacros(ctx context.Context, svc Services, foods []*food.Food) (map[int64]food.Detailed, error) {
	ids := make([]int64, len(foods))
	for i, f := range foods {
		ids[i] = f.ID
	}
	macros, err := svc.NutritionResolver.PerPortionDetailedMany(ctx, ids)
	if err == nil {
		return macros, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	macros = make(map[int64]food.Detailed, len(ids))
	for _, id := range ids {
		d, err := svc.NutritionResolver.PerPortionDetailed(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			slog.WarnContext(ctx, "find_foods nutrition failed", "food_id", id, "error", err)
			d = food.Detailed{}
		}
		macros[id] = d
	}
	return macros, nil
}

// candidateFoods reads the foods the filter may keep: the ids, or the library
// narrowed by what the repository filters (one kind, diet).
func candidateFoods(ctx context.Context, svc Services, in foodsFilter, today time.Time) (foods []*food.Food, missing []int64, err error) {
	if len(in.IDs) > 0 {
		out := make([]*food.Food, 0, len(in.IDs))
		for _, id := range in.IDs {
			f, err := svc.Foods.Get(ctx, id)
			if errors.Is(err, domain.ErrNotFound) {
				missing = append(missing, id)
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			if in.Diet != "" && !diet.Compatible(f.DietClass, diet.Class(in.Diet)) {
				continue
			}
			out = append(out, f)
		}
		if s := strings.TrimSpace(in.Search); s != "" {
			res := &food.ListResult{Items: make([]food.Food, len(out))}
			for i, f := range out {
				res.Items[i] = *f
			}
			res = searchPage(res, s, 10000, 0)
			out = out[:0]
			for i := range res.Items {
				out = append(out, &res.Items[i])
			}
		}
		return out, missing, nil
	}
	q := food.ListQuery{Today: today, Diet: diet.Class(in.Diet), Limit: 10000}
	if len(in.Kinds) == 1 {
		q.Kind = food.Kind(in.Kinds[0])
	}
	res, err := svc.Foods.List(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	if s := strings.TrimSpace(in.Search); s != "" {
		res = searchPage(res, s, 10000, 0)
	}
	out := make([]*food.Food, len(res.Items))
	for i := range res.Items {
		out[i] = &res.Items[i]
	}
	return out, nil, nil
}

func matchesFilter(f *food.Food, in foodsFilter) bool {
	if len(in.Kinds) > 0 && !slices.Contains(in.Kinds, string(f.Kind)) {
		return false
	}
	if len(in.Roles) > 0 {
		role, _ := shopping.EffectiveRole(f)
		if role == nil || !slices.Contains(in.Roles, string(*role)) {
			return false
		}
	}
	if in.MaxMinutes != nil {
		total := totalMinutes(f)
		if total == 0 || total > *in.MaxMinutes {
			return false
		}
	}
	if in.NoCook != nil && planning.NoCook(f) != *in.NoCook {
		return false
	}
	for _, t := range in.Tags {
		if !slices.Contains(f.Tags, t) {
			return false
		}
	}
	return true
}

// totalMinutes is a recipe's prep + cook minutes; 0 means unknown (a leaf,
// or a recipe with neither set).
func totalMinutes(f *food.Food) int {
	if f.Kind != food.KindComposed {
		return 0
	}
	total := 0
	if f.PrepMinutes != nil {
		total += *f.PrepMinutes
	}
	if f.CookMinutes != nil {
		total += *f.CookMinutes
	}
	return total
}

type macroOut struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
}

type foodItem struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Kind             string    `json:"kind"`
	Role             string    `json:"role"`
	DietClass        string    `json:"diet_class"`
	CountsAsCooking  bool      `json:"counts_as_cooking"`
	NoCook           bool      `json:"no_cook"`
	PrepMinutes      int       `json:"prep_minutes,omitempty"`
	CookMinutes      int       `json:"cook_minutes,omitempty"`
	TotalMinutes     int       `json:"total_minutes,omitempty"`
	PerPortion       macroOut  `json:"per_portion"`
	Basis            string    `json:"basis"`
	PortionG         *float64  `json:"portion_g,omitempty"`
	Per100g          *macroOut `json:"per_100g,omitempty"`
	NutritionOK      bool      `json:"nutrition_complete"`
	Missing          []string  `json:"missing_nutrition,omitempty"`
	KeepDays         *int      `json:"keep_days,omitempty"`
	Freezable        *bool     `json:"freezable,omitempty"`
	LastEaten        *string   `json:"last_eaten"`
	TimesCooked      *int      `json:"times_cooked,omitempty"`
	Tags             []string  `json:"tags"`
	Favorite         bool      `json:"favorite"`
	lastEatenSortKey string
}

func newFoodItem(f *food.Food, d food.Detailed, defaultKeep int) foodItem {
	it := foodItem{
		ID: f.ID, Name: f.Name, Kind: string(f.Kind), DietClass: string(f.DietClass), CountsAsCooking: f.Kind == food.KindComposed,
		NoCook: planning.NoCook(f), NutritionOK: d.Complete, Tags: f.Tags, Favorite: f.Favorite, Basis: "per_portion",
	}
	if it.Tags == nil {
		it.Tags = []string{}
	}
	if role, _ := shopping.EffectiveRole(f); role != nil {
		it.Role = string(*role)
	}
	if f.Kind == food.KindComposed {
		if f.PrepMinutes != nil {
			it.PrepMinutes = *f.PrepMinutes
		}
		if f.CookMinutes != nil {
			it.CookMinutes = *f.CookMinutes
		}
		it.TotalMinutes = it.PrepMinutes + it.CookMinutes
	}
	for _, m := range d.Missing {
		it.Missing = append(it.Missing, m.Name)
	}
	m := macroOut{Kcal: round1(d.Macros.Kcal), Protein: round1(d.Macros.Protein), Fat: round1(d.Macros.Fat), Carbs: round1(d.Macros.Carbs)}
	it.PerPortion = m
	if f.Kind == food.KindLeaf {
		g := float64(planning.TypicalLeafGrams)
		it.Basis, it.PortionG, it.Per100g = "per_100g", &g, &m
		k := g / 100
		it.PerPortion = macroOut{Kcal: round1(m.Kcal * k), Protein: round1(m.Protein * k), Fat: round1(m.Fat * k), Carbs: round1(m.Carbs * k)}
		return it
	}
	keep := plateKeep(f, defaultKeep)
	it.KeepDays, it.Freezable = &keep, f.Freezable
	times := f.History.TimesCooked
	it.TimesCooked = &times
	if f.History.LastEaten != nil {
		s := f.History.LastEaten.Format(time.DateOnly)
		it.LastEaten, it.lastEatenSortKey = &s, s
	}
	return it
}

func plateKeep(f *food.Food, defaultKeep int) int {
	if f.KeepDays != nil {
		return *f.KeepDays
	}
	return defaultKeep
}

func sortItems(items []foodItem, in foodsFilter) {
	desc := func(def bool) bool {
		if in.SortDesc != nil {
			return *in.SortDesc
		}
		return def
	}
	var less func(a, b foodItem) int
	switch in.Sort {
	case "kcal":
		less = func(a, b foodItem) int { return cmp.Compare(a.PerPortion.Kcal, b.PerPortion.Kcal) }
		if desc(true) {
			slices.SortStableFunc(items, func(a, b foodItem) int { return -less(a, b) })
			return
		}
	case "protein":
		less = func(a, b foodItem) int { return cmp.Compare(a.PerPortion.Protein, b.PerPortion.Protein) }
		if desc(true) {
			slices.SortStableFunc(items, func(a, b foodItem) int { return -less(a, b) })
			return
		}
	case "minutes":
		// Unknown totals (0) sort after every known one.
		less = func(a, b foodItem) int {
			ka, kb := a.TotalMinutes, b.TotalMinutes
			if ka == 0 {
				ka = math.MaxInt
			}
			if kb == 0 {
				kb = math.MaxInt
			}
			return cmp.Compare(ka, kb)
		}
	case "last_eaten":
		// Never eaten first, then the longest uneaten.
		less = func(a, b foodItem) int { return cmp.Compare(a.lastEatenSortKey, b.lastEatenSortKey) }
	default:
		less = func(a, b foodItem) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }
	}
	if desc(false) {
		slices.SortStableFunc(items, func(a, b foodItem) int { return -less(a, b) })
		return
	}
	slices.SortStableFunc(items, less)
}
