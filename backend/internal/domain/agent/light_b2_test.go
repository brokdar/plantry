package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// A mark on a meal that is still far over its cap is refused: the mark is
// derived from the result, and the message names the numbers and find_ways
// (audit B2 showed the S6 as new; E006 stops the mark instead).
func TestApplyChanges_LightCallOnAMealOverItsCapIsRefused(t *testing.T) {
	e := newPlanToolEnv(t)
	e.withMacros()
	e.sql(`UPDATE foods SET kcal_100g = 1000 WHERE name = 'Kichererbsen'`)
	ctx, _ := e.chat()
	dinner := e.slotID("dinner")
	var p *plate.Plate
	for _, d := range []int{2, 3, 4} {
		if q := e.plateAtCell(planStart.AddDate(0, 0, d).Format(time.DateOnly), dinner); q != nil && q.State == plate.StateDraft {
			p = q
			break
		}
	}
	require.NotNil(t, p)

	input := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"meal","slot":"dinner"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":1}]}`, p.DateString(), p.ID)
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit")
	assert.Contains(t, err.Error(), "find_ways")
}
