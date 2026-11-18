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

// The light request of the call itself is not part of the baseline: a dinner
// marked light that is still far over its cap shows S6 as a new issue (audit B2).
func TestApplyChanges_LightCallShowsItsOwnS6AsNew(t *testing.T) {
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
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))
	require.NoError(t, err)
	var res struct {
		After struct {
			Issues struct {
				New []struct {
					Rule string `json:"rule"`
					Code string `json:"code"`
					Date string `json:"date"`
				} `json:"new"`
			} `json:"issues"`
		} `json:"after"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	var codes []string
	for _, is := range res.After.Issues.New {
		if is.Date == p.DateString() {
			codes = append(codes, is.Rule+"."+is.Code)
		}
	}
	assert.Contains(t, codes, "S6.meal_not_light")
}
