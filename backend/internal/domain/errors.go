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

	// Plate feedback errors.
	ErrInvalidFeedbackStatus = errors.New("invalid feedback status")

	// Import errors.
	ErrImportFetchFailed       = errors.New("import fetch failed")
	ErrImportBodyTooLarge      = errors.New("import body too large")
	ErrImportNotHTML           = errors.New("import not html")
	ErrImportNoRecipe          = errors.New("import no recipe found")
	ErrImportLLMFailed         = errors.New("import llm extraction failed")
	ErrImportInvalidResolution = errors.New("import invalid resolution")

	// Profile planning preference errors.
	ErrInvalidDietType          = errors.New("invalid diet type")
	ErrInvalidMeatDays          = errors.New("invalid meat days per week")
	ErrInvalidHouseholdPortions = errors.New("invalid household portions")

	// Availability errors.
	ErrCellHasPlate     = errors.New("cell has a plate")
	ErrPresetRequired   = errors.New("routine requires a preset")
	ErrPresetNotAllowed = errors.New("preset only allowed for routine")
	ErrSlotInactive     = errors.New("slot inactive")
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

	// Store sections (Phase 2 D18).
	ErrInvalidShopSection  = errors.New("unknown store section")
	ErrShopSectionLeafOnly = errors.New("store sections apply to leaf foods only")
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
	{ErrInvalidShopSection, "error.food.invalid_shop_section"},
	{ErrShopSectionLeafOnly, "error.food.shop_section_leaf_only"},
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
	{ErrInvalidFeedbackStatus, "error.feedback.invalid_status"},
	{ErrImportBodyTooLarge, "error.import.body_too_large"},
	{ErrImportNotHTML, "error.import.not_html"},
	{ErrImportNoRecipe, "error.import.no_recipe"},
	{ErrImportFetchFailed, "error.import.fetch_failed"},
	{ErrImportLLMFailed, "error.import.llm_failed"},
	{ErrImportInvalidResolution, "error.import.invalid_resolution"},
	{ErrInvalidDietType, "error.profile.invalid_diet_type"},
	{ErrInvalidMeatDays, "error.profile.invalid_meat_days"},
	{ErrInvalidHouseholdPortions, "error.profile.invalid_household_portions"},
	{ErrCellHasPlate, "error.availability.cell_has_plate"},
	{ErrPresetRequired, "error.availability.preset_required"},
	{ErrPresetNotAllowed, "error.availability.preset_not_allowed"},
	{ErrSlotInactive, "error.availability.slot_inactive"},
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
