package theme

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Store persists the themes and suggestions.
type Store interface {
	List(ctx context.Context, start time.Time) ([]Theme, error)
	Add(ctx context.Context, t Theme) (Theme, error)
	// Delete removes the window's theme id; domain.ErrNotFound when missing.
	Delete(ctx context.Context, start time.Time, id int64) error
	// Suggestion is the window's suggestion; nil without one.
	Suggestion(ctx context.Context, start time.Time) (*Suggestion, error)
	// Suggest stores the window's suggestion as open (one per window).
	Suggest(ctx context.Context, start time.Time, s Suggestion) error
	// Decide closes the window's open suggestion; domain.ErrNotFound when
	// none is open.
	Decide(ctx context.Context, start time.Time, state string, at time.Time) error
	// RecentDecisions are the two newest decided suggestions, newest first.
	RecentDecisions(ctx context.Context) ([]Decision, error)
	// RecentValues are the theme and suggestion values of windows from since.
	RecentValues(ctx context.Context, since time.Time) ([]string, error)
}

// Deps wire the service.
type Deps struct {
	Store  Store
	ChefOn func(ctx context.Context) (bool, error)
	// Library counts the library recipes that fit t.
	Library func(ctx context.Context, t Theme) (int, error)
	// Find starts n finds for a theme (finds.Service.ThemeSet); nil: none.
	Find func(ctx context.Context, theme string, n int) error
	// Cuisines are the Cuisines to explore (discovery.cuisines).
	Cuisines func(ctx context.Context) ([]string, error)
	// Loves are the cuisines the household loves (taste items).
	Loves func(ctx context.Context) ([]string, error)
	Now   func() time.Time
}

// Service manages week themes.
type Service struct{ d Deps }

// NewService creates a Service.
func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// Window is a window's themes, the window before's themes to offer again
// while this one has none (THEME-5), and the open suggestion (THEME-4).
type Window struct {
	Themes     []Theme     `json:"themes"`
	Again      []Theme     `json:"again"`
	Suggestion *Suggestion `json:"suggestion"`
}

// Themes are the window's themes the planner follows: none with the Chef
// off (ON-1).
func (s *Service) Themes(ctx context.Context, start time.Time) ([]Theme, error) {
	if on, err := s.d.ChefOn(ctx); err != nil || !on {
		return nil, err
	}
	return s.d.Store.List(ctx, start)
}

// Window returns the window starting on start. A current or future window
// without themes gets at most one suggestion with the Chef on, unless it
// offers the window before's themes again.
func (s *Service) Window(ctx context.Context, start time.Time) (Window, error) {
	ts, err := s.d.Store.List(ctx, start)
	if err != nil {
		return Window{}, err
	}
	out := Window{Themes: ts, Again: []Theme{}}
	if len(ts) > 0 || !s.open(start) {
		return out, nil
	}
	if out.Again, err = s.d.Store.List(ctx, start.AddDate(0, 0, -7)); err != nil {
		return Window{}, err
	}
	on, err := s.d.ChefOn(ctx)
	if err != nil || !on || len(out.Again) > 0 {
		return out, err // "again?" is the window's one offer
	}
	out.Suggestion, err = s.suggestion(ctx, start)
	return out, err
}

// open reports whether the window starting on start has not ended.
func (s *Service) open(start time.Time) bool {
	return s.d.Now().Before(start.AddDate(0, 0, 7))
}

// Result is a set theme with what the library holds for it and how many
// finds it started ("No Japanese recipes yet. I'll find 2.").
type Result struct {
	Theme   Theme `json:"theme"`
	Library int   `json:"library"`
	Finding int   `json:"finding"`
}

// Set adds a theme to the window starting on start: at most MaxPerWindow, a
// window that has not ended. With the Chef on it starts the finds the
// library lacks the same day (FIND-1d).
func (s *Service) Set(ctx context.Context, start time.Time, t Theme) (Result, error) {
	t, err := t.Norm()
	if err != nil {
		return Result{}, err
	}
	if !s.open(start) {
		return Result{}, fmt.Errorf("%w: the window has ended", domain.ErrInvalidInput)
	}
	ts, err := s.d.Store.List(ctx, start)
	if err != nil {
		return Result{}, err
	}
	for _, o := range ts {
		if o.Kind == t.Kind && o.Value == t.Value {
			return s.result(ctx, o, false)
		}
	}
	if len(ts) >= MaxPerWindow {
		return Result{}, domain.ErrThemeLimit
	}
	t.WindowStart = start
	if t, err = s.d.Store.Add(ctx, t); err != nil {
		return Result{}, err
	}
	return s.result(ctx, t, true)
}

