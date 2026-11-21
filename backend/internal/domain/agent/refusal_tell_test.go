package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// E036b: a refused light mark carries its own short tell.
func TestRefusedLightMark_HasATell(t *testing.T) {
	t.Parallel()
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
	tell := agent.RefusalTell(err)
	require.Len(t, tell, 2)
	assert.Contains(t, tell[0], "saved nothing")
	assert.Contains(t, tell[0], "more than a light meal")
	assert.Contains(t, tell[1], "I can make it smaller first")
}
