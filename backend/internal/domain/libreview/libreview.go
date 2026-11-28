// Package libreview is the opt-in monthly library review (LIB-1..3): at
// most one card a month on Recipes with at most three findings, computed by
// rules without a model call, and the gaps the assistant fills itself,
// queued for the next planning session.
package libreview

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// MaxFindings is the most findings one card shows (LIB-2).
const MaxFindings = 3

// ReplacedMin and ReplacedWindow: a recipe the user took off a plan at
// least ReplacedMin times within ReplacedWindow keeps getting replaced.
const (
	ReplacedMin    = 3
	ReplacedWindow = 90 * 24 * time.Hour
)

// Kind is a finding kind.
type Kind string

// Finding kinds (LIB-2).
const (
	KindDuplicates Kind = "duplicates"
	KindReplaced   Kind = "replaced"
	KindData       Kind = "data"
)

// RecipeRef names a recipe in a finding; Missing lists the data it lacks
// (times, steps) in a data finding.
type RecipeRef struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Missing []string `json:"missing,omitempty"`
}

// Finding is one thing the review noticed. Key is stable within a month.
type Finding struct {
	Key       string      `json:"key"`
	Kind      Kind        `json:"kind"`
	Recipes   []RecipeRef `json:"recipes"`
	Count     int         `json:"count,omitempty"`
	Dismissed bool        `json:"dismissed,omitempty"`
}

// Card is the review card of a month.
type Card struct {
	Month    string    `json:"month"`
	Findings []Finding `json:"findings"`
}

// State is the stored review: the month it was computed for.
type State struct {
	Month         string
	Findings      []Finding
	CardDismissed bool
}

// Recipe is what the rules read of an active recipe.
type Recipe struct {
	ID           int64
	Name         string
	Role         string
	Prep, Cook   int
	Steps        int
	Ingredients  []int64 // leaf ids, sorted, distinct
	VariantGroup int64   // 0: none
}

// Gap is a gap the assistant fills itself (LIB-3): Dish is the find query.
type Gap struct {
	Kind string
	Dish string
}

// Store persists the review and reads the library.
type Store interface {
	State(ctx context.Context) (State, error)
	SaveState(ctx context.Context, s State) error
	Recipes(ctx context.Context) ([]Recipe, error)
	// Replaced counts per recipe the times the user took it off a plan
	// since the time.
	Replaced(ctx context.Context, since time.Time) (map[int64]int, error)
	// QueueGap queues the gap; a gap of that kind already queued stays.
	QueueGap(ctx context.Context, g Gap, at time.Time) error
	// GroupVariants puts recipes a and b into one variant group.
	GroupVariants(ctx context.Context, a, b int64) error
}

// Deps wire the Service.
type Deps struct {
	Store Store
	// Enabled reports whether the review is on (the Chef and LIB-1).
	Enabled func(ctx context.Context) (bool, error)
	Tx      domain.Transactor
	Now     func() time.Time
}

// Service computes and serves the review card.
type Service struct{ d Deps }

// NewService creates a Service.
func NewService(d Deps) *Service { return &Service{d: d} }

func month(t time.Time) string { return t.Format("2006-01") }

// Get returns this month's card, computing it the first time it is asked in
// a month; nil when the review is off, the card was dismissed or holds
// nothing. Gaps found while computing are queued for planning.
func (s *Service) Get(ctx context.Context) (*Card, error) {
	on, err := s.d.Enabled(ctx)
	if err != nil || !on {
		return nil, err
	}
	now := s.d.Now()
	st, err := s.d.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if st.Month != month(now) {
		if st, err = s.compute(ctx, now); err != nil {
			return nil, err
		}
	}
	if st.CardDismissed {
		return nil, nil
	}
	var shown []Finding
	for _, f := range st.Findings {
		if !f.Dismissed {
			shown = append(shown, f)
		}
	}
	if len(shown) == 0 {
		return nil, nil
	}
	return &Card{Month: st.Month, Findings: shown}, nil
}

func (s *Service) compute(ctx context.Context, now time.Time) (State, error) {
	recipes, err := s.d.Store.Recipes(ctx)
	if err != nil {
		return State{}, err
	}
	counts, err := s.d.Store.Replaced(ctx, now.Add(-ReplacedWindow))
	if err != nil {
		return State{}, err
	}
	st := State{Month: month(now), Findings: Pick(duplicates(recipes), replaced(recipes, counts), data(recipes))}
	for _, g := range gaps(recipes) {
		if err := s.d.Store.QueueGap(ctx, g, now); err != nil {
			return State{}, err
		}
	}
	return st, s.d.Store.SaveState(ctx, st)
}

// Dismiss hides one finding of the current card, or the whole card (empty
// key). An unknown key is domain.ErrNotFound.
func (s *Service) Dismiss(ctx context.Context, key string) error {
	st, err := s.d.Store.State(ctx)
	if err != nil {
		return err
	}
	if key == "" {
		st.CardDismissed = true
		return s.d.Store.SaveState(ctx, st)
	}
	i := slices.IndexFunc(st.Findings, func(f Finding) bool { return f.Key == key })
	if i < 0 {
		return fmt.Errorf("%w: library review finding %q", domain.ErrNotFound, key)
	}
	st.Findings[i].Dismissed = true
	return s.d.Store.SaveState(ctx, st)
}

// MakeVariants groups the two recipes of a duplicates finding as variants
// of one dish and dismisses the finding.
func (s *Service) MakeVariants(ctx context.Context, key string) error {
	return s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		st, err := s.d.Store.State(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(st.Findings, func(f Finding) bool { return f.Key == key && f.Kind == KindDuplicates })
		if i < 0 {
			return fmt.Errorf("%w: library review finding %q", domain.ErrNotFound, key)
		}
		f := st.Findings[i]
		if err := s.d.Store.GroupVariants(ctx, f.Recipes[0].ID, f.Recipes[1].ID); err != nil {
			return err
		}
		st.Findings[i].Dismissed = true
		return s.d.Store.SaveState(ctx, st)
	})
}
