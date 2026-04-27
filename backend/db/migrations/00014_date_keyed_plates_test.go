package migrations_test

import (
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/db/migrations"
)

func TestIsoWeekStart(t *testing.T) {
	t.Run("2020 week 1 starts on 2019-12-30", func(t *testing.T) {
		got := migrations.IsoWeekStart(2020, 1)
		want := time.Date(2019, 12, 30, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("IsoWeekStart(2020,1) = %s; want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	})

	t.Run("2025 week 1 starts on 2024-12-30", func(t *testing.T) {
		got := migrations.IsoWeekStart(2025, 1)
		want := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("IsoWeekStart(2025,1) = %s; want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	})

	t.Run("2020 week 53 day 6 is 2021-01-03", func(t *testing.T) {
		// 2020 is a 53-week year. week=53, day=6 (Sunday).
		got := migrations.IsoWeekStart(2020, 53).AddDate(0, 0, 6)
		want := time.Date(2021, 1, 3, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("IsoWeekStart(2020,53)+6d = %s; want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	})

	t.Run("round-trip 2020-01-01 through 2030-12-31", func(t *testing.T) {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC)
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			y, w := d.ISOWeek()
			// day offset: 0=Mon .. 6=Sun
			day := int(d.Weekday()+6) % 7
			got := migrations.IsoWeekStart(y, w).AddDate(0, 0, day)
			if !got.Equal(d) {
				t.Errorf("round-trip %s: IsoWeekStart(%d,%d)+%d = %s",
					d.Format("2006-01-02"), y, w, day, got.Format("2006-01-02"))
			}
		}
	})
}
