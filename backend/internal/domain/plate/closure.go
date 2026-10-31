package plate

import "slices"

// RowSet names plates and cook sessions by id, each list ascending.
type RowSet struct {
	PlateIDs   []int64
	SessionIDs []int64
}

// Empty reports whether the set names no row.
func (r RowSet) Empty() bool { return len(r.PlateIDs) == 0 && len(r.SessionIDs) == 0 }

// Closure returns the batch closure of the seeds (D3): every plate and cook
// session reachable from them through component → session links, the seeds
// included. It walks only the links of plates: a session reaches the plates
// that eat from it, a plate the sessions its components eat from. The caller
// loads every plate linked to a session it may reach; a session no loaded
// plate links to is reached only as a seed. Pure.
func Closure(plates []Plate, seedPlates, seedSessions []int64) RowSet {
	byID := make(map[int64]*Plate, len(plates))
	eaters := map[int64][]int64{} // session → plates eating from it
	for i := range plates {
		p := &plates[i]
		byID[p.ID] = p
		for _, sid := range linkedSessions(p.Components) {
			eaters[sid] = append(eaters[sid], p.ID)
		}
	}
	seenPlate := map[int64]bool{}
	seenSession := map[int64]bool{}
	var plateQueue, sessionQueue []int64
	addPlate := func(id int64) {
		if !seenPlate[id] {
			seenPlate[id] = true
			plateQueue = append(plateQueue, id)
		}
	}
	addSession := func(id int64) {
		if !seenSession[id] {
			seenSession[id] = true
			sessionQueue = append(sessionQueue, id)
		}
	}
	for _, id := range seedPlates {
		addPlate(id)
	}
	for _, id := range seedSessions {
		addSession(id)
	}
	for len(plateQueue) > 0 || len(sessionQueue) > 0 {
		if n := len(plateQueue); n > 0 {
			id := plateQueue[n-1]
			plateQueue = plateQueue[:n-1]
			if p, ok := byID[id]; ok {
				for _, sid := range linkedSessions(p.Components) {
					addSession(sid)
				}
			}
			continue
		}
		n := len(sessionQueue)
		sid := sessionQueue[n-1]
		sessionQueue = sessionQueue[:n-1]
		for _, pid := range eaters[sid] {
			addPlate(pid)
		}
	}
	return RowSet{PlateIDs: sortedKeys(seenPlate), SessionIDs: sortedKeys(seenSession)}
}

func sortedKeys(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
