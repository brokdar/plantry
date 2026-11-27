package planview

import (
	"context"
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
	var soak, thaw bool
	for _, c := range f.Children {
		n := strings.ToLower(c.ChildName) + " "
		if hasAny(n, driedWords) && hasAny(n, soakLegumes) {
			soak = true
		}
		if hasAny(n, frozenWords) {
			thaw = true
		}
	}
	marinate := false
	for _, in := range f.Instructions {
		if hasAny(strings.ToLower(in.Text), marinateWords) {
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
