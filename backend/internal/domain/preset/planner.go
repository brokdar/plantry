package preset

import (
	"sort"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const dateLayout = "2006-01-02"

// PresetSnapshot maps a preset's plates onto target as a plate.PlanSnapshot:
// one plate per preset slot, in sort order, with the household's eaters.
// Composed components carry no session, so each gets a fresh one. A plate
// whose slot is not active is reported in noSlot; a second plate for a slot
// already taken by the preset is reported in duplicate. active nil means
// every slot is active.
func PresetSnapshot(plates []Plate, target time.Time, active map[int64]struct{}) (snap plate.PlanSnapshot, noSlot, duplicate []plate.CellKey) {
	sorted := make([]Plate, len(plates))
	copy(sorted, plates)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].SortOrder < sorted[j].SortOrder })

	snap = plate.PlanSnapshot{Plates: []plate.SnapshotPlate{}, Sessions: []plate.SnapshotSession{}}
	taken := map[int64]bool{}
	for _, pp := range sorted {
		cell := plate.CellKey{Date: target, SlotID: pp.SlotID}
		if !isSlotActive(active, pp.SlotID) {
			noSlot = append(noSlot, cell)
			continue
		}
		if taken[pp.SlotID] {
			duplicate = append(duplicate, cell)
			continue
		}
		taken[pp.SlotID] = true
		comps := make([]plate.SnapshotComponent, len(pp.Components))
		for i, c := range pp.Components {
			comps[i] = plate.SnapshotComponent{
				FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams, SortOrder: i,
			}
		}
		snap.Plates = append(snap.Plates, plate.SnapshotPlate{
			Date: target.Format(dateLayout), SlotID: pp.SlotID, Components: comps,
		})
	}
	return snap, noSlot, duplicate
}

// CopySnapshot maps the plates of the 7-day window from sourceStart onto the
// window shiftDays later as a plate.PlanSnapshot. Every plate keeps its slot,
// eaters, note and components. A session cooking in the source window is
// copied, shifted, with the same portions and note, no frozen portions and
// not cooked, and its components stay linked to the copy. A component whose
// session cooks outside the window gets a fresh session. sessions holds the
// sources' sessions by id. A plate whose slot is not active is reported in
// noSlot at its target cell. active nil means every slot is active.
func CopySnapshot(sources []plate.Plate, sessions map[int64]plate.CookSession, sourceStart time.Time, shiftDays int, active map[int64]struct{}) (snap plate.PlanSnapshot, noSlot []plate.CellKey) {
	sorted := make([]plate.Plate, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Date.Equal(sorted[j].Date) {
			return sorted[i].Date.Before(sorted[j].Date)
		}
		return sorted[i].SlotID < sorted[j].SlotID
	})
	sourceEnd := sourceStart.AddDate(0, 0, 6)
	inWindow := func(d time.Time) bool { return !d.Before(sourceStart) && !d.After(sourceEnd) }
	shift := func(d time.Time) time.Time { return d.AddDate(0, 0, shiftDays) }

	snap = plate.PlanSnapshot{Plates: []plate.SnapshotPlate{}, Sessions: []plate.SnapshotSession{}}
	refs := map[int64]string{}
	for _, src := range sorted {
		target := shift(src.Date)
		if !isSlotActive(active, src.SlotID) {
			noSlot = append(noSlot, plate.CellKey{Date: target, SlotID: src.SlotID})
			continue
		}
		comps := make([]plate.SnapshotComponent, len(src.Components))
		for i, c := range src.Components {
			sc := plate.SnapshotComponent{
				FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams, SortOrder: c.SortOrder,
			}
			if c.CookSessionID != nil {
				if sess, ok := sessions[*c.CookSessionID]; ok && inWindow(sess.CookDate) {
					ref, seen := refs[sess.ID]
					if !seen {
						ref = "s" + strconv.Itoa(len(refs)+1)
						refs[sess.ID] = ref
						snap.Sessions = append(snap.Sessions, plate.SnapshotSession{
							Ref: ref, FoodID: sess.FoodID, CookDate: shift(sess.CookDate).Format(dateLayout), CookSlotID: sess.CookSlotID,
							Portions: sess.Portions, Note: sess.Note,
						})
					}
					sc.Session = &plate.SessionLink{Ref: ref}
				}
			}
			comps[i] = sc
		}
		snap.Plates = append(snap.Plates, plate.SnapshotPlate{
			Date: target.Format(dateLayout), SlotID: src.SlotID, Eaters: src.Eaters, Note: src.Note, Components: comps,
		})
	}
	return snap, noSlot
}
