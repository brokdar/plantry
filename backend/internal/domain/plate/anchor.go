package plate

import (
	"slices"
	"time"
)

// Batch invariants (meal editing, R1 and R2). The decision is pure; settle
// applies it to every session a mutation tracks.
//
// R1: an uncooked batch cooks at its earliest linked meal (cell order, D12),
// or earlier when it was cooked ahead, and never on a "none" cell.
// R2: every link lies within the keeping time from the cook date
// (past_keep_days of Warnings). A re-anchor never adds a past_keep_days
// warning.

// What values of AlsoChanged. AlsoCookMoved: the batch's cook moved to
// (Date, SlotID), the cell of the meal PlateID (0: none), from (FromDate,
// FromSlotID). The meal operations (meal.go) add AlsoBecameCooking (the
// cook moved to the meal PlateID the caller did not name), AlsoCooksFresh
// (the named meal PlateID left batch SessionID and cooks fresh at its own
// cell (Date, SlotID); Why says which rule broke) and AlsoBatchResized
// (D11 resized SessionID, cooking at (Date, SlotID)).
const (
	AlsoCookMoved     = "cook_moved"
	AlsoBecameCooking = "became_cooking_meal"
	AlsoCooksFresh    = "cooks_fresh"
	AlsoBatchResized  = "batch_resized"
)

// AlsoChanged is a change a mutation made beyond what the caller named (see
// the What values).
type AlsoChanged struct {
	SessionID  int64  `json:"session_id"`
	PlateID    int64  `json:"plate_id"`
	Date       string `json:"date"`
	SlotID     int64  `json:"slot_id"`
	What       string `json:"what"`
	FromDate   string `json:"from_date"`
	FromSlotID int64  `json:"from_slot_id"`
	Why        string `json:"why,omitempty"`
}

// batchBefore is a session as it was before a mutation: its cook cell and
// the cells of its links.
type batchBefore struct {
	cook  CellKey
	cells []CellKey
}

// cookedAhead reports whether the batch cooked strictly before every one of
// its meals: nothing was eaten on its cook cell. A batch without meals before
// the mutation (one it creates, such as a cook_at session) counts as cooked
// ahead: its cook cell is the one its creator chose.
func (b batchBefore) cookedAhead(order SlotOrder) bool {
	for _, c := range b.cells {
		if !Before(b.cook, c, order) {
			return false
		}
	}
	return true
}

// anchorInput is what Reanchor judges for one session after a mutation.
type anchorInput struct {
	Session CookSession
	Before  batchBefore
	Links   []Link
	// CookNone: the session's cook cell resolves to "none".
	CookNone bool
	// Restore: the mutation brings removed meals back (Apply with Restore,
	// or an Apply snapshot that links this session by id), so a meal before
	// the cook pulls the cook back to it.
	Restore bool
	// Today: the cook never moves to a cell before it.
	Today    time.Time
	KeepDays int
	Set      BatchSettings
	Order    SlotOrder
}

// reanchor returns the cell an uncooked batch must cook at after a mutation
// and true, or false when it stays. A batch whose cook cell no longer holds
// one of its meals moves to its earliest meal unless it was cooked ahead; a
// cooked-ahead batch moves only when its cook cell became "none". The target
// is the earliest meal that is neither frozen (it eats from the freezer, not
// from the fresh cook) nor before Today; with none, the batch stays. A cooked
// batch never moves, nor does a move that would add a past_keep_days warning.
// The returned link is the meal now cooking.
func reanchor(in anchorInput) (Link, bool) {
	s := in.Session
	if s.CookedAt != nil || len(in.Links) == 0 {
		return Link{}, false
	}
	ordered := OrderLinks(s, in.Links, in.KeepDays, in.Order)
	i := slices.IndexFunc(ordered, func(l Link) bool {
		return l.Kind != KindFrozen && !dateOnly(l.Cell.Date).Before(dateOnly(in.Today))
	})
	if i < 0 {
		return Link{}, false
	}
	first := ordered[i]
	cook := s.CookCell()
	if first.Cell.Equal(cook) {
		return Link{}, false
	}
	cookHasMeal := false
	for _, l := range ordered {
		if l.Cell.Equal(cook) {
			cookHasMeal = true
			break
		}
	}
	ahead := !cookHasMeal && in.Before.cookedAhead(in.Order)
	move := (!cookHasMeal && !ahead) || (ahead && in.CookNone) ||
		(in.Restore && len(in.Before.cells) > 0 && Before(first.Cell, cook, in.Order))
	if !move {
		return Link{}, false
	}
	moved := s
	moved.CookDate, moved.CookSlotID = first.Cell.Date, first.Cell.SlotID
	if pastKeep(moved, in.Links, in) > pastKeep(s, in.Links, in) {
		return Link{}, false
	}
	return first, true
}

func pastKeep(s CookSession, links []Link, in anchorInput) int {
	n := 0
	for _, w := range Warnings(s, OrderLinks(s, links, in.KeepDays, in.Order), in.KeepDays, in.Set, in.Order) {
		if w.Code == WarnPastKeepDays {
			n++
		}
	}
	return n
}
