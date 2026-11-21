package slot

// Kind says what a slot's plates must hold: a full meal needs a main, a
// light meal (breakfast, snack) takes anything.
type Kind string

const (
	KindLight Kind = "light"
	KindMeal  Kind = "meal"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == KindLight || k == KindMeal }

// TimeSlot is a named meal time (e.g. breakfast, lunch). Users define their own.
type TimeSlot struct {
	ID        int64
	NameKey   string
	Icon      string
	SortOrder int
	Active    bool
	// Kind defaults to KindMeal when empty on create and update.
	Kind Kind
	// BandMinPct and BandMaxPct are the slot's own energy band, in percent of
	// the day's target; both nil means the default band (see DefaultBand).
	BandMinPct, BandMaxPct *float64
}
