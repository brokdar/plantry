package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkOut struct {
	Report struct {
		Check struct {
			Before []checkDay `json:"before_days"`
			After  []checkDay `json:"after_days"`
		} `json:"check"`
	} `json:"report"`
}

type checkDay struct {
	Date   string `json:"date"`
	Plates []struct {
		PlateID int64   `json:"plate_id"`
		Eaters  float64 `json:"eaters"`
	} `json:"plates"`
	Light []struct {
		Scope string `json:"scope"`
		Slot  string `json:"slot"`
	} `json:"light"`
}

func (c checkOut) day(days []checkDay, date string) *checkDay {
	for i := range days {
		if days[i].Date == date {
			return &days[i]
		}
	}
	return nil
}

// The after snapshot holds the call's own light marks, which are saved with the write.
func TestReportCheck_AfterDaysHoldTheLightTheCallSaves(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	p, _ := e.twoDinners()
	raw := e.raw("apply_changes", fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":1}]}`, p.DateString(), p.ID))
	var out checkOut
	require.NoError(t, json.Unmarshal(raw, &out))
	day := out.day(out.Report.Check.After, p.DateString())
	require.NotNil(t, day, "the touched day is in the snapshot")
	require.Len(t, day.Light, 1)
	assert.Equal(t, "day", day.Light[0].Scope)
	before := out.day(out.Report.Check.Before, p.DateString())
	require.NotNil(t, before)
	assert.Empty(t, before.Light, "the mark was not there before")
}

// A guests write lists its day in both snapshots with the eaters before and after.
func TestReportCheck_SetEatersListsItsDay(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	p, _ := e.twoDinners()
	raw := e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":%v}]}`, p.ID, p.Eaters+1))
	var out checkOut
	require.NoError(t, json.Unmarshal(raw, &out))
	b, a := out.day(out.Report.Check.Before, p.DateString()), out.day(out.Report.Check.After, p.DateString())
	require.NotNil(t, b, "before_days lists the day of the plate")
	require.NotNil(t, a, "after_days lists the day of the plate")
	find := func(d *checkDay) float64 {
		for _, pl := range d.Plates {
			if pl.PlateID == p.ID {
				return pl.Eaters
			}
		}
		return -1
	}
	assert.Equal(t, p.Eaters, find(b))
	assert.Equal(t, p.Eaters+1, find(a))
}
