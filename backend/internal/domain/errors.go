package domain

import (
	"errors"
	"strings"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateName = errors.New("duplicate name")
	ErrInUse         = errors.New("in use")
	ErrInvalidInput  = errors.New("invalid input")
	ErrLookupFailed  = errors.New("lookup failed")
	ErrInvalidDay    = errors.New("invalid day")
	ErrSlotUnknown   = errors.New("slot unknown")
	ErrInvalidMacros = errors.New("invalid macros")

	// ErrInvalidPortions: a scaled recipe needs 0 < portions ≤ 100.
	ErrInvalidPortions = errors.New("invalid portions")

	// AI / agent errors.
	ErrAIProviderMissing   = errors.New("ai provider not configured")
	ErrAIStreamInterrupted = errors.New("ai stream interrupted")
	// ErrAIProviderFailed: the provider call failed (network, API error,
	// timeout). ErrAIInvalidPlan: no reply of a generation held a readable
	// submit_plan call (D11).
	ErrAIProviderFailed = errors.New("ai provider error")
	ErrAIInvalidPlan    = errors.New("ai plan unreadable")
	// ErrAIRefused: the model declined the request (stop reason refusal).
	ErrAIRefused = errors.New("ai refused")
	// ErrAIRateLimited: the provider answered 429 or 529. ErrAIKeyRejected: it
	// answered 401 or 403.
	ErrAIRateLimited = errors.New("ai provider rate limited")
	ErrAIKeyRejected = errors.New("ai provider rejected the key")
	// ErrAIPastDate: the agent never writes a plate, session or cell dated
	// before the chat's today (Phase 4 D7). ErrAIPreferenceReserved: the
	// agent never records likes/dislikes, which plate feedback owns (D9).
	ErrAIPastDate           = errors.New("the assistant cannot change days before today")
	ErrAIPreferenceReserved = errors.New("preference key is reserved")
	// ErrAIUnlockNeedsUser: the agent confirmed an unlock without a preview
	// the user answered in a later message (Phase 4 D6).
	ErrAIUnlockNeedsUser = errors.New("unlock needs the user's yes in a later message")
	// ErrAIChangeStale: a row of an agent change was written again after it,
	// so undoing the change would lose that write. ErrAIChangeUndone: the
	// change was undone already (Phase 4 D13).
	ErrAIChangeStale  = errors.New("the change was edited afterwards")
	ErrAIChangeUndone = errors.New("the change was undone already")
	// ErrAIChangeNotUndoable: the change wrote nothing undo can put back.
	ErrAIChangeNotUndoable = errors.New("the change has nothing to undo")
	// ErrAIChangeConfirmed: a plate or session of the change is confirmed
	// now, so undo would rewrite a decision (Z1).
	ErrAIChangeConfirmed = errors.New("the change's rows are confirmed")

	// Plate feedback errors.
	ErrInvalidFeedbackStatus = errors.New("invalid feedback status")

	// Import errors.
	ErrImportFetchFailed       = errors.New("import fetch failed")
	ErrImportInvalidURL        = errors.New("import invalid url")
	ErrImportNotFound          = errors.New("import page not found")
	ErrImportBlocked           = errors.New("import page blocked")
	ErrImportTimeout           = errors.New("import fetch timed out")
	ErrImportUnreachable       = errors.New("import host unreachable")
	ErrImportNoRecipeNoAI      = errors.New("import no recipe found and no ai provider")
	ErrImportBodyTooLarge      = errors.New("import body too large")
	ErrImportNotHTML           = errors.New("import not html")
	ErrImportNoRecipe          = errors.New("import no recipe found")
	ErrImportLLMFailed         = errors.New("import llm extraction failed")
	ErrImportInvalidResolution = errors.New("import invalid resolution")
	// ErrImportUnmatchedLine: a reviewed line has no food (P3, MATCH-5).
	ErrImportUnmatchedLine = errors.New("import line not matched")
	// ErrImportNoNutrition: a line's food is not a leaf with nutrition per
	// 100 g and the line is not "to taste" (P3).
	ErrImportNoNutrition = errors.New("import food without nutrition")

	// Profile planning preference errors.
	ErrInvalidDietType          = errors.New("invalid diet type")
	ErrInvalidMeatDays          = errors.New("invalid meat days per week")
	ErrInvalidHouseholdPortions = errors.New("invalid household portions")
	ErrInvalidGoal              = errors.New("invalid goal")
	ErrInvalidBodyData          = errors.New("invalid body data")
	ErrInvalidKitchen           = errors.New("invalid kitchen data")
	ErrKcalBelowFloor           = errors.New("kcal target below the safety floor")

	// Availability errors.
	ErrCellHasPlate     = errors.New("cell has a plate")
	ErrPresetRequired   = errors.New("routine requires a preset")
	ErrPresetNotAllowed = errors.New("preset only allowed for routine")
	ErrSlotInactive     = errors.New("slot inactive")
	// ErrInvalidSlotBands: the slot energy bands of the active slots do not add
	// up (minimums over 100, maximums under 100), or a band is not 0-100.
	ErrInvalidSlotBands = errors.New("invalid slot bands")
	// ErrAvailabilitySlotUnknown and ErrAvailabilityPresetUnknown name a
	// slot or preset in a request body that does not exist (400, not 404).
	ErrAvailabilitySlotUnknown   = errors.New("availability slot unknown")
	ErrAvailabilityPresetUnknown = errors.New("availability preset unknown")

	// Plate cell errors (D10).
	ErrCellUnavailable = errors.New("cell is marked no plate")
	ErrCellOccupied    = errors.New("cell already has a plate")

	// Cook session errors (Phase 2 D13, D14). All are conflicts (409).
	ErrSessionBeforeCook       = errors.New("cell is before the session's cook cell")
	ErrSessionPastKeepDays     = errors.New("cell is past the session's use-by date")
	ErrSessionPastFreezerDays  = errors.New("cell is past the session's freezer keep days")
	ErrSessionFoodMismatch     = errors.New("food differs from the session's food")
	ErrSessionFreezingDisabled = errors.New("freezing is disabled")
	ErrSessionNotFreezable     = errors.New("recipe is not freezable")
	// ErrSessionNotFound is a session a component asks to link to that does
	// not exist (409, unlike a missing session resource, which is 404).
	ErrSessionNotFound = errors.New("linked cook session not found")

	// ErrInvalidPlanStart: a plan window must start on the shopping day
	// (Phase 2 D24).
	ErrInvalidPlanStart = errors.New("plan window must start on the shopping day")

	// Plan state (Phase 3 D4): accept or discard of a window without drafts,
	// and unaccept of rows that are not (or no longer) confirmed.
	ErrNothingToAccept = errors.New("no draft rows in the window")
	ErrStateMismatch   = errors.New("rows are not confirmed")
	// ErrRowConfirmed: the agent wrote to a confirmed plate or session, or
	// to a draft linked to one (Phase 4 D5). It changes drafts only.
	ErrRowConfirmed = errors.New("row is confirmed")

	// Generation (Phase 3 B6, D6, D9): one run per window, a scope that names
	// a confirmed row or nothing valid, and a library below the onboarding
	// target.
	ErrGenerationRunning = errors.New("a generation runs for this window")
	ErrScopeConfirmed    = errors.New("the scope names a confirmed row")
	ErrInvalidScope      = errors.New("invalid generation scope")
	ErrNotEnoughRecipes  = errors.New("not enough recipes to plan")
	// ErrPlanChanged is a run whose save found the plan changed while it
	// chose (a replaced draft confirmed or gone, a target cell taken).
	ErrPlanChanged = errors.New("the plan changed while it was generated")

	// Discovery (D7, D11): one discovery run at a time, a decision
	// on a suggestion that is decided already, research output that stayed
	// unreadable after the repair call, an unknown mode and an explore run
	// without a cuisine.
	ErrDiscoveryRunning         = errors.New("a discovery run is running")
	ErrDiscoveryDecided         = errors.New("the suggestion is decided already")
	ErrDiscoveryInvalidOutput   = errors.New("the research output is not valid JSON")
	ErrDiscoveryInvalidMode     = errors.New("unknown discovery mode")
	ErrDiscoveryCuisineRequired = errors.New("an explore discovery needs a cuisine")

	// Mid-week re-plan (Phase 3 B8): confirmed plates in the No-plate cells
	// need unlock_confirmed, and a re-plan starts today at the earliest.
	ErrConfirmedAffected = errors.New("the re-plan removes confirmed plates")
	ErrReplanPast        = errors.New("a re-plan cannot start in the past")
	// ErrPastCell: a meal operation (SetAway) names a cell before today.
	ErrPastCell = errors.New("the cell lies in the past")
	// ErrPlanChangeStale: a hand edit's undo token names rows (or a cell)
	// written again after it, so reverting it would lose that write.
	ErrPlanChangeStale = errors.New("the change was edited afterwards")

	// Store sections (Phase 2 D18).
	ErrInvalidShopSection  = errors.New("unknown store section")
	ErrShopSectionLeafOnly = errors.New("store sections apply to leaf foods only")

	// ErrInvalidDietClass is a diet class override outside the four classes
	// (Phase 3 D28).
	ErrInvalidDietClass = errors.New("unknown diet class")

	// ErrInvalidServing is a serving size that is not a positive number.
	ErrInvalidServing = errors.New("serving size must be positive")

	// ErrServingLeafOnly is a serving size on a recipe.
	ErrServingLeafOnly = errors.New("serving sizes apply to ingredients only")
)

