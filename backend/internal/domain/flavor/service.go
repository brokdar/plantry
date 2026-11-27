package flavor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
)

// Store keeps the stored attribute records. A food without one reads as an
// empty Record. Calls join the unit of work in ctx.
type Store interface {
	Get(ctx context.Context, foodID int64) (Record, error)
	Many(ctx context.Context, foodIDs []int64) (map[int64]Record, error)
	Put(ctx context.Context, rec Record) error
	Delete(ctx context.Context, foodID int64) error
}

// Foods reads a food (domain.ErrNotFound when missing).
type Foods interface {
	Get(ctx context.Context, id int64) (*food.Food, error)
}

// Nutrition resolves foods' leaves and energy per portion.
type Nutrition interface {
	PerPortionDetailedMany(ctx context.Context, ids []int64) (map[int64]food.Detailed, error)
}

// Recorder is the food change log (foodevent.Service).
type Recorder interface {
	Recorded(ctx context.Context, fn func(ctx context.Context) error) (foodevent.Undo, error)
	Record(ctx context.Context, e foodevent.Event) error
}

// View is a food's effective attributes. Version is the stored record's
// schema version, 0 when nothing is stored.
type View struct {
	FoodID  int64 `json:"food_id"`
	Version int   `json:"version"`
	Resolved
}

// Service reads and edits recipe attributes.
type Service struct {
	store     Store
	foods     Foods
	nutrition Nutrition
	events    Recorder
}

// NewService creates a Service.
func NewService(store Store, foods Foods, nutrition Nutrition, events Recorder) *Service {
	return &Service{store: store, foods: foods, nutrition: nutrition, events: events}
}

var _ foodevent.Restorer = (*Service)(nil)

func (s *Service) input(ctx context.Context, id int64) (Input, error) {
	f, err := s.foods.Get(ctx, id)
	if err != nil {
		return Input{}, err
	}
	d, err := s.nutrition.PerPortionDetailedMany(ctx, []int64{id})
	if err != nil {
		return Input{}, fmt.Errorf("nutrition of food %d: %w", id, err)
	}
	return InputOf(f, d[id]), nil
}

// Get returns the attributes of food id.
func (s *Service) Get(ctx context.Context, id int64) (View, error) {
	in, err := s.input(ctx, id)
	if err != nil {
		return View{}, err
	}
	rec, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	return View{FoodID: id, Version: rec.Version, Resolved: Resolve(in, rec)}, nil
}

// Set applies the user's edit p to food id and records it as an attribute
// food event; the Undo is empty when nothing changed. An invalid patch is
// domain.ErrInvalidInput.
func (s *Service) Set(ctx context.Context, id int64, p Patch) (View, foodevent.Undo, error) {
	var view View
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		in, err := s.input(ctx, id)
		if err != nil {
			return err
		}
		if err := p.check(in); err != nil {
			return err
		}
		old, err := s.store.Get(ctx, id)
		if err != nil {
			return err
		}
		next := p.apply(old)
		next.FoodID = id
		if err := s.write(ctx, old, next); err != nil {
			return err
		}
		view = View{FoodID: id, Resolved: Resolve(in, next)}
		if !next.Empty() {
			view.Version = next.Version
		}
		return nil
	})
	if err != nil {
		return View{}, foodevent.Undo{}, err
	}
	return view, undo, nil
}

// SetAuto applies the assistant's values p.Fields to food id (VAR-4, ATTR-2):
// a field the user set keeps the user's value (A1's user-wins merge) and is
// returned in kept. p.Clear is ignored: the assistant only sets or corrects.
func (s *Service) SetAuto(ctx context.Context, id int64, p Patch) (View, foodevent.Undo, []Field, error) {
	var view View
	var kept []Field
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		in, err := s.input(ctx, id)
		if err != nil {
			return err
		}
		p.Clear = nil
		if err := p.check(in); err != nil {
			return err
		}
		old, err := s.store.Get(ctx, id)
		if err != nil {
			return err
		}
		next := Record{FoodID: id, Version: Version, Sources: map[Field]Source{}}
		for f, src := range old.Sources {
			next.Sources[f] = src
			copyField(&next.Values, old.Values, f)
		}
		for _, f := range p.Fields {
			if old.Sources[f] == SourceUser {
				kept = append(kept, f)
				continue
			}
			next.Sources[f] = SourceAuto
			copyField(&next.Values, p.Set, f)
		}
		if err := s.write(ctx, old, next); err != nil {
			return err
		}
		view = View{FoodID: id, Resolved: Resolve(in, next)}
		if !next.Empty() {
			view.Version = next.Version
		}
		return nil
	})
	if err != nil {
		return View{}, foodevent.Undo{}, nil, err
	}
	return view, undo, kept, nil
}

// write stores next in place of old and records the change; nothing when
// they are equal.
func (s *Service) write(ctx context.Context, old, next Record) error {
	before, err := snapshot(old)
	if err != nil {
		return err
	}
	after, err := snapshot(next)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}
	action := foodevent.ActionUpdate
	switch {
	case next.Empty():
		action = foodevent.ActionDelete
		err = s.store.Delete(ctx, next.FoodID)
	case old.Empty():
		action = foodevent.ActionCreate
		err = s.store.Put(ctx, next)
	default:
		err = s.store.Put(ctx, next)
	}
	if err != nil {
		return err
	}
	return s.events.Record(ctx, foodevent.Event{
		Kind: foodevent.KindAttribute, SubjectID: next.FoodID, Action: action, Before: before, After: after,
	})
}

// snapshot is a record's food event JSON; nil for an empty record.
func snapshot(r Record) (json.RawMessage, error) {
	if r.Empty() {
		return nil, nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("attribute snapshot: %w", err)
	}
	return b, nil
}

// Restore puts food id's stored attributes back to before (nil: none); it
// is the attribute kind's foodevent.Restorer.
func (s *Service) Restore(ctx context.Context, id int64, before json.RawMessage) error {
	if before == nil {
		return s.store.Delete(ctx, id)
	}
	var rec Record
	if err := json.Unmarshal(before, &rec); err != nil {
		return fmt.Errorf("attribute snapshot of food %d: %w", id, err)
	}
	rec.FoodID = id
	return s.store.Put(ctx, rec)
}
