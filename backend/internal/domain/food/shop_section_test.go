package food_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func ptrSection(s food.ShopSection) *food.ShopSection { return &s }

func TestService_SetShopSection(t *testing.T) {
	tests := []struct {
		name    string
		kind    food.Kind
		section *food.ShopSection
		wantErr error
	}{
		{"leaf to frozen", food.KindLeaf, ptrSection(food.SectionFrozen), nil},
		{"leaf back to auto", food.KindLeaf, nil, nil},
		{"unknown section", food.KindLeaf, ptrSection("garden"), domain.ErrInvalidShopSection},
		{"empty section", food.KindLeaf, ptrSection(""), domain.ErrInvalidShopSection},
		{"composed food", food.KindComposed, ptrSection(food.SectionProduce), domain.ErrShopSectionLeafOnly},
		{"composed food back to auto", food.KindComposed, nil, domain.ErrShopSectionLeafOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{getFood: &food.Food{ID: 7, Kind: tt.kind}}
			f, err := food.NewService(repo).SetShopSection(context.Background(), 7, tt.section)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.False(t, repo.shopSectionSet, "nothing is written")
				return
			}
			require.NoError(t, err)
			assert.True(t, repo.shopSectionSet)
			assert.Equal(t, tt.section, f.ShopSection)
		})
	}
}

func TestShopSections_WalkOrder(t *testing.T) {
	assert.Equal(t, []food.ShopSection{
		food.SectionProduce, food.SectionBakery, food.SectionDairyEggs, food.SectionMeatFish,
		food.SectionDryGoods, food.SectionCansJars, food.SectionSpicesOils, food.SectionFrozen,
		food.SectionOther,
	}, food.ShopSections)
	for _, s := range food.ShopSections {
		assert.True(t, food.ValidShopSection(s), s)
	}
	assert.False(t, food.ValidShopSection("garden"))
}
