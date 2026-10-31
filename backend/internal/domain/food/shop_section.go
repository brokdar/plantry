package food

import (
	"context"
	"fmt"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// ShopSection is a store section of the shopping list (D18).
type ShopSection string

// The store sections.
const (
	SectionProduce    ShopSection = "produce"
	SectionBakery     ShopSection = "bakery"
	SectionDairyEggs  ShopSection = "dairy_eggs"
	SectionMeatFish   ShopSection = "meat_fish"
	SectionDryGoods   ShopSection = "dry_goods"
	SectionCansJars   ShopSection = "cans_jars"
	SectionSpicesOils ShopSection = "spices_oils"
	SectionFrozen     ShopSection = "frozen"
	SectionOther      ShopSection = "other"
)

// ShopSections lists every section in the fixed store walk order (UX §5.7).
var ShopSections = []ShopSection{
	SectionProduce, SectionBakery, SectionDairyEggs, SectionMeatFish,
	SectionDryGoods, SectionCansJars, SectionSpicesOils, SectionFrozen,
	SectionOther,
}

// ValidShopSection reports whether s is a known section.
func ValidShopSection(s ShopSection) bool { return slices.Contains(ShopSections, s) }

// SetShopSection sets a leaf's store section override; nil clears it so the
// section is classified from the name again.
func (s *Service) SetShopSection(ctx context.Context, id int64, section *ShopSection) (*Food, error) {
	if section != nil && !ValidShopSection(*section) {
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidShopSection, *section)
	}
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind != KindLeaf {
		return nil, fmt.Errorf("%w: food %d", domain.ErrShopSectionLeafOnly, id)
	}
	return s.repo.SetShopSection(ctx, id, section)
}
