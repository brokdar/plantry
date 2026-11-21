package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
)

// E030: the final reply passes a rewriter for unambiguous planner terms. The
// sentences are the replies of the paid rounds 4 to 6.
func TestPlainReply(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, in, want string }{
		{"percent band", "Average energy is 102% of target (was 100%), within the 95–105% band.", "Average energy is 102% of target (was 100%), within the 95–105% range."},
		{"RP3: batch, eaters and light day are kitchen words", "The batch feeds 4 eaters on the light day.", "The cooking feeds 4 people on the day you eat less."},
		{"band of the week", "average energy is 99% of target, within the 95–105% band.", "average energy is 99% of target, within the 95–105% range."},
		{"allowed band", "The allowed band is 95-105% for the week.", "The allowed range is 95-105% for the week."},
		{"kept: allowed band of a choir", "The allowed band size is five.", "The allowed band size is five."},
		{"its band", "That puts Tuesday over its band.", "That puts Tuesday over its range."},
		{"energy band", "Thursday is outside the energy band.", "Thursday is outside the energy range."},
		{"number floor", "The minimum (1650) is met, but the day is 79 kcal short of the 1815 floor.", "The minimum (1650) is met, but the day is 79 kcal short of the 1815 limit."},
		{"energy floor", "It stays above the energy floor.", "It stays above the minimum."},
		{"hearty minimum", "It now meets the hearty minimum.", "It now meets the minimum for a filling meal."},
		{"light cap", "It is over the light cap.", "It is over the limit for a light meal."},
		{"cooking occasion", "so no new cooking occasion is added. Two cooking occasions that day.", "so no new cooking time is added. Two cooking times that day."},
		{"add-on", "The bread is over the add-on limit; add-ons are capped.", "The bread is over the limit for sides; sides are capped."},
		{"issues", "No soft issues and no hard issues.", "No warnings and no problems."},
		{"the tool", "I didn't mark it as light because the tool only allows that once the limit is met.", "I didn't mark it as light because it only allows that once the limit is met."},
		{"food id", "The Herb omelette (food_id 188) is a vegetarian main.", "The Herb omelette is a vegetarian main."},
		{"id in text", "Plate plate_id 34 is lighter.", "Plate it is lighter."},
		{"iso date", "Moved to 2026-11-26 dinner.", "Moved to Thu 26 Nov dinner."},
		{"german", "Ciabatta überschreitet die Zusatzgrenze von 120 g.", "Ciabatta überschreitet die Grenze für Beilagen von 120 g."},
		{"required rule", "This is a warning, not a required rule, and the day stays above its minimum of 1650.", "This is a warning, not a must, and the day stays above its minimum of 1650."},
		{"required-rule problem", "It is a warning, not a required-rule breach, for Tue.", "It is a warning, not a must, for Tue."},
		{"article", "Shall I look for an add-on? An add-on limit applies.", "Shall I look for a side? A limit for sides applies."},
		{"german plural", "Die Zusatz-Warnungen bleiben offen.", "Die Warnungen zu Beilagen bleiben offen."},
		{"kept: tool in the kitchen", "Use the tool for peeling.", "Use the tool for peeling."},
		{"kept: a band", "She plays in the band.", "She plays in the band."},
		{"kept: an occasion", "A special cooking occasion.", "A special cooking occasion."},
		{"kept: baking rules", "The hard rules of baking.", "The hard rules of baking."},
		{"kept: a soft item", "Add a soft item such as butter.", "Add a soft item such as butter."},
		{"kept: a quoted soft cheese", "Dinner on Tue has \"Soft add-on cheese\" and 800 kcal; the add-on is over its limit.", "Dinner on Tue has \"Soft add-on cheese\" and 800 kcal; the side is over its limit."},
		{"kept: a food called floor lamp cake", "A band of onions, floor plan, hard cheese and soft pretzel stay.", "A band of onions, floor plan, hard cheese and soft pretzel stay."},
	} {
		assert.Equal(t, c.want, agent.PlainReply(c.in), c.name)
	}
}
