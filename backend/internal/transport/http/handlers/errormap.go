package handlers

import (
	"errors"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

func toHTTPWithResource(err error, resource string) (int, string) {
	switch {
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
	default:
		return http.StatusInternalServerError, "error.server"
	}
}

func toHTTP(err error) (int, string) {
	return toHTTPWithResource(err, "ingredient")
}
