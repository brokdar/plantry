package food_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func ptrDiet(c diet.Class) *diet.Class { return &c }

func dietLeaf(id int64, name string) *food.Food {
	return &food.Food{ID: id, Kind: food.KindLeaf, Name: name}
}

func dietRecipe(id int64, children ...int64) *food.Food {
	f := &food.Food{ID: id, Kind: food.KindComposed, Name: "Recipe"}
	for _, c := range children {
		f.Children = append(f.Children, food.FoodComponent{ParentID: id, ChildID: c})
	}
	return f
}

func dietNodes(foods ...*food.Food) map[int64]*food.Food {
	m := map[int64]*food.Food{}
	for _, f := range foods {
		m[f.ID] = f
	}
	return m
}

// B3-AC2 on loaded trees.
func TestSetDietClasses(t *testing.T) {
	chickpeas, feta, speck, xyz := dietLeaf(1, "Kichererbsen"), dietLeaf(2, "Feta"), dietLeaf(3, "Speck"), dietLeaf(4, "Xyz")

	t.Run("leaves classify from their name", func(t *testing.T) {
		n := dietNodes(dietLeaf(1, "Kichererbsen"), dietLeaf(2, "Feta"))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Vegan, n[1].DietClass)
		assert.Equal(t, food.DietSourceAuto, n[1].DietClassSource)
		assert.Equal(t, diet.Vegetarian, n[2].DietClass)
	})
	t.Run("a recipe is the max of its children", func(t *testing.T) {
		n := dietNodes(chickpeas, feta, dietRecipe(10, 1, 2))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Vegetarian, n[10].DietClass)
		assert.Equal(t, food.DietSourceDerived, n[10].DietClassSource)
	})
	t.Run("adding Speck makes it omnivore", func(t *testing.T) {
		n := dietNodes(chickpeas, feta, speck, dietRecipe(10, 1, 2, 3))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Omnivore, n[10].DietClass)
	})
	t.Run("an override on the recipe wins", func(t *testing.T) {
		r := dietRecipe(10, 1, 2, 3)
		r.DietOverride = ptrDiet(diet.Vegan)
		n := dietNodes(chickpeas, feta, speck, r)
		food.SetDietClasses(n)
		assert.Equal(t, diet.Vegan, n[10].DietClass)
		assert.Equal(t, food.DietSourceUser, n[10].DietClassSource)
	})
	t.Run("an unknown leaf makes the recipe unknown", func(t *testing.T) {
		n := dietNodes(chickpeas, speck, xyz, dietRecipe(10, 1, 3, 4))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Unknown, n[10].DietClass)
	})
	t.Run("a leaf override feeds the recipe", func(t *testing.T) {
		x := dietLeaf(4, "Xyz")
		x.DietOverride = ptrDiet(diet.Vegan)
		n := dietNodes(chickpeas, x, dietRecipe(10, 1, 4))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Vegan, n[10].DietClass)
		assert.Equal(t, food.DietSourceUser, n[4].DietClassSource)
	})
	t.Run("nested recipes use the sub-recipe's class", func(t *testing.T) {
		n := dietNodes(chickpeas, feta, dietRecipe(10, 1, 2), dietRecipe(11, 10, 1))
		food.SetDietClasses(n)
		assert.Equal(t, diet.Vegetarian, n[11].DietClass)
	})
	t.Run("a recipe without children, a missing child and a cycle are unknown", func(t *testing.T) {
		n := dietNodes(dietRecipe(10), dietRecipe(11, 99), dietRecipe(12, 13), dietRecipe(13, 12))
		food.SetDietClasses(n)
		for _, id := range []int64{10, 11, 12, 13} {
			assert.Equal(t, diet.Unknown, n[id].DietClass, id)
		}
	})
}

func TestService_SetDietClass(t *testing.T) {
	tests := []struct {
		name    string
		class   *diet.Class
		wantErr error
	}{
		{"set vegan", ptrDiet(diet.Vegan), nil},
		{"clear", nil, nil},
		{"unknown is not settable", ptrDiet(diet.Unknown), domain.ErrInvalidDietClass},
		{"invalid", ptrDiet("paleo"), domain.ErrInvalidDietClass},
		{"empty", ptrDiet(""), domain.ErrInvalidDietClass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			f, err := food.NewService(repo).SetDietClass(context.Background(), 7, tt.class)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.False(t, repo.dietClassSet, "nothing is written")
				return
			}
			require.NoError(t, err)
			assert.True(t, repo.dietClassSet)
			assert.Equal(t, tt.class, f.DietOverride)
		})
	}
}

func TestService_List_InvalidDiet(t *testing.T) {
	for _, d := range []diet.Class{diet.Unknown, "paleo"} {
		_, err := food.NewService(&fakeRepo{}).List(context.Background(), food.ListQuery{Diet: d})
		require.ErrorIs(t, err, domain.ErrInvalidDietClass, d)
	}
}