// resourcePlaceholder in a message key is replaced by the caller's resource
// name (e.g. "error.{resource}.in_use" → "error.slot.in_use").
const resourcePlaceholder = "{resource}"

// messageKeys maps every sentinel to its stable i18n message key. It is the
// single source for the HTTP error mapping and the agent's tool errors. Order
// matters: errors often wrap ErrInvalidInput together with a more specific
// sentinel, so the specific ones come first.
var messageKeys = []struct {
	err error
	key string
}{
	// It wraps the plate error that found the change.
	{ErrPlanChanged, "error.plan.changed_during_run"},
	{ErrInvalidShopSection, "error.food.invalid_shop_section"},
	{ErrShopSectionLeafOnly, "error.food.shop_section_leaf_only"},
	{ErrInvalidDietClass, "error.food.invalid_diet_class"},
	{ErrInvalidServing, "error.food.invalid_serving"},
	{ErrServingLeafOnly, "error.food.serving_leaf_only"},
	{ErrInvalidPortions, "error.food.invalid_portions"},
	{ErrNotFound, "error.not_found"},
	{ErrDuplicateName, "error.{resource}.duplicate_name"},
	{ErrInUse, "error.{resource}.in_use"},
	{ErrSlotUnknown, "error.plate.slot_unknown"},
	{ErrInvalidDay, "error.invalid_body"},
	{ErrInvalidInput, "error.invalid_body"},
	{ErrLookupFailed, "error.{resource}.lookup_failed"},
	{ErrInvalidMacros, "error.profile.invalid_macros"},
	{ErrAIProviderMissing, "error.ai.provider_missing"},
	{ErrAIStreamInterrupted, "error.ai.stream_interrupted"},
	{ErrAIProviderFailed, "error.ai.provider_error"},
	{ErrAIInvalidPlan, "error.ai.invalid_plan"},
	{ErrAIRefused, "error.ai.refused"},
	{ErrAIRateLimited, "error.ai.rate_limited"},
	{ErrAIKeyRejected, "error.ai.key_rejected"},
	{ErrAIPastDate, "error.ai.past_date"},
	{ErrAIPreferenceReserved, "error.ai.preference_reserved"},
	{ErrAIUnlockNeedsUser, "error.ai.unlock_needs_user"},
	{ErrAIChangeStale, "error.ai.change_stale"},
	{ErrAIChangeUndone, "error.ai.change_undone"},
	{ErrAIChangeNotUndoable, "error.ai.change_not_undoable"},
	{ErrAIChangeConfirmed, "error.ai.change_confirmed"},
	{ErrInvalidFeedbackStatus, "error.feedback.invalid_status"},
	{ErrImportBodyTooLarge, "error.import.body_too_large"},
	{ErrImportNotHTML, "error.import.not_html"},
	{ErrImportNoRecipe, "error.import.no_recipe"},
	{ErrImportFetchFailed, "error.import.fetch_failed"},
	{ErrImportInvalidURL, "error.import.invalid_url"},
	{ErrImportNotFound, "error.import.not_found"},
	{ErrImportBlocked, "error.import.blocked"},
	{ErrImportTimeout, "error.import.timeout"},
	{ErrImportUnreachable, "error.import.unreachable"},
	{ErrImportNoRecipeNoAI, "error.import.no_recipe_no_ai"},
	{ErrImportLLMFailed, "error.import.llm_failed"},
	{ErrImportInvalidResolution, "error.import.invalid_resolution"},
	{ErrImportUnmatchedLine, "error.import.unmatched_line"},
	{ErrImportNoNutrition, "error.import.no_nutrition"},
	{ErrInvalidDietType, "error.profile.invalid_diet_type"},
	{ErrInvalidMeatDays, "error.profile.invalid_meat_days"},
	{ErrInvalidHouseholdPortions, "error.profile.invalid_household_portions"},
	{ErrInvalidGoal, "error.profile.invalid_goal"},
	{ErrInvalidBodyData, "error.profile.invalid_body_data"},
	{ErrInvalidKitchen, "error.profile.invalid_kitchen"},
	{ErrKcalBelowFloor, "error.profile.kcal_below_floor"},
	{ErrCellHasPlate, "error.availability.cell_has_plate"},
	{ErrPresetRequired, "error.availability.preset_required"},
	{ErrPresetNotAllowed, "error.availability.preset_not_allowed"},
	{ErrSlotInactive, "error.availability.slot_inactive"},
	{ErrInvalidSlotBands, "error.slot.invalid_bands"},
	{ErrAvailabilitySlotUnknown, "error.availability.slot_unknown"},
	{ErrAvailabilityPresetUnknown, "error.availability.preset_unknown"},
	{ErrCellUnavailable, "error.plate.cell_unavailable"},
	{ErrCellOccupied, "error.plate.cell_occupied"},
	{ErrSessionBeforeCook, "error.session.before_cook"},
	{ErrSessionPastKeepDays, "error.session.past_keep_days"},
	{ErrSessionPastFreezerDays, "error.session.past_freezer_days"},
	{ErrSessionFoodMismatch, "error.session.food_mismatch"},
	{ErrSessionFreezingDisabled, "error.session.freezing_disabled"},
	{ErrSessionNotFreezable, "error.session.not_freezable"},
	{ErrSessionNotFound, "error.session.not_found"},
	{ErrInvalidPlanStart, "error.plan.invalid_start"},
	{ErrNothingToAccept, "error.plan.nothing_to_accept"},
	{ErrStateMismatch, "error.plan.state_mismatch"},
	{ErrRowConfirmed, "error.plan.row_confirmed"},
	{ErrGenerationRunning, "error.plan.generation_running"},
	{ErrScopeConfirmed, "error.plan.scope_confirmed"},
	{ErrInvalidScope, "error.plan.invalid_scope"},
	{ErrNotEnoughRecipes, "error.plan.not_enough_recipes"},
	{ErrDiscoveryRunning, "error.discovery.running"},
	{ErrDiscoveryDecided, "error.discovery.decided"},
	{ErrDiscoveryInvalidOutput, "error.discovery.invalid_output"},
	{ErrDiscoveryInvalidMode, "error.discovery.invalid_mode"},
	{ErrDiscoveryCuisineRequired, "error.discovery.cuisine_required"},
	{ErrConfirmedAffected, "error.plan.confirmed_affected"},
	{ErrReplanPast, "error.plan.replan_past"},
	{ErrPastCell, "error.plan.past_cell"},
	{ErrPlanChangeStale, "error.plan.change_stale"},
}

// MessageKey returns the i18n message key of the first sentinel err wraps,
// and that sentinel. resource fills the resource-scoped keys (duplicate_name,
// in_use, lookup_failed). An error that wraps no sentinel yields ("error.server", nil).
func MessageKey(err error, resource string) (key string, sentinel error) {
	for _, m := range messageKeys {
		if errors.Is(err, m.err) {
			return strings.Replace(m.key, resourcePlaceholder, resource, 1), m.err
		}
	}
	return "error.server", nil
}
