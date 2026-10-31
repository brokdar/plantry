// Package window is the plan window maths (Phase 2 D24). A window is the 7
// days starting on the shopping day (plan.shopping_day, 0 = Monday … 6 =
// Sunday). Everything here is pure.
package window

import "time"

// Anchor values of plan.anchor.
const (
	// AnchorToday opens the window containing today.
	AnchorToday = "today"
	// AnchorNextShoppingDay opens the window starting on the next shopping
	// day, today included.
	AnchorNextShoppingDay = "next_shopping_day"
)

// Length is the number of days in a window.
const Length = 7

// Window is a 7-day plan window: Start is a shopping day, End = Start + 6.
type Window struct {
	Start time.Time
	End   time.Time
}

func at(start time.Time) Window {
	return Window{Start: start, End: start.AddDate(0, 0, Length-1)}
}

// Containing returns the window holding date: it starts on the latest
// shopping day on or before date.
func Containing(date time.Time, shoppingDay int) Window {
	date = dateOnly(date)
	back := (weekday(date) - shoppingDay + Length) % Length
	return at(date.AddDate(0, 0, -back))
}

// Anchored returns the window the plan opens on: the window containing today
// for AnchorToday, otherwise the window starting on the first shopping day on
// or after today.
func Anchored(today time.Time, anchor string, shoppingDay int) Window {
	if anchor == AnchorToday {
		return Containing(today, shoppingDay)
	}
	today = dateOnly(today)
	ahead := (shoppingDay - weekday(today) + Length) % Length
	return at(today.AddDate(0, 0, ahead))
}

// IsStart reports whether date is a shopping day, i.e. a valid window start.
func IsStart(date time.Time, shoppingDay int) bool { return weekday(date) == shoppingDay }

// Prev is the start of the previous window.
func (w Window) Prev() time.Time { return w.Start.AddDate(0, 0, -Length) }

// Next is the start of the next window.
func (w Window) Next() time.Time { return w.Start.AddDate(0, 0, Length) }

// Contains reports whether date falls in [Start, End].
func (w Window) Contains(date time.Time) bool {
	date = dateOnly(date)
	return !date.Before(w.Start) && !date.After(w.End)
}

// Days lists the window's 7 dates in order.
func (w Window) Days() []time.Time {
	out := make([]time.Time, Length)
	for i := range out {
		out[i] = w.Start.AddDate(0, 0, i)
	}
	return out
}

// weekday returns t's weekday with Monday = 0.
func weekday(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
