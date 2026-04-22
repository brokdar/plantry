package slot

// TimeSlot is a named meal time (e.g. breakfast, lunch). Users define their own.
type TimeSlot struct {
	ID        int64
	NameKey   string
	Icon      string
	SortOrder int
	Active    bool
}
