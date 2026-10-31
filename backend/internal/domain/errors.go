package domain

import "errors"

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
