package handlers

import (
	"errors"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func toHTTPWithResource(err error, resource string) (int, string) {
	switch {
	// Plate quantity sentinels (more specific than ErrInvalidInput; check first).
	case errors.Is(err, plate.ErrInvalidQuantityShape):
		return http.StatusBadRequest, "error.plate.invalid_quantity_shape"
	case errors.Is(err, plate.ErrInvalidQuantityForComposed):
		return http.StatusBadRequest, "error.plate.invalid_quantity_for_composed"
	case errors.Is(err, plate.ErrInvalidQuantityForLeaf):
		return http.StatusBadRequest, "error.plate.invalid_quantity_for_leaf"
	case errors.Is(err, plate.ErrUnitRequiresPortion):
		return http.StatusBadRequest, "error.plate.unit_requires_portion"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "error.not_found"
	case errors.Is(err, domain.ErrDuplicateName):
		return http.StatusConflict, "error." + resource + ".duplicate_name"
	case errors.Is(err, domain.ErrInUse):
		return http.StatusConflict, "error." + resource + ".in_use"
	case errors.Is(err, domain.ErrSlotUnknown):
		return http.StatusUnprocessableEntity, "error.plate.slot_unknown"
	case errors.Is(err, domain.ErrInvalidDay):
		return http.StatusBadRequest, "error.invalid_body"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "error.invalid_body"
	case errors.Is(err, domain.ErrLookupFailed):
		return http.StatusBadGateway, "error." + resource + ".lookup_failed"
	case errors.Is(err, domain.ErrInvalidMacros):
		return http.StatusBadRequest, "error.profile.invalid_macros"
	case errors.Is(err, domain.ErrAIProviderMissing):
		return http.StatusServiceUnavailable, "error.ai.provider_missing"
	case errors.Is(err, domain.ErrAIStreamInterrupted):
		return http.StatusBadGateway, "error.ai.stream_interrupted"
	case errors.Is(err, domain.ErrInvalidFeedbackStatus):
		return http.StatusUnprocessableEntity, "error.plate.feedback_invalid_status"
	case errors.Is(err, domain.ErrImportBodyTooLarge):
		return http.StatusRequestEntityTooLarge, "error.import.body_too_large"
	case errors.Is(err, domain.ErrImportNotHTML):
		return http.StatusUnsupportedMediaType, "error.import.not_html"
	case errors.Is(err, domain.ErrImportNoRecipe):
		return http.StatusUnprocessableEntity, "error.import.no_recipe"
	case errors.Is(err, domain.ErrImportFetchFailed):
		return http.StatusBadGateway, "error.import.fetch_failed"
	case errors.Is(err, domain.ErrImportLLMFailed):
		return http.StatusBadGateway, "error.import.llm_failed"
	case errors.Is(err, domain.ErrImportInvalidResolution):
		return http.StatusUnprocessableEntity, "error.import.invalid_resolution"
	// Profile planning preferences.
	case errors.Is(err, domain.ErrInvalidDietType):
		return http.StatusBadRequest, "error.profile.invalid_diet_type"
	case errors.Is(err, domain.ErrInvalidMeatDays):
		return http.StatusBadRequest, "error.profile.invalid_meat_days"
	case errors.Is(err, domain.ErrInvalidHouseholdPortions):
		return http.StatusBadRequest, "error.profile.invalid_household_portions"
	// Availability.
	case errors.Is(err, domain.ErrCellHasPlate):
		return http.StatusConflict, "error.availability.cell_has_plate"
	case errors.Is(err, domain.ErrPresetRequired):
		return http.StatusBadRequest, "error.availability.preset_required"
	case errors.Is(err, domain.ErrPresetNotAllowed):
		return http.StatusBadRequest, "error.availability.preset_not_allowed"
	case errors.Is(err, domain.ErrSlotInactive):
		return http.StatusBadRequest, "error.availability.slot_inactive"
	// Plate cells (D10).
	case errors.Is(err, domain.ErrCellUnavailable):
		return http.StatusConflict, "error.plate.cell_unavailable"
	case errors.Is(err, domain.ErrCellOccupied):
		return http.StatusConflict, "error.plate.cell_occupied"
	default:
		return http.StatusInternalServerError, "error.server"
	}
}

func toHTTP(err error) (int, string) {
	return toHTTPWithResource(err, "food")
}
