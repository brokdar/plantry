package planning

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Proposal is what a chooser returns (D14): new sessions by string key and
// plates by cell whose composed components eat from a proposed session (by
// key), an existing one (by id) or, with neither, a fresh session of their
// own.
type Proposal struct {
	Sessions []ProposedSession
	Plates   []ProposedPlate
	// Notes is the chooser's short summary for the user.
	Notes string
}

// ProposedSession is a session the proposal cooks.
type ProposedSession struct {
	Key            string
	FoodID         int64
	CookDate       time.Time
	CookSlotID     int64
	Portions       float64
	FrozenPortions float64
	Reason         string
}

// ProposedPlate is a plate the proposal places. Eaters 0 means the
// household portions (D16).
type ProposedPlate struct {
	Date       time.Time
	SlotID     int64
	Eaters     float64
	Components []ProposedComponent
	Reason     string
}

// ProposedComponent is a composed food on a proposed plate, per eater.
type ProposedComponent struct {
	FoodID     int64
	Portions   float64
	SessionKey string
	SessionID  int64
}

// eaters resolves a proposed plate's eaters (D16).
func (p ProposedPlate) eaters(s State) float64 {
	if p.Eaters > 0 {
		return p.Eaters
	}
	return s.Profile.HouseholdPortions
}

// Merge returns the state with the target's draft rows replaced by the
// proposal: the plan the checker and the score judge. Confirmed rows stay
// even when the target names them (Check reports that as H6). Proposed
// plates get negative ids (-1, -2, … in proposal order) and proposed
// sessions their key. s is not modified.
func Merge(s State, t Target, p Proposal) State {
	dropPlate := idSet(t.PlateIDs)
	dropSession := idSet(t.SessionIDs)

	out := s
	out.Plates = make([]Plate, 0, len(s.Plates)+len(p.Plates))
	for _, pl := range s.Plates {
		if dropPlate[pl.ID] && !pl.Confirmed {
			continue
		}
		out.Plates = append(out.Plates, pl)
	}
	out.Sessions = make([]Session, 0, len(s.Sessions)+len(p.Sessions))
	for _, ss := range s.Sessions {
		if dropSession[ss.ID] && !ss.Confirmed {
			continue
		}
		out.Sessions = append(out.Sessions, ss)
	}
	for _, ps := range p.Sessions {
		out.Sessions = append(out.Sessions, Session{
			Key: ps.Key,
			CookSession: plate.CookSession{
				FoodID:         ps.FoodID,
				CookDate:       ps.CookDate,
				CookSlotID:     ps.CookSlotID,
				Portions:       ps.Portions,
				FrozenPortions: ps.FrozenPortions,
			},
		})
	}
	for i, pp := range p.Plates {
		comps := make([]Component, len(pp.Components))
		for j, pc := range pp.Components {
			portions := pc.Portions
			comps[j] = Component{FoodID: pc.FoodID, Portions: &portions, SessionKey: pc.SessionKey, SessionID: pc.SessionID}
		}
		out.Plates = append(out.Plates, Plate{
			ID:         -int64(i + 1),
			Date:       pp.Date,
			SlotID:     pp.SlotID,
			Eaters:     pp.eaters(s),
			Components: comps,
		})
	}
	return out
}

// ToSnapshot maps a proposal to the snapshot plate.Service.Apply writes
// (D10, D14): session keys become snapshot refs, existing session ids
// SessionLink{ID}, and plates without eaters get the household portions
// (D16).
func ToSnapshot(p Proposal, s State) plate.PlanSnapshot {
	snap := plate.PlanSnapshot{
		Plates:   make([]plate.SnapshotPlate, 0, len(p.Plates)),
		Sessions: make([]plate.SnapshotSession, 0, len(p.Sessions)),
	}
	for _, ps := range p.Sessions {
		snap.Sessions = append(snap.Sessions, plate.SnapshotSession{
			Ref:            ps.Key,
			FoodID:         ps.FoodID,
			CookDate:       ps.CookDate.Format(time.DateOnly),
			CookSlotID:     ps.CookSlotID,
			Portions:       ps.Portions,
			FrozenPortions: ps.FrozenPortions,
		})
	}
	for _, pp := range p.Plates {
		comps := make([]plate.SnapshotComponent, len(pp.Components))
		for i, pc := range pp.Components {
			portions := pc.Portions
			c := plate.SnapshotComponent{FoodID: pc.FoodID, Portions: &portions, SortOrder: i}
			switch {
			case pc.SessionKey != "":
				c.Session = &plate.SessionLink{Ref: pc.SessionKey}
			case pc.SessionID != 0:
				c.Session = &plate.SessionLink{ID: pc.SessionID}
			}
			comps[i] = c
		}
		snap.Plates = append(snap.Plates, plate.SnapshotPlate{
			Date:       pp.Date.Format(time.DateOnly),
			SlotID:     pp.SlotID,
			Eaters:     pp.eaters(s),
			Components: comps,
		})
	}
	return snap
}

func idSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
