package plate

import (
	"errors"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Position is a component's place on its plate: the main (at most one per
// plate), a side, or an extra (dessert, drink, a snack).
type Position string

// Positions.
const (
	PositionMain  Position = "main"
	PositionSide  Position = "side"
	PositionExtra Position = "extra"
)

// Valid reports whether p is one of the three positions.
func (p Position) Valid() bool {
	return p == PositionMain || p == PositionSide || p == PositionExtra
}

// ErrSecondMain is returned when a component asks for position main on a
// plate that already has a main.
var ErrSecondMain = errors.New("a plate has at most one main")

// DerivePosition is the position of a component of food f with the effective
// role role (nil: none) on a plate that already has a main (hasMain). A
// composed food whose role is main, standalone or none is the main unless the
// plate has one (then a side); dessert and drink are extras; other composed
// roles are sides. A leaf is a side when its effective role is a side role,
// else an extra.
func DerivePosition(f *food.Food, role *food.Role, hasMain bool) Position {
	if f.Kind == food.KindLeaf {
		if role != nil {
			switch *role {
			case food.RoleSideStarch, food.RoleSideVeg, food.RoleSideProtein:
				return PositionSide
			}
		}
		return PositionExtra
	}
	if role == nil {
		role = f.Role
	}
	switch {
	case role == nil || *role == food.RoleMain || *role == food.RoleStandalone:
		if hasMain {
			return PositionSide
		}
		return PositionMain
	case *role == food.RoleDessert || *role == food.RoleDrink:
		return PositionExtra
	}
	return PositionSide
}

// resolvePosition sets pc.Position for a component about to be written onto
// a plate whose components so far are existing: an explicit position is
// kept (a second main is ErrSecondMain), an empty one is derived.
func (s *Service) resolvePosition(pc *PlateComponent, f *food.Food, existing []PlateComponent) error {
	hasMain := false
	for _, c := range existing {
		if c.Position == PositionMain {
			hasMain = true
		}
	}
	if pc.Position != "" {
		if !pc.Position.Valid() {
			return fmt.Errorf("%w: invalid position %q", domain.ErrInvalidInput, pc.Position)
		}
		if pc.Position == PositionMain && hasMain {
			return fmt.Errorf("%w: %w", domain.ErrInvalidInput, ErrSecondMain)
		}
		return nil
	}
	var role *food.Role
	if s.roles != nil {
		role = s.roles(f)
	}
	pc.Position = DerivePosition(f, role, hasMain)
	return nil
}
