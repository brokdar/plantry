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
)
