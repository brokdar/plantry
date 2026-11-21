package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// KR7 (v1.2): a day replan that ends with the same main at both meals saves
// nothing only where H8 is triggered. With a library of one main H8 is relaxed
// (variety_relaxed), so the same dish twice is saved.
func TestKR7_SameMainTwiceIsSavedWhereVarietyIsRelaxed(t *testing.T) {
	e := newSvcEnv(t)
	e.exec(`DELETE FROM foods WHERE name IN ('Vollkornbrot', 'Gouda Käse', 'Gurke')`)
	for _, id := range e.mains[1:] {
		e.exec(`UPDATE foods SET role = 'side_veg' WHERE id = ?`, id)
	}
	date := "2026-11-10"
	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(date)})

	end := e.runs.end(id)

	assert.NotEqual(t, airun.StatusError, end.Status, "notes %q key %q", end.Notes, end.ErrorKey)
}
