package planview

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Chef prep reminder kinds (PRO-1): computed by rule, never by a model.
const (
	ReminderSoak     = "soak"
	ReminderMarinate = "marinate"
	ReminderThaw     = "thaw"
)

// PrepRecipes loads the full recipes (ingredients and steps) of ids; it
// returns nil while the Chef is off, so Today shows no prep line.
type PrepRecipes func(ctx context.Context, ids []int64) (map[int64]*food.Food, error)

// Name and step words of each prep kind, English and German, lower case.
var (
	driedWords    = []string{"dried", "dry ", "getrocknet"}
	soakLegumes   = []string{"chickpea", "bean", "kichererbse", "bohne", "erbse", "peas"}
	marinateWords = []string{"marinate", "marinade", "marinier"}
	frozenWords   = []string{"frozen", "tiefgekühlt", "tiefgefroren", "gefroren", "tk-", "tk "}
)

func hasAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// PrepKind is the night-before step a recipe needs: soak for a dried
// legume that needs soaking (lentils do not), marinate for a marinating
// step, thaw for a frozen ingredient; "" for none. One kind per recipe, in
// that order.
func PrepKind(f *food.Food) string {
	names := make([]string, len(f.Children))
	for i, c := range f.Children {
		names[i] = c.ChildName
	}
	steps := make([]string, len(f.Instructions))
	for i, in := range f.Instructions {
		steps[i] = in.Text
	}
	return PrepKindOf(names, steps)
}

// PrepKindOf is PrepKind for a recipe given as ingredient names and step
// texts (a find is not a food yet).
func PrepKindOf(ingredients, steps []string) string {
	var soak, thaw bool
	for _, name := range ingredients {
		n := strings.ToLower(name) + " "
		if hasAny(n, driedWords) && hasAny(n, soakLegumes) {
			soak = true
		}
		if hasAny(n, frozenWords) {
			thaw = true
		}
	}
	marinate := false
	for _, st := range steps {
		if hasAny(strings.ToLower(st), marinateWords) {
			marinate = true
		}
	}
	switch {
	case soak:
		return ReminderSoak
	case marinate:
		return ReminderMarinate
	case thaw:
		return ReminderThaw
	}
	return ""
}

// prepReminder is the one Chef line Today may show (PRO-2): the first
// session cooked tomorrow, in cell order, whose recipe needs a step tonight.
// sessions are sorted by cook cell.
func prepReminder(sessions []plate.SessionView, tomorrow time.Time, recipes map[int64]*food.Food) (Reminder, bool) {
	for _, v := range sessions {
		f, ok := recipes[v.FoodID]
		if !ok || !v.CookDate.Equal(tomorrow) {
			continue
		}
		if k := PrepKind(f); k != "" {
			c := v.CookCell()
			return Reminder{Kind: k, SessionID: v.ID, FoodName: v.FoodName, Date: c.Date, SlotID: c.SlotID}, true
		}
	}
	return Reminder{}, false
}

// PrepAhead says how far ahead a recipe needs its prep step: Kind as
// PrepKind; Hours is the lead time a step states ("marinate for at least 4
// hours" is 4), 0 for the night before ("overnight", 12 hours or more, or no
// time stated). Derived, never stored.
type PrepAhead struct {
	Kind  string `json:"kind"`
	Hours int    `json:"hours,omitempty"`
}

var (
	hoursRe     = regexp.MustCompile(`(\d+)(?:\s*[-–]\s*(\d+))?\s*(?:hours?|hrs?|h\b|stunden|std)`)
	minutesRe   = regexp.MustCompile(`\d+\s*(?:minutes?|mins?|minuten)`)
	overnightRe = regexp.MustCompile(`overnight|over night|night before|über nacht|nacht vorher`)
)

// prepSameDayMaxHours: a stated lead time above this is the night before.
const prepSameDayMaxHours = 8

// PrepAheadOf is the prep-ahead mark of a recipe given as ingredient names
// and step texts; nil when it needs none. A marinade that states only
// minutes needs no mark.
func PrepAheadOf(ingredients, steps []string) *PrepAhead {
	kind := PrepKindOf(ingredients, steps)
	if kind == "" {
		return nil
	}
	words := marinateWords
	if kind == ReminderSoak {
		words = []string{"soak", "einweich", "einlegen"}
	}
	if kind == ReminderThaw {
		return &PrepAhead{Kind: kind}
	}
	hours, minutesOnly := 0, false
	for _, st := range steps {
		low := strings.ToLower(st)
		if !hasAny(low, words) {
			continue
		}
		if overnightRe.MatchString(low) {
			return &PrepAhead{Kind: kind}
		}
		if m := hoursRe.FindStringSubmatch(low); m != nil {
			h, _ := strconv.Atoi(m[1])
			if m[2] != "" {
				h, _ = strconv.Atoi(m[2])
			}
			if h > 0 && h <= prepSameDayMaxHours && (hours == 0 || h > hours) {
				hours = h
			} else if h > prepSameDayMaxHours {
				return &PrepAhead{Kind: kind}
			}
			continue
		}
		if minutesRe.MatchString(low) {
			minutesOnly = true
		}
	}
	if hours == 0 && minutesOnly && kind == ReminderMarinate {
		return nil
	}
	return &PrepAhead{Kind: kind, Hours: hours}
}

// PrepAheadFood is PrepAheadOf for a recipe food.
func PrepAheadFood(f *food.Food) *PrepAhead {
	names := make([]string, len(f.Children))
	for i, c := range f.Children {
		names[i] = c.ChildName
	}
	steps := make([]string, len(f.Instructions))
	for i, in := range f.Instructions {
		steps[i] = in.Text
	}
	return PrepAheadOf(names, steps)
}
