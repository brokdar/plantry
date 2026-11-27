package finds

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Not for me reasons (NEW-4). Taste, ingredients and too long match the
// taste package; looks wrong lowers the source instead of teaching taste.
const (
	ReasonTaste       = "taste"
	ReasonIngredients = "ingredients"
	ReasonTooLong     = "too_long"
	ReasonLooksWrong  = "looks_wrong"
)

// Taste is the taste hook a discard feeds (taste.Service).
type Taste interface {
	NotForMeIngredients(ctx context.Context, heroes []string) error
}

// Get returns a find.
func (s *Service) Get(ctx context.Context, id int64) (*Find, error) { return s.d.Store.Get(ctx, id) }

// List returns finds, optionally of one state.
func (s *Service) List(ctx context.Context, state *State) ([]Find, error) {
	if state != nil && !ValidState(*state) {
		return nil, fmt.Errorf("%w: unknown find state %q", domain.ErrInvalidInput, *state)
	}
	return s.d.Store.List(ctx, state, 100)
}

// Keep marks the find in the library with the recipe the review saved.
func (s *Service) Keep(ctx context.Context, id, foodID int64) (*Find, error) {
	return s.d.Store.Decide(ctx, id, StateInLibrary, nil, &foodID)
}

// Discard is "Not for me" on a ready or needs_check find (NEW-4): the
// ingredients reason teaches the named ingredients, looks wrong lowers the
// base source's site; taste, too long and no reason teach nothing yet (a
// find has no attributes before it is a recipe).
func (s *Service) Discard(ctx context.Context, id int64, reason string, ingredients []string) (*Find, error) {
	var r *string
	switch reason {
	case "":
	case ReasonTaste, ReasonIngredients, ReasonTooLong, ReasonLooksWrong:
		r = &reason
	default:
		return nil, fmt.Errorf("%w: unknown reason %q", domain.ErrInvalidInput, reason)
	}
	f, err := s.d.Store.Decide(ctx, id, StateDiscarded, r, nil)
	if err != nil {
		return nil, err
	}
	switch reason {
	case ReasonLooksWrong:
		for _, src := range f.Sources {
			if src.Base {
				if err := s.d.Store.PenaliseSource(ctx, src.Site); err != nil {
					return nil, err
				}
			}
		}
	case ReasonIngredients:
		if s.d.Taste != nil {
			if err := s.d.Taste.NotForMeIngredients(ctx, ingredients); err != nil {
				return nil, err
			}
		}
	}
	return f, nil
}
