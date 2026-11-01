package handlers

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// TestSentinelStatus_CoversEveryDomainSentinel counts the Err* sentinels in
// domain/errors.go: each needs an HTTP status, else it would answer 500.
func TestSentinelStatus_CoversEveryDomainSentinel(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../../../domain/errors.go", nil, 0)
	require.NoError(t, err)
	declared := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if spec, ok := n.(*ast.ValueSpec); ok {
			for _, name := range spec.Names {
				if strings.HasPrefix(name.Name, "Err") {
					declared++
				}
			}
		}
		return true
	})
	require.Positive(t, declared)
	assert.Len(t, sentinelStatus, declared)
}

func TestToHTTP_KeysFromDomain(t *testing.T) {
	tests := []struct {
		err      error
		resource string
		status   int
		key      string
	}{
		{fmt.Errorf("%w: %w", domain.ErrInvalidInput, plate.ErrUnitRequiresPortion), "plate", http.StatusBadRequest, "error.plate.unit_requires_portion"},
		{domain.ErrInUse, "slot", http.StatusConflict, "error.slot.in_use"},
		{domain.ErrInvalidPlanStart, "food", http.StatusBadRequest, "error.plan.invalid_start"},
		{domain.ErrNothingToAccept, "plate", http.StatusBadRequest, "error.plan.nothing_to_accept"},
		{domain.ErrStateMismatch, "plate", http.StatusConflict, "error.plan.state_mismatch"},
		{domain.ErrRowConfirmed, "plate", http.StatusConflict, "error.plan.row_confirmed"},
		{domain.ErrAIPastDate, "plate", http.StatusBadRequest, "error.ai.past_date"},
		{domain.ErrAIPreferenceReserved, "profile", http.StatusBadRequest, "error.ai.preference_reserved"},
		{errors.New("boom"), "food", http.StatusInternalServerError, "error.server"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			status, key := toHTTPWithResource(fmt.Errorf("op: %w", tt.err), tt.resource)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.key, key)
		})
	}
}

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
