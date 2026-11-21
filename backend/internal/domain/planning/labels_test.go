package planning

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

var ruleToken = regexp.MustCompile(`\b[HS]\d+b?\b`)

// Every issue code has a plain label and sentence, and none of them carries a
// rule code or an issue code.
func TestPlainText_CoversEveryCodeWithoutCodes(t *testing.T) {
	codes := []string{
		CodeMissingPlate, CodeExtraPlate, CodePlateInNoneCell, CodeCookInNoneCell, CodeTooManyCookOccasions,
		CodeDietIncompatible, CodeMeatDaysExceeded, CodeDietUnknown, CodeConfirmedInTarget, CodeSessionOutsideTarget,
		CodeUnknownFood, CodeUnknownSession, CodeNotComposed, CodeSessionFoodMismatch, CodeDuplicateSessionKey,
		CodeInvalidPortions, CodePlateWithoutComponent, CodePlateOutsideTarget, CodePlateOnConfirmedCell,
		CodeVarietyRepeat, CodeVarietyRelaxed, CodeVarietyRepeatWeek, CodeVarietyRepeatNamed, CodeKcalBelowFloor, CodeKcalBelowBand,
		CodeKcalAboveBand, CodeProteinLow, CodeWeekKcalBelowBand, CodeWeekKcalAboveBand, CodeWeekProteinLow,
		CodeWeekCarbsRange, CodeWeekFatRange, CodeMealNotLight, CodeMealNotHearty, CodeDishRepeats, CodeMealOutsideSlotBand, CodeAddOnExcess,
		CodeWeekMeatAboveDGE, CodeWeekFishBelowDGE, CodeWeekLegumesBelow,
		"leftover_before_cook", "past_keep_days", "deficit", "frozen_deficit",
	}
	for _, c := range codes {
		tx, ok := issueTexts[c]
		assert.True(t, ok, "%s has a text", c)
		assert.NotEmpty(t, tx.Label, c)
		assert.NotEmpty(t, tx.Message, c)
		assert.NotRegexp(t, ruleToken, tx.Label+" "+tx.Message, c)
		assert.NotContains(t, tx.Label+tx.Message, "_", c)
	}
}

func TestDescribePlain_NoRuleCodes(t *testing.T) {
	p := PolicyOf(Profile{KcalTarget: f64p(2000), WeightKg: f64p(70)})
	plain := p.DescribePlain()
	assert.NotRegexp(t, ruleToken, plain)
	for _, code := range []string{"dish_repeats_exceeded", "week_meat_above_dge", "kcal_below_floor"} {
		assert.NotContains(t, plain, code)
	}
	assert.Regexp(t, ruleToken, p.Describe(), "the planner's text keeps its rule codes")
	assert.Contains(t, plain, "weekly energy")
}

func f64p(v float64) *float64 { return &v }
