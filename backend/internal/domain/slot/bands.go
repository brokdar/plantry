package slot

import (
	"context"
	"fmt"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Band is a slot's effective energy band: the share of the day's target its
// meal should hold, in percent.
type Band struct {
	SlotID         int64
	MinPct, MaxPct float64
}

// DefaultBand is the band of a slot with none of its own (spec SB1): by the
// slot's name (breakfast 20-30, lunch 30-40, dinner 25-40, snack 5-15), else by
// its kind (light 5-15, meal 25-40). There is no official split; these follow
// common nutrition guidance.
func DefaultBand(nameKey string, kind Kind) (minPct, maxPct float64) {
	switch strings.TrimPrefix(nameKey, "slot.") {
	case "breakfast":
		return 20, 30
	case "lunch":
		return 30, 40
	case "dinner":
		return 25, 40
	case "snack":
		return 5, 15
	}
	if kind == KindLight {
		return 5, 15
	}
	return 25, 40
}

// Effective is the slot's own band, else its default.
func (t TimeSlot) Effective() Band {
	if t.BandMinPct != nil && t.BandMaxPct != nil {
		return Band{SlotID: t.ID, MinPct: *t.BandMinPct, MaxPct: *t.BandMaxPct}
	}
	lo, hi := DefaultBand(t.NameKey, t.Kind)
	return Band{SlotID: t.ID, MinPct: lo, MaxPct: hi}
}

// Shares are the expected shares of the slots (SB2): each band's midpoint over
// the sum of the midpoints, as fractions of the day. Every rule that needs "the
// meal's share" reads this one function.
func Shares(bands []Band) map[int64]float64 {
	total := 0.0
	for _, b := range bands {
		total += (b.MinPct + b.MaxPct) / 2
	}
	out := make(map[int64]float64, len(bands))
	for _, b := range bands {
		if total > 0 {
			out[b.SlotID] = (b.MinPct + b.MaxPct) / 2 / total
		} else if len(bands) > 0 {
			out[b.SlotID] = 1 / float64(len(bands))
		}
	}
	return out
}

// ValidateBands checks a set of effective bands: each within 0-100 and
// min <= max, the minimums adding up to at most 100 and the maximums to at
// least 100. The error names the numbers.
func ValidateBands(bands []Band) error {
	minSum, maxSum := 0.0, 0.0
	for _, b := range bands {
		if b.MinPct < 0 || b.MaxPct > 100 || b.MinPct > b.MaxPct {
			return fmt.Errorf("%w: a band of %g to %g %% is not inside 0 to 100 with the minimum at most the maximum", domain.ErrInvalidSlotBands, b.MinPct, b.MaxPct)
		}
		minSum += b.MinPct
		maxSum += b.MaxPct
	}
	if len(bands) > 0 && (minSum > 100+1e-9 || maxSum < 100-1e-9) {
		return fmt.Errorf("%w: minimums add up to %g %% (at most 100), maximums to %g %% (at least 100)", domain.ErrInvalidSlotBands, minSum, maxSum)
	}
	return nil
}

// Bands returns the effective bands of the active slots in sort order.
func (s *Service) Bands(ctx context.Context) ([]Band, error) {
	slots, err := s.repo.List(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]Band, len(slots))
	for i, t := range slots {
		out[i] = t.Effective()
	}
	return out, nil
}

// SetBands stores the given bands (slots not named keep theirs) after checking
// the whole effective set of active slots: a refusal changes nothing.
func (s *Service) SetBands(ctx context.Context, set []Band) error {
	slots, err := s.repo.List(ctx, true)
	if err != nil {
		return err
	}
	byID := map[int64]Band{}
	for _, t := range slots {
		byID[t.ID] = t.Effective()
	}
	for _, b := range set {
		if _, ok := byID[b.SlotID]; !ok {
			return fmt.Errorf("%w: time slot %d", domain.ErrNotFound, b.SlotID)
		}
		byID[b.SlotID] = b
	}
	effective := make([]Band, 0, len(slots))
	for _, t := range slots {
		effective = append(effective, byID[t.ID])
	}
	if err := ValidateBands(effective); err != nil {
		return err
	}
	write := func(ctx context.Context) error {
		for _, b := range set {
			lo, hi := b.MinPct, b.MaxPct
			if err := s.repo.SetBand(ctx, b.SlotID, &lo, &hi); err != nil {
				return err
			}
		}
		return nil
	}
	if s.tx != nil {
		return s.tx.InTx(ctx, write)
	}
	return write(ctx)
}

// presets are the two named distributions besides the default: the biggest
// meal of the day by slot name; a snack or custom slot keeps its kind's band.
var presets = map[string]map[string][2]float64{
	"lunch_biggest":  {"breakfast": {20, 30}, "lunch": {35, 45}, "dinner": {20, 30}},
	"dinner_biggest": {"breakfast": {20, 30}, "lunch": {20, 30}, "dinner": {35, 45}},
}

// ApplyPreset sets the bands of the active slots to a named preset
// ("lunch_biggest" or "dinner_biggest"); any other name is invalid input.
func (s *Service) ApplyPreset(ctx context.Context, name string) error {
	preset, ok := presets[name]
	if !ok {
		return fmt.Errorf("%w: unknown band preset %q", domain.ErrInvalidInput, name)
	}
	slots, err := s.repo.List(ctx, true)
	if err != nil {
		return err
	}
	set := make([]Band, 0, len(slots))
	for _, t := range slots {
		lo, hi := DefaultBand(t.NameKey, t.Kind)
		if v, ok := preset[strings.TrimPrefix(t.NameKey, "slot.")]; ok {
			lo, hi = v[0], v[1]
		}
		set = append(set, Band{SlotID: t.ID, MinPct: lo, MaxPct: hi})
	}
	return s.SetBands(ctx, set)
}
