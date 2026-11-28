package planning

// IssueText is how an issue reads to the chat model and the user: a short
// plain label for its rule and one plain sentence. Rule codes (H2, S6b) and
// issue codes are internal; the chat never shows them.
type IssueText struct {
	Label   string
	Message string
}

// issueTexts is keyed by issue code (a code belongs to one rule).
var issueTexts = map[string]IssueText{
	CodeMissingPlate:          {"missing plate", "A meal slot that needs a plate has none."},
	CodeExtraPlate:            {"extra plate", "A plate sits in a slot that should stay empty."},
	CodePlateInNoneCell:       {"plate in a no-plate slot", "A plate sits in a slot the user marked as no plate."},
	CodeCookInNoneCell:        {"cooking in a no-plate slot", "Something is cooked in a slot the user marked as no plate."},
	CodeTooManyCookOccasions:  {"cooking times per day", "The day has more separate cooking times than allowed."},
	CodeDietIncompatible:      {"diet", "A food does not fit the user's diet."},
	CodeDietUnknown:           {"diet check", "A food has no known diet class, so it cannot be checked against the diet."},
	CodeMeatDaysExceeded:      {"meat days per week", "More days of the week have meat than the user's limit."},
	CodeConfirmedInTarget:     {"confirmed meals", "The change would touch a confirmed meal."},
	CodeSessionOutsideTarget:  {"batch outside the change", "A batch cooked outside the changed range is affected."},
	CodeUnknownFood:           {"unknown food", "A food in the plan does not exist."},
	CodeUnknownSession:        {"unknown batch", "A plate eats from a batch that does not exist."},
	CodeNotComposed:           {"not a recipe", "A food that is not a recipe is used as one."},
	CodeSessionFoodMismatch:   {"batch of another recipe", "A plate eats from a batch of a different recipe."},
	CodeDuplicateSessionKey:   {"duplicate batch", "Two batches share one name."},
	CodeInvalidPortions:       {"portions", "A portion amount is not valid."},
	CodePlateWithoutComponent: {"empty plate", "A plate has no food on it."},
	CodePlateOutsideTarget:    {"plate outside the range", "A plate lies outside the range being planned."},
	CodePlateOnConfirmedCell:  {"plate on a confirmed slot", "A plate sits on a slot that is already confirmed."},
	CodeVarietyRepeat:         {"dish variety", "A main was cooked or eaten too recently before."},
	CodeVarietyRelaxed:        {"dish variety", "A main repeats a recent one, which is allowed because too few recipes exist."},
	CodeVarietyRepeatWeek:     {"dish repeated this week", "A main is cooked more than once in the same week."},
	CodeTrialRepeat:           {"recipe on trial", "A recipe on trial is cooked more than once in the same week."},
	CodeVarietyRepeatNamed:    {"dish repeated", "A dish you named was cooked or eaten recently before; it is in the plan as you asked."},
	CodeKcalBelowFloor:        {"day energy minimum", "The day is below its minimum energy, which is never acceptable."},
	CodeKcalBelowBand:         {"day energy", "The day's energy is below its target range."},
	CodeKcalAboveBand:         {"day energy", "The day's energy is above its target range."},
	CodeProteinLow:            {"day protein", "The day's protein is below its target."},
	CodeWeekKcalBelowBand:     {"weekly energy", "The week's average energy is below its target range."},
	CodeWeekKcalAboveBand:     {"weekly energy", "The week's average energy is above its target range."},
	CodeWeekProteinLow:        {"weekly protein", "The week's average protein is below its target."},
	CodeWeekCarbsRange:        {"weekly carbs", "The week's carbohydrate share is outside its range."},
	CodeWeekFatRange:          {"weekly fat", "The week's fat share is outside its range."},
	CodeMealNotLight:          {"light meal", "A meal the user wants light holds more than a light meal should."},
	CodeMealNotHearty:         {"filling meal", "A meal the user wants filling holds less than a filling meal needs."},
	CodeMealOutsideSlotBand:   {"meal outside its share of the day", "A meal holds less or more of the day's energy than its time slot is meant to."},
	CodeDishRepeats:           {"dish as main too often", "One dish is the main of more meals than the user's limit."},
	CodeAddOnExcess:           {"sides", "A side is over its usual serving or repeated too often."},
	CodeWeekMeatAboveDGE:      {"weekly meat", "The week holds more meat than the recommended maximum."},
	CodeWeekFishBelowDGE:      {"weekly fish", "The week holds no fish meal."},
	CodeWeekFishAboveDGE:      {"weekly fish", "The week holds more than two fish meals."},
	CodeWeekLegumesBelow:      {"weekly legumes", "The week holds no legume meal."},
	// H3/H4 reuse the session warning codes of plate.
	"leftover_before_cook": {"leftover before cooking", "A leftover is eaten before its batch is cooked."},
	"past_keep_days":       {"leftover too old", "A leftover is eaten after its batch stopped keeping."},
	"deficit":              {"batch portions", "A batch is eaten more than it was cooked for."},
	"frozen_deficit":       {"frozen portions", "A frozen batch has fewer portions than are eaten from it."},
}

// PlainText is the label and sentence of an issue code; an unknown code reads
// as a generic planning rule so no code leaks.
func PlainText(code string) IssueText {
	if t, ok := issueTexts[code]; ok {
		return t
	}
	return IssueText{"planning rule", "A planning rule is not met."}
}
