package finds_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

// NFR-5: the query type can only carry dish, cuisine, diet and language.
func TestQuery_CarriesNoProfileData(t *testing.T) {
	var fields []string
	typ := reflect.TypeFor[finds.Query]()
	for i := range typ.NumField() {
		fields = append(fields, typ.Field(i).Name)
	}
	assert.Equal(t, []string{"Dish", "Cuisine", "Diet", "Language"}, fields)
}

func TestQuery_Text(t *testing.T) {
	q := finds.Query{Dish: "  Linsensuppe\n", Cuisine: "german", Diet: "vegetarian", Language: "de"}
	assert.Equal(t, "recipe: Linsensuppe; cuisine: german; diet: vegetarian; language: de", q.Text())
	assert.Equal(t, "recipe: tacos", finds.Query{Dish: "tacos"}.Text())
	assert.LessOrEqual(t, len([]rune(finds.Query{Dish: strings.Repeat("ä", 500)}.Text())), len("recipe: ")+80)
}
