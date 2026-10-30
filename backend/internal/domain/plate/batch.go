package plate

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Batch geometry (Phase 2 D11–D14). Everything in this file is pure.

// CellKey identifies a (date, slot) cell.
type CellKey struct {
	Date   time.Time
	SlotID int64
}

// Equal reports whether c and o name the same cell.
func (c CellKey) Equal(o CellKey) bool { return c.SlotID == o.SlotID && c.Date.Equal(o.Date) }

// SlotOrder maps a slot id to its sort_order.
type SlotOrder map[int64]int

// Compare orders cells by (date, slot sort_order, slot id) (D12): -1 when a
// comes first, 1 when b does, 0 for the same cell.
func Compare(a, b CellKey, order SlotOrder) int {
	switch {
	case a.Date.Before(b.Date):
		return -1
	case a.Date.After(b.Date):
		return 1
	}
	if oa, ob := order[a.SlotID], order[b.SlotID]; oa != ob {
		if oa < ob {
			return -1
		}
		return 1
	}
	switch {
	case a.SlotID < b.SlotID:
		return -1
	case a.SlotID > b.SlotID:
		return 1
	}
	return 0
}

// Before reports whether cell a comes strictly before cell b.
func Before(a, b CellKey, order SlotOrder) bool { return Compare(a, b, order) < 0 }

// LinkKind is how a linked component eats from its session.
type LinkKind string

const (
	// KindFresh: the component is on the session's cook cell.
	KindFresh LinkKind = "fresh"
	// KindLeftover: a later cell, on or before use-by.
	KindLeftover LinkKind = "leftover"
	// KindFrozen: after use-by; it eats frozen portions (D14).
	KindFrozen LinkKind = "frozen"
)

// UseBy returns the last day a session's leftovers are good: cook date plus
// keep days.
func UseBy(s CookSession, keepDays int) time.Time { return s.CookDate.AddDate(0, 0, keepDays) }

// Classify tells how a component on cell eats from s. A cell before the cook
// cell classifies as a leftover; Warnings reports it.
func Classify(cell CellKey, s CookSession, keepDays int) LinkKind {
	switch {
	case cell.Equal(s.CookCell()):
		return KindFresh
	case cell.Date.After(UseBy(s, keepDays)):
		return KindFrozen
	}
	return KindLeftover
}

// Usage is what a session's links consume: Total portions, of which Frozen
// are eaten past use-by.
type Usage struct {
	Total  float64
	Frozen float64
}

// Consumption sums what links consume from s.
func Consumption(s CookSession, links []Link, keepDays int) Usage {
	var u Usage
	for _, l := range links {
		p := l.PerEater * l.Eaters
		u.Total += p
		if Classify(l.Cell, s, keepDays) == KindFrozen {
			u.Frozen += p
		}
	}
	return u
}

// OrderLinks returns a copy of links sorted by cell, with Portions, Kind and
// Position filled in (D12).
func OrderLinks(s CookSession, links []Link, keepDays int, order SlotOrder) []Link {
	out := make([]Link, len(links))
	copy(out, links)
	sort.SliceStable(out, func(i, j int) bool {
		if c := Compare(out[i].Cell, out[j].Cell, order); c != 0 {
			return c < 0
		}
		return out[i].ComponentID < out[j].ComponentID
	})
	for i := range out {
		out[i].Portions = out[i].PerEater * out[i].Eaters
		out[i].Kind = Classify(out[i].Cell, s, keepDays)
		out[i].Position = i + 1
	}
	return out
}

// BatchSettings are the plan settings batch rules read.
type BatchSettings struct {
	DefaultKeepDays int
	FreezingEnabled bool
	FreezerKeepDays int
}

// KeepDays resolves a food's keep days: its own value, else the default.
func (b BatchSettings) KeepDays(foodKeepDays *int) int {
	if foodKeepDays != nil {
		return *foodKeepDays
	}
	return b.DefaultKeepDays
}

// CheckLink enforces the hard rules for linking a component of foodID on cell
// to s (D13): same food, on or after the cook cell (the cook cell itself is
// fresh), within use-by unless freezing is enabled, and within the freezer
// keep days.
func CheckLink(cell CellKey, foodID int64, s CookSession, keepDays int, set BatchSettings, order SlotOrder) error {
	if foodID != s.FoodID {
		return fmt.Errorf("%w: food %d, session %d cooks food %d", domain.ErrSessionFoodMismatch, foodID, s.ID, s.FoodID)
	}
	if Before(cell, s.CookCell(), order) {
		return fmt.Errorf("%w: session %d", domain.ErrSessionBeforeCook, s.ID)
	}
	if !cell.Date.After(UseBy(s, keepDays)) {
		return nil
	}
	if !set.FreezingEnabled {
		return fmt.Errorf("%w: session %d", domain.ErrSessionPastKeepDays, s.ID)
	}
	if cell.Date.After(s.CookDate.AddDate(0, 0, set.FreezerKeepDays)) {
		return fmt.Errorf("%w: session %d", domain.ErrSessionPastFreezerDays, s.ID)
	}
	return nil
}

// Warnings lists what is inconsistent about s and its links (D13): per link,
// leftover_before_cook and past_keep_days (past use-by with freezing off, or
// past the freezer keep days); then the session-wide deficit and, with
// freezing on, frozen_deficit.
func Warnings(s CookSession, links []Link, keepDays int, set BatchSettings, order SlotOrder) []Warning {
	out := []Warning{}
	useBy := UseBy(s, keepDays)
	freezerLimit := s.CookDate.AddDate(0, 0, set.FreezerKeepDays)
	for _, l := range links {
		plateID := l.PlateID
		if Before(l.Cell, s.CookCell(), order) {
			out = append(out, Warning{Code: WarnLeftoverBeforeCook, PlateID: &plateID})
		}
		if l.Cell.Date.After(useBy) && (!set.FreezingEnabled || l.Cell.Date.After(freezerLimit)) {
			out = append(out, Warning{Code: WarnPastKeepDays, PlateID: &plateID})
		}
	}
	use := Consumption(s, links, keepDays)
	if exceeds(use.Total, s.Portions) {
		out = append(out, Warning{Code: WarnDeficit})
	}
	if set.FreezingEnabled && exceeds(use.Frozen, s.FrozenPortions) {
		out = append(out, Warning{Code: WarnFrozenDeficit})
	}
	return out
}

// Sync applies the session portion rule (D11): a session that exactly covered
// its links before a change covers them exactly after it; a session with a
// surplus or deficit keeps its portions. Frozen portions never exceed
// portions.
func Sync(before, after Usage, portions, frozen float64) (float64, float64) {
	if almostEqual(portions, before.Total) {
		portions = after.Total
	}
	if frozen > portions {
		frozen = portions
	}
	return portions, frozen
}

const portionEpsilon = 1e-6

func almostEqual(a, b float64) bool { return math.Abs(a-b) < portionEpsilon }

func exceeds(a, b float64) bool { return a-b >= portionEpsilon }
