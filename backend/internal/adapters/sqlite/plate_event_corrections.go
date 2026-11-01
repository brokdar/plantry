package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

var _ planning.Corrections = (*PlateEventRepo)(nil)

// UserReplacements are the user's edits of plates dated in [from, to) that
// swapped recipes (planning.Corrections), oldest first, from the newest 500 events.
func (r *PlateEventRepo) UserReplacements(ctx context.Context, from, to time.Time) ([]planning.Replacement, error) {
	rows, err := r.queriesFor(ctx).ListUserPlateUpdates(ctx, sqlcgen.ListUserPlateUpdatesParams{
		Date: from.Format(dateLayout), Date_2: to.Format(dateLayout),
	})
	if err != nil {
		return nil, fmt.Errorf("list user plate updates: %w", err)
	}
	var out []planning.Replacement
	for _, row := range rows {
		date, err := time.Parse(dateLayout, row.Date)
		if err != nil {
			return nil, fmt.Errorf("plate event date: %w", err)
		}
		var before, after plate.SnapshotPlate
		if json.Unmarshal(rawJSON(row.BeforeJson), &before) != nil || json.Unmarshal(rawJSON(row.AfterJson), &after) != nil {
			slog.DebugContext(ctx, "plate event snapshot unreadable", "date", row.Date, "slot_id", row.SlotID)
			continue
		}
		removed, added := foodDiff(before, after)
		if len(removed) == 0 || len(added) == 0 {
			continue
		}
		out = append(out, planning.Replacement{Date: date, SlotID: row.SlotID, Removed: removed, Added: added})
	}
	slices.Reverse(out) // the query is newest first
	return out, nil
}

// foodDiff is the foods only before has and the foods only after has.
func foodDiff(before, after plate.SnapshotPlate) (removed, added []int64) {
	has := func(p plate.SnapshotPlate, id int64) bool {
		return slices.ContainsFunc(p.Components, func(c plate.SnapshotComponent) bool { return c.FoodID == id })
	}
	for _, c := range before.Components {
		if !has(after, c.FoodID) && !slices.Contains(removed, c.FoodID) {
			removed = append(removed, c.FoodID)
		}
	}
	for _, c := range after.Components {
		if !has(before, c.FoodID) && !slices.Contains(added, c.FoodID) {
			added = append(added, c.FoodID)
		}
	}
	return removed, added
}