func (s *Service) result(ctx context.Context, t Theme, find bool) (Result, error) {
	n, err := s.d.Library(ctx, t)
	if err != nil {
		return Result{}, err
	}
	r := Result{Theme: t, Library: n}
	on, err := s.d.ChefOn(ctx)
	if err != nil || !on {
		return r, err
	}
	r.Finding = FindsFor(n)
	if find && r.Finding > 0 && s.d.Find != nil {
		if err := s.d.Find(ctx, t.Value, r.Finding); err != nil {
			return r, err
		}
	}
	return r, nil
}

// FindsFor is how many new dishes a theme asks for when the library holds
// n recipes that fit it: two theme dishes at least (THEME-3).
func FindsFor(n int) int { return max(0, 2-n) }

// NewDishes is how many new dishes the window's themes add to New dishes
// per week with the Chef on (THEME-3: a theme may raise them, the caller
// caps at 3).
func (s *Service) NewDishes(ctx context.Context, start time.Time) (int, error) {
	if on, err := s.d.ChefOn(ctx); err != nil || !on {
		return 0, err
	}
	ts, err := s.d.Store.List(ctx, start)
	if err != nil {
		return 0, err
	}
	extra := 0
	for _, t := range ts {
		n, err := s.d.Library(ctx, t)
		if err != nil {
			return 0, err
		}
		extra += FindsFor(n)
	}
	return extra, nil
}

// Delete removes a theme of the window.
func (s *Service) Delete(ctx context.Context, start time.Time, id int64) error {
	return s.d.Store.Delete(ctx, start, id)
}

// Accept sets the window's open suggestion as its theme.
func (s *Service) Accept(ctx context.Context, start time.Time) (Result, error) {
	sg, err := s.d.Store.Suggestion(ctx, start)
	if err != nil {
		return Result{}, err
	}
	if sg == nil || sg.State != StateOpen {
		return Result{}, domain.ErrNotFound
	}
	r, err := s.Set(ctx, start, Theme{Kind: sg.Kind, Value: sg.Value, Source: SourceSuggestion})
	if err != nil {
		return Result{}, err
	}
	return r, s.d.Store.Decide(ctx, start, StateAccepted, s.d.Now())
}

// Decline refuses the window's open suggestion.
func (s *Service) Decline(ctx context.Context, start time.Time) error {
	return s.d.Store.Decide(ctx, start, StateDeclined, s.d.Now())
}

// PauseAfter is how long suggestions pause after two refusals in a row
// (THEME-4).
const PauseAfter = 28 * 24 * time.Hour

// suggestion is the window's suggestion: the stored one, else a new one
// unless suggestions pause. Nil when there is none.
func (s *Service) suggestion(ctx context.Context, start time.Time) (*Suggestion, error) {
	sg, err := s.d.Store.Suggestion(ctx, start)
	if err != nil || sg != nil {
		if sg != nil && sg.State != StateOpen {
			return nil, err
		}
		return sg, err
	}
	ds, err := s.d.Store.RecentDecisions(ctx)
	if err != nil {
		return nil, err
	}
	if len(ds) == 2 && ds[0].State == StateDeclined && ds[1].State == StateDeclined && s.d.Now().Sub(ds[0].At) < PauseAfter {
		return nil, nil
	}
	c, ok, err := s.candidate(ctx, start)
	if err != nil || !ok {
		return nil, err
	}
	if err := s.d.Store.Suggest(ctx, start, c); err != nil {
		return nil, err
	}
	return s.d.Store.Suggestion(ctx, start)
}

// candidate picks a suggestion: a cuisine to explore, then a loved cuisine,
// then the season's ingredient, skipping values of the last four windows.
func (s *Service) candidate(ctx context.Context, start time.Time) (Suggestion, bool, error) {
	recent, err := s.d.Store.RecentValues(ctx, start.AddDate(0, 0, -28))
	if err != nil {
		return Suggestion{}, false, err
	}
	var cands []Suggestion
	for _, src := range []func(context.Context) ([]string, error){s.d.Cuisines, s.d.Loves} {
		if src == nil {
			continue
		}
		vs, err := src(ctx)
		if err != nil {
			return Suggestion{}, false, err
		}
		for _, v := range vs {
			cands = append(cands, Suggestion{Kind: KindCuisine, Value: norm(v)})
		}
	}
	cands = append(cands, Suggestion{Kind: KindIngredient, Value: Season(start)})
	for _, c := range cands {
		if c.Value != "" && !slices.Contains(recent, c.Value) {
			c.State = StateOpen
			return c, true, nil
		}
	}
	return Suggestion{}, false, nil
}

// Season is the seasonal ingredient a window starting on day may suggest.
func Season(day time.Time) string {
	switch day.Month() {
	case time.March, time.April, time.May:
		return "asparagus"
	case time.June, time.July, time.August:
		return "tomato"
	case time.September, time.October, time.November:
		return "pumpkin"
	}
	return "cabbage"
}
