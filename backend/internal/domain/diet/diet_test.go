package diet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

func TestMax(t *testing.T) {
	tests := []struct {
		name string
		in   []diet.Class
		want diet.Class
	}{
		{"empty is unknown", nil, diet.Unknown},
		{"single", []diet.Class{diet.Vegan}, diet.Vegan},
		{"vegan and vegetarian", []diet.Class{diet.Vegan, diet.Vegetarian}, diet.Vegetarian},
		{"omnivore wins", []diet.Class{diet.Vegetarian, diet.Omnivore, diet.Pescetarian}, diet.Omnivore},
		{"pescetarian over vegetarian", []diet.Class{diet.Pescetarian, diet.Vegetarian}, diet.Pescetarian},
		{"unknown absorbs", []diet.Class{diet.Omnivore, diet.Unknown}, diet.Unknown},
		{"invalid is unknown", []diet.Class{diet.Vegan, "paleo"}, diet.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, diet.Max(tt.in...))
		})
	}
}

func TestCompatible(t *testing.T) {
	tests := []struct {
		class, dietType diet.Class
		want            bool
	}{
		{diet.Vegan, diet.Vegan, true},
		{diet.Vegetarian, diet.Vegan, false},
		{diet.Vegan, diet.Vegetarian, true},
		{diet.Vegetarian, diet.Vegetarian, true},
		{diet.Pescetarian, diet.Vegetarian, false},
		{diet.Pescetarian, diet.Pescetarian, true},
		{diet.Omnivore, diet.Pescetarian, false},
		{diet.Omnivore, diet.Omnivore, true},
		{diet.Unknown, diet.Omnivore, false},
		{diet.Vegan, diet.Unknown, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.class)+"/"+string(tt.dietType), func(t *testing.T) {
			assert.Equal(t, tt.want, diet.Compatible(tt.class, tt.dietType))
		})
	}
}

func TestValid(t *testing.T) {
	for _, c := range diet.Order {
		assert.True(t, diet.Valid(c), c)
	}
	assert.False(t, diet.Valid(diet.Unknown))
	assert.False(t, diet.Valid(""))
}
