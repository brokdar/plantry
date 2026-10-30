package food_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func ptrInt(v int) *int    { return &v }
func ptrBool(v bool) *bool { return &v }

func TestService_Create_KeepDaysAndFreezable(t *testing.T) {
	composed := func(keep *int, freezable *bool) *food.Food {
		return &food.Food{
			Name: "Chili", Kind: food.KindComposed, Role: ptrRole(food.RoleMain),
			Children: []food.FoodComponent{{ChildID: 1, Amount: 200, Unit: "g"}},
			KeepDays: keep, Freezable: freezable,
		}
	}
	tests := []struct {
		name    string
		f       *food.Food
		wantErr bool
	}{
		{"composed without either", composed(nil, nil), false},
		{"composed keep 1", composed(ptrInt(1), nil), false},
		{"composed keep 7, not freezable", composed(ptrInt(7), ptrBool(false)), false},
		{"composed freezable", composed(nil, ptrBool(true)), false},
		{"composed keep 0", composed(ptrInt(0), nil), true},
		{"composed keep 8", composed(ptrInt(8), nil), true},
		{"leaf with keep days", &food.Food{Name: "Apple", Kind: food.KindLeaf, KeepDays: ptrInt(3)}, true},
		{"leaf with freezable", &food.Food{Name: "Apple", Kind: food.KindLeaf, Freezable: ptrBool(true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := food.NewService(&fakeRepo{}).Create(context.Background(), tt.f)
			if tt.wantErr {
				assert.True(t, errors.Is(err, domain.ErrInvalidInput), "got %v", err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
