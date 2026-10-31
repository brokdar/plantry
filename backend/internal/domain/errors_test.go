package domain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageKeys_CoverEverySentinel parses errors.go and checks that every
// exported Err* sentinel has an entry in the messageKeys table, so a new
// sentinel cannot silently fall through to error.server.
func TestMessageKeys_CoverEverySentinel(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	require.NoError(t, err)

	var declared, mapped []string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, name := range spec.Names {
			if strings.HasPrefix(name.Name, "Err") {
				declared = append(declared, name.Name)
			}
		}
		if len(spec.Names) == 1 && spec.Names[0].Name == "messageKeys" {
			ast.Inspect(spec, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && strings.HasPrefix(id.Name, "Err") {
					mapped = append(mapped, id.Name)
				}
				return true
			})
		}
		return true
	})
	sort.Strings(declared)
	sort.Strings(mapped)
	require.NotEmpty(t, declared)
	assert.Equal(t, declared, mapped)
}

func TestMessageKey(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		resource string
		want     string
		sentinel error
	}{
		{"session", ErrSessionPastKeepDays, "plate", "error.session.past_keep_days", ErrSessionPastKeepDays},
		{"plan start", ErrInvalidPlanStart, "plan", "error.plan.invalid_start", ErrInvalidPlanStart},
		{"wrapped", fmt.Errorf("component[0]: %w", ErrSessionNotFound), "plate", "error.session.not_found", ErrSessionNotFound},
		{"resource scoped", ErrDuplicateName, "slot", "error.slot.duplicate_name", ErrDuplicateName},
		{"invalid day before invalid input", fmt.Errorf("%w: %w", ErrInvalidInput, ErrInvalidDay), "food", "error.invalid_body", ErrInvalidDay},
		{"specific before invalid input", fmt.Errorf("%w: %w", ErrInvalidInput, ErrInvalidPortions), "food", "error.food.invalid_portions", ErrInvalidPortions},
		{"unknown", fmt.Errorf("boom"), "food", "error.server", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, sentinel := MessageKey(tt.err, tt.resource)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.sentinel, sentinel)
		})
	}
}
