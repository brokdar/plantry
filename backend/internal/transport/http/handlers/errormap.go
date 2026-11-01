package handlers

import (
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// sentinelStatus is the HTTP status of each domain sentinel. The message keys
// come from domain.MessageKey and plate.MessageKey (the single source the
// agent shares).
var sentinelStatus = map[error]int{
	domain.ErrInvalidShopSection:        http.StatusBadRequest,
	domain.ErrInvalidDietClass:          http.StatusBadRequest,
	domain.ErrShopSectionLeafOnly:       http.StatusBadRequest,
	domain.ErrInvalidPortions:           http.StatusBadRequest,
	domain.ErrNotFound:                  http.StatusNotFound,
	domain.ErrDuplicateName:             http.StatusConflict,
	domain.ErrInUse:                     http.StatusConflict,
	domain.ErrSlotUnknown:               http.StatusUnprocessableEntity,
	domain.ErrInvalidDay:                http.StatusBadRequest,
	domain.ErrInvalidInput:              http.StatusBadRequest,
	domain.ErrLookupFailed:              http.StatusBadGateway,
	domain.ErrInvalidMacros:             http.StatusBadRequest,
	domain.ErrAIProviderMissing:         http.StatusServiceUnavailable,
	domain.ErrAIStreamInterrupted:       http.StatusBadGateway,
	domain.ErrAIProviderFailed:          http.StatusBadGateway,
	domain.ErrAIInvalidPlan:             http.StatusBadGateway,
	domain.ErrAIPastDate:                http.StatusBadRequest,
	domain.ErrAIPreferenceReserved:      http.StatusBadRequest,
	domain.ErrAIUnlockNeedsUser:         http.StatusConflict,
	domain.ErrAIChangeStale:             http.StatusConflict,
	domain.ErrAIChangeUndone:            http.StatusConflict,
	domain.ErrAIChangeNotUndoable:       http.StatusConflict,
	domain.ErrInvalidFeedbackStatus:     http.StatusBadRequest,
	domain.ErrImportBodyTooLarge:        http.StatusRequestEntityTooLarge,
	domain.ErrImportNotHTML:             http.StatusUnsupportedMediaType,
	domain.ErrImportNoRecipe:            http.StatusUnprocessableEntity,
	domain.ErrImportFetchFailed:         http.StatusBadGateway,
	domain.ErrImportLLMFailed:           http.StatusBadGateway,
	domain.ErrImportInvalidResolution:   http.StatusUnprocessableEntity,
	domain.ErrInvalidDietType:           http.StatusBadRequest,
	domain.ErrInvalidMeatDays:           http.StatusBadRequest,
	domain.ErrInvalidHouseholdPortions:  http.StatusBadRequest,
	domain.ErrCellHasPlate:              http.StatusConflict,
	domain.ErrPresetRequired:            http.StatusBadRequest,
	domain.ErrPresetNotAllowed:          http.StatusBadRequest,
	domain.ErrSlotInactive:              http.StatusBadRequest,
	domain.ErrAvailabilitySlotUnknown:   http.StatusBadRequest,
	domain.ErrAvailabilityPresetUnknown: http.StatusBadRequest,
	domain.ErrCellUnavailable:           http.StatusConflict,
	domain.ErrCellOccupied:              http.StatusConflict,
	domain.ErrSessionBeforeCook:         http.StatusConflict,
	domain.ErrSessionPastKeepDays:       http.StatusConflict,
	domain.ErrSessionPastFreezerDays:    http.StatusConflict,
	domain.ErrSessionFoodMismatch:       http.StatusConflict,
	domain.ErrSessionFreezingDisabled:   http.StatusConflict,
	domain.ErrSessionNotFreezable:       http.StatusConflict,
	domain.ErrSessionNotFound:           http.StatusConflict,
	domain.ErrInvalidPlanStart:          http.StatusBadRequest,
	domain.ErrNothingToAccept:           http.StatusBadRequest,
	domain.ErrStateMismatch:             http.StatusConflict,
	domain.ErrRowConfirmed:              http.StatusConflict,
	domain.ErrGenerationRunning:         http.StatusConflict,
	domain.ErrScopeConfirmed:            http.StatusConflict,
	domain.ErrInvalidScope:              http.StatusBadRequest,
	domain.ErrNotEnoughRecipes:          http.StatusBadRequest,
	domain.ErrPlanChanged:               http.StatusConflict,
	domain.ErrConfirmedAffected:         http.StatusConflict,
	domain.ErrReplanPast:                http.StatusBadRequest,
}

func toHTTPWithResource(err error, resource string) (int, string) {
	// Plate quantity sentinels wrap ErrInvalidInput; check them first.
	if key, ok := plate.MessageKey(err); ok {
		return http.StatusBadRequest, key
	}
	key, sentinel := domain.MessageKey(err, resource)
	status, ok := sentinelStatus[sentinel]
	if !ok {
		return http.StatusInternalServerError, "error.server"
	}
	return status, key
}

func toHTTP(err error) (int, string) {
	return toHTTPWithResource(err, "food")
}
