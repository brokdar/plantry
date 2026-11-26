package planning

import (
	"testing"
	"time"
)

func TestDayOfIsYearAndYearDay(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, loc := range []*time.Location{time.UTC, berlin} {
		for d := time.Date(1999, 12, 25, 23, 30, 0, 0, loc); d.Year() < 2102; d = d.AddDate(0, 0, 1) {
			if got, want := dayOf(d), d.Year()*1000+d.YearDay(); got != want {
				t.Fatalf("dayOf(%v) = %d, want %d", d, got, want)
			}
		}
	}
}
