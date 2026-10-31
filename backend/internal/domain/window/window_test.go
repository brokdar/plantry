package window

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

const saturday = 5 // 0 = Monday … 6 = Sunday

func TestContaining(t *testing.T) {
	tests := []struct {
		name        string
		date        string
		shoppingDay int
		start       string
	}{
		{"AC1 Wednesday in a Saturday window", "2026-11-04", saturday, "2026-10-31"},
		{"the shopping day itself", "2026-10-31", saturday, "2026-10-31"},
		{"the last day of the window", "2026-11-06", saturday, "2026-10-31"},
		{"Monday shopping day on a Sunday", "2026-11-08", 0, "2026-11-02"},
		{"Sunday shopping day on a Sunday", "2026-11-08", 6, "2026-11-08"},
		{"across a month and year end", "2027-01-01", 0, "2026-12-28"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Containing(d(tt.date), tt.shoppingDay)
			assert.Equal(t, tt.start, w.Start.Format(time.DateOnly))
			assert.Equal(t, d(tt.start).AddDate(0, 0, 6), w.End)
		})
	}
}

func TestContaining_DropsTimeOfDay(t *testing.T) {
	at := time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC)
	assert.Equal(t, d("2026-10-31"), Containing(at, saturday).Start)
}

func TestAnchored(t *testing.T) {
	tests := []struct {
		name   string
		today  string
		anchor string
		start  string
	}{
		{"AC1 next_shopping_day from Thursday", "2026-11-05", AnchorNextShoppingDay, "2026-11-07"},
		{"AC1 next_shopping_day on the shopping day", "2026-11-07", AnchorNextShoppingDay, "2026-11-07"},
		{"next_shopping_day the day before", "2026-11-06", AnchorNextShoppingDay, "2026-11-07"},
		{"today from Thursday", "2026-11-05", AnchorToday, "2026-10-31"},
		{"today on the shopping day", "2026-11-07", AnchorToday, "2026-11-07"},
		{"unknown anchor falls back to next_shopping_day", "2026-11-05", "fixed_weekday", "2026-11-07"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.start, Anchored(d(tt.today), tt.anchor, saturday).Start.Format(time.DateOnly))
		})
	}
}

func TestIsStart(t *testing.T) {
	assert.True(t, IsStart(d("2026-10-31"), saturday))
	assert.False(t, IsStart(d("2026-11-01"), saturday))
	assert.True(t, IsStart(d("2026-11-02"), 0))
}

func TestWindow_PrevNextContainsDays(t *testing.T) {
	w := Containing(d("2026-11-04"), saturday)
	assert.Equal(t, d("2026-10-24"), w.Prev())
	assert.Equal(t, d("2026-11-07"), w.Next())
	assert.True(t, w.Contains(d("2026-10-31")))
	assert.True(t, w.Contains(d("2026-11-06")))
	assert.False(t, w.Contains(d("2026-11-07")))
	assert.False(t, w.Contains(d("2026-10-30")))
	days := w.Days()
	assert.Len(t, days, 7)
	assert.Equal(t, d("2026-11-06"), days[6])
}
