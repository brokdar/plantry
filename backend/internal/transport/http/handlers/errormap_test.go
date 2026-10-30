package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

func TestToHTTP_SessionErrors(t *testing.T) {
	tests := []struct {
		err error
		key string
	}{
		{domain.ErrSessionBeforeCook, "error.session.before_cook"},
		{domain.ErrSessionPastKeepDays, "error.session.past_keep_days"},
		{domain.ErrSessionPastFreezerDays, "error.session.past_freezer_days"},
		{domain.ErrSessionFoodMismatch, "error.session.food_mismatch"},
		{domain.ErrSessionFreezingDisabled, "error.session.freezing_disabled"},
		{domain.ErrSessionNotFreezable, "error.session.not_freezable"},
		{domain.ErrSessionNotFound, "error.session.not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			status, key := plateError(fmt.Errorf("component[0]: %w: session 7", tt.err))
			assert.Equal(t, http.StatusConflict, status)
			assert.Equal(t, tt.key, key)
		})
	}
}
