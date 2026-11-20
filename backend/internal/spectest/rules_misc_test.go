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

// SF3: a dish the user names is applied even though it repeats inside the
// variety window; the reply carries a variety warning; nothing is refused.
func TestSF3_NamedDishAppliedWithVarietyWarning(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "chat-"+h, chatWeek)
			s := e.st()
			_, dinner := mealPlates(s)
			// a main eaten in the weeks before the window (a hard H8 repeat)
			var dish planning.FoodFacts
			for _, f := range s.Foods {
				if f.Kind == food.KindComposed && f.Role == food.RoleMain && f.LastEaten != nil && f.NutritionComplete && (dish.ID == 0 || f.ID < dish.ID) {
					dish = f
				}
			}
			if dish.ID == 0 || len(dinner) == 0 {
				t.Skip("no recently eaten main in this household")
			}
			var comp int64
			var plateID int64
			for _, p := range dinner {
				for _, c := range p.Components {
					if f := s.Foods[c.FoodID]; f.Role == food.RoleMain && c.FoodID != dish.ID {
						comp, plateID = c.ID, p.ID
					}
				}
				if comp != 0 {
					break
				}
			}
			require.NotZero(t, comp)
			before := e.violations()
			rs, chs := e.turn("Put "+dish.Name+" on that dinner", tc{"apply_changes", map[string]any{"ops": []any{
				map[string]any{"op": "replace", "plate_component_id": comp, "food_id": dish.ID, "portions": 1.0},
			}}})
			require.Len(t, rs, 1)
			assert.False(t, rs[0].Err, "refused: %s", rs[0].Raw)
			assert.NotEmpty(t, chs, "the named dish was not saved (plate %d, %s)", plateID, dish.Name)
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
