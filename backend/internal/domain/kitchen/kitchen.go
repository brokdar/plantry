// Package kitchen holds the foods the household always has at home (KIT-4)
// and the foods it never eats (TASTE-1). Both are lists of foods by
// reference; the kitchen settings themselves live on the profile.
package kitchen

import (
	"context"
	"strings"
)

// Food is a listed food.
type Food struct {
	ID   int64
	Name string
}

// Repository is the port adapters implement. Add is idempotent and returns
// domain.ErrNotFound for an unknown food; Remove of an absent food is not an
// error.
type Repository interface {
	Basics(ctx context.Context) ([]Food, error)
	AddBasic(ctx context.Context, foodID int64) error
	RemoveBasic(ctx context.Context, foodID int64) error
	NeverItems(ctx context.Context) ([]Food, error)
	AddNeverItem(ctx context.Context, foodID int64) error
	RemoveNeverItem(ctx context.Context, foodID int64) error
}

// Service manages the basics at home. The never items are read and written
// by later work through the repository.
type Service struct{ repo Repository }

// NewService creates a kitchen service.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Basics lists the foods at home, by name.
func (s *Service) Basics(ctx context.Context) ([]Food, error) { return s.repo.Basics(ctx) }

// AddBasic marks a food as always at home.
func (s *Service) AddBasic(ctx context.Context, foodID int64) error {
	return s.repo.AddBasic(ctx, foodID)
}

// RemoveBasic takes a food off the list.
func (s *Service) RemoveBasic(ctx context.Context, foodID int64) error {
	return s.repo.RemoveBasic(ctx, foodID)
}

// NeverItems lists the foods the household never eats, by name.
func (s *Service) NeverItems(ctx context.Context) ([]Food, error) { return s.repo.NeverItems(ctx) }

// AddNeverItem makes a food a never item.
func (s *Service) AddNeverItem(ctx context.Context, foodID int64) error {
	return s.repo.AddNeverItem(ctx, foodID)
}

// RemoveNeverItem takes a food off the never list.
func (s *Service) RemoveNeverItem(ctx context.Context, foodID int64) error {
	return s.repo.RemoveNeverItem(ctx, foodID)
}

// basicNames are the foods a kitchen has at home (KIT-4): salt, pepper, oil,
// sugar, flour and common dried spices, in English and German. Migration
// 00059 backfills the same list for foods that already exist.
var basicNames = map[string]bool{
	"salt": true, "salz": true, "pepper": true, "pfeffer": true, "black pepper": true, "schwarzer pfeffer": true,
	"oil": true, "öl": true, "olive oil": true, "olivenöl": true, "vegetable oil": true, "pflanzenöl": true,
	"sunflower oil": true, "sonnenblumenöl": true, "rapeseed oil": true, "rapsöl": true,
	"sugar": true, "zucker": true, "flour": true, "mehl": true, "wheat flour": true, "weizenmehl": true,
	"oregano": true, "thyme": true, "thymian": true, "dried basil": true, "basilikum getrocknet": true,
	"paprika powder": true, "paprikapulver": true, "cumin": true, "kreuzkümmel": true, "cinnamon": true, "zimt": true,
	"curry powder": true, "currypulver": true, "bay leaf": true, "bay leaves": true, "lorbeerblatt": true, "lorbeerblätter": true,
	"chili flakes": true, "chiliflocken": true, "nutmeg": true, "muskat": true, "muskatnuss": true,
	"turmeric": true, "kurkuma": true, "garlic powder": true, "knoblauchpulver": true,
}

// IsBasicName reports whether a leaf food of this name belongs on the
// prefilled basics list.
func IsBasicName(name string) bool { return basicNames[strings.ToLower(strings.TrimSpace(name))] }
