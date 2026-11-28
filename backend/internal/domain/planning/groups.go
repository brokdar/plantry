package planning

import (
	"regexp"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// DGE weekly food rules (soft, S7). Source: DGE "Gut essen und trinken"
// 2024, https://www.dge.de/gesunde-ernaehrung/gut-essen-und-trinken/dge-empfehlungen/
// meat and sausage at most 300 g per week, fish 1-2 times per week, legumes
// at least once per week, 5 portions of fruit and vegetables per day.
const (
	// DGEMeatMaxG is the weekly meat and sausage limit per eater.
	DGEMeatMaxG = 300
	// DGEFishMaxMeals is the weekly fish meals past which fish is too much (the
	// DGE asks for fish once or twice a week).
	DGEFishMaxMeals = 2
	// weekDays prorates the meat limit to the counted days of a window.
	weekDays = 7
	// minDGEDays is how many complete days the DGE week issues need.
	minDGEDays = 5
	// minMealG is the grams per eater of fish or legumes that make a plate a
	// fish or legume meal: a dash of fish sauce or a spoon of hummus is not.
	minMealG = 50
)

// FoodGroups is the grams of the DGE food groups in one portion of a
// recipe, or in 100 g of a leaf (the unit of FoodFacts.Macros).
type FoodGroups struct {
	MeatG, FishG, LegumeG float64
	// ProduceG is fruit and vegetables: leaves of the produce shop section
	// (override or shopping.Classify), potatoes excluded as the DGE does.
	// It is displayed per day only, never an issue: the DGE counts 5
	// portions of 100-200 g, and portion grams are too uncertain to judge.
	ProduceG float64
}

var potato = regexp.MustCompile(`(?i)kartoffel|potato|pommes`)

// GroupsOf sums the groups of leaves (grams as given).
func GroupsOf(leaves []food.LeafGrams) FoodGroups {
	var g FoodGroups
	for _, l := range leaves {
		switch diet.GroupOf(l.Name) {
		case diet.GroupMeat:
			g.MeatG += l.Grams
		case diet.GroupFish:
			g.FishG += l.Grams
		case diet.GroupLegume:
			g.LegumeG += l.Grams
		default:
			section := shopping.Classify(l.Name)
			if l.Section != nil {
				section = *l.Section
			}
			if section == food.SectionProduce && !potato.MatchString(l.Name) {
				g.ProduceG += l.Grams
			}
		}
	}
	return g
}

func (g FoodGroups) times(m float64) FoodGroups {
	return FoodGroups{g.MeatG * m, g.FishG * m, g.LegumeG * m, g.ProduceG * m}
}

func (g FoodGroups) add(o FoodGroups) FoodGroups {
	return FoodGroups{g.MeatG + o.MeatG, g.FishG + o.FishG, g.LegumeG + o.LegumeG, g.ProduceG + o.ProduceG}
}
