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
	default:
		return http.StatusInternalServerError, "error.server"
	}
}

func toHTTP(err error) (int, string) {
	return toHTTPWithResource(err, "ingredient")
}
