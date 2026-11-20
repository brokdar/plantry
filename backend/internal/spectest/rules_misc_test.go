//go:build spectest

package spectest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// namedDishCase finds a replacement where only the variety rule (H8) is at
// stake: the replaced main is the single-use dish of its plate (a session of its
// own, cooked at that plate's cell), so a fresh session at the same cell keeps
// the cooking times, leftovers and portions as they are; the new main was eaten
// inside the variety window (a hard H8 repeat), has the same diet class and at
// least the energy of the old one (the day cannot fall under its minimum).
func namedDishCase(s planning.State) (plateID, compID int64, dish planning.FoodFacts, ok bool) {
	uses := map[int64]int{}
	for _, p := range s.Plates {
		for _, c := range p.Components {
			if c.SessionID != 0 {
				uses[c.SessionID]++
			}
		}
	}
	sess := map[int64]planning.Session{}
	for _, x := range s.Sessions {
		sess[x.ID] = x
	}
	span := s.Window.Start.AddDate(0, 0, -7*s.Settings.VarietyWeeks)
	lunch, dinner := mealPlates(s)
	for _, p := range append(dinner, lunch...) {
		for _, c := range p.Components {
			old := s.Foods[c.FoodID]
			x := sess[c.SessionID]
			if old.Kind != food.KindComposed || old.Role != food.RoleMain || uses[c.SessionID] != 1 || x.CookDate != p.Date || x.CookSlotID != p.SlotID {
				continue
			}
			for _, f := range s.Foods {
				if f.Kind == food.KindComposed && f.Role == food.RoleMain && f.ID != old.ID && f.NutritionComplete && f.LastEaten != nil &&
					!f.LastEaten.Before(span) && f.DietClass == old.DietClass && f.Macros.Kcal >= old.Macros.Kcal && (dish.ID == 0 || f.ID < dish.ID) {
					dish = f
				}
			}
			if dish.ID != 0 {
				return p.ID, c.ID, dish, true
			}
		}
	}
	return 0, 0, dish, false
}

// SF3: a dish the user names is applied even though it repeats inside the
// variety window; the reply carries a variety warning; nothing is refused.
func TestSF3_NamedDishAppliedWithVarietyWarning(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "chat-"+h, chatWeek)
			plateID, comp, dish, ok := namedDishCase(e.st())
			if !ok {
				t.Skip("no replacement in this household where only the variety rule is at stake")
			}
			before := e.violations()
			rs, chs := e.turn("Put "+dish.Name+" on that dinner", tc{"apply_changes", map[string]any{"ops": []any{
				map[string]any{"op": "replace", "plate_component_id": comp, "food_id": dish.ID, "portions": 1.0},
			}}})
			require.Len(t, rs, 1)
			assert.False(t, rs[0].Err, "refused: %s", rs[0].Raw)
			assert.NotEmpty(t, chs, "the named dish was not saved (plate %d, %s): %.300s", plateID, dish.Name, rs[0].Raw)
			assert.Regexp(t, regexp.MustCompile(`(?i)variety|repeat|recently|wiederhol|kürzlich`), rs[0].Raw, "no variety warning in the result")
			for k := range e.violations() {
				assert.True(t, before[k], "new hard issue %s", k)
			}
		})
	}
}

// CC7 / NU: the priority order is 5 food-group rules, 6 carb and fat ranges,
// 7 preferences and effort.
func TestNU_PriorityOrderTextFollowsResearch(t *testing.T) {
	txt := planning.PolicyOf(planning.Profile{}).Describe()
	assert.Regexp(t, `5 food-group rules[^;]*, 6 carb and fat[^,]*, 7 preferences and effort`, txt)
}

func renderPrompt(t *testing.T) string {
	e := newEnv(t, "chat-dev", chatWeek)
	s := e.st()
	return planning.RenderPrompt(planning.ChooseInput{State: s, Target: planning.WindowTarget(s), Locale: "en"})
}

// NU6: with a g/kg protein target the profile's protein percent is not shown.
func TestNU6_NoProteinPercentWhenGramsPerKgApply(t *testing.T) {
	txt := renderPrompt(t)
	assert.NotRegexp(t, `protein target: \d+% of kcal`, txt)
}

// NU3: the meat-day limit appears once (structured), not again as prose in the
// profile's notes.
func TestNU3_MeatLimitStatedOnce(t *testing.T) {
	txt := strings.ToLower(renderPrompt(t))
	assert.Equal(t, 1, strings.Count(txt, "meat days per week at most")+strings.Count(txt, "meat at most"), "the meat limit is stated more than once")
}

// RP2: a model reply that ends on a question loses the question.
func TestRP2_ReplyDoesNotEndOnAQuestion(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	e.final = "I moved the dinner to Friday. Do you want me to change anything else?"
	e.turn("move the dinner", tc{"apply_changes", `{"ops":[{"op":"set_eaters","plate_id":28,"eaters":2}],"dry_run":true}`})
	require.NotEmpty(t, e.text)
	assert.False(t, strings.HasSuffix(strings.TrimSpace(e.text), "?"), "reply ends on a question: %q", e.text)
}

// RP3: planner words do not reach the household.
func TestRP3_PlainWords(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	e.final = "The batch for the eaters is a light day with a hearty minimum, the cap is soft."
	e.turn("hi", tc{"get_plan", `{}`})
	require.NotEmpty(t, e.text)
	for _, w := range []string{"batch", "eaters", "light day", "hearty minimum", " cap ", "soft"} {
		t.Run(strings.TrimSpace(w), func(t *testing.T) {
			assert.NotContains(t, strings.ToLower(e.text), w, "banned word in %q", e.text)
		})
	}
}

// RP4: the model's text holds no number that is not in the block or a tool result.
func TestRP4_NoUnbackedNumbers(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	e.final = "Dinner now holds 9123 kcal and the week is at 777 percent."
	e.turn("show", tc{"get_plan", `{}`})
	require.NotEmpty(t, e.text)
	assert.NotContains(t, e.text, "9123")
	assert.NotContains(t, e.text, "777")
}
