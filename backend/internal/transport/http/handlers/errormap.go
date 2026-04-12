package handlers

import (
	"errors"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

func toHTTP(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "error.not_found"
	case errors.Is(err, domain.ErrDuplicateName):
		return http.StatusConflict, "error.ingredient.duplicate_name"
	case errors.Is(err, domain.ErrInUse):
		return http.StatusConflict, "error.ingredient.in_use"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "error.invalid_body"
	default:
		return http.StatusInternalServerError, "error.server"
	}
}
