package profile

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Kitchen is what the household cooks with and how much time it has (KIT).
type Kitchen struct {
	// MinutesWeekday and MinutesWeekend are the active cooking time allowed
	// for one cook (KIT-1).
	MinutesWeekday int
	MinutesWeekend int
	// Skill is "" while unknown; an unknown skill counts as home cook (KIT-2).
	Skill Skill
	// Equipment is the tools present. Oven and stovetop are the default; a
	// tool not in the list counts as absent (KIT-3).
	Equipment []string
	// Heat is "" while unknown (KIT-5).
	Heat     Heat
	Leanings []string
}

// Skill is the household's cooking skill.
type Skill string

const (
	SkillBeginner  Skill = "beginner"
	SkillHomeCook  Skill = "home_cook"
	SkillConfident Skill = "confident"
)

// Heat is the household's tolerance for spicy food.
type Heat string

const (
	HeatNone   Heat = "none"
	HeatMild   Heat = "mild"
	HeatMedium Heat = "medium"
	HeatHot    Heat = "hot"
)

// Defaults of the time allowances (KIT-1).
const (
	DefaultMinutesWeekday = 30
	DefaultMinutesWeekend = 60
)

// Bounds of the kitchen data.
const (
	MinAllowanceMinutes = 5
	MaxAllowanceMinutes = 480
	MaxLeanings         = 5
	MaxLeaningRunes     = 40
)

// EquipmentTools are the tools of the checklist (KIT-3).
var EquipmentTools = []string{
	"oven", "stovetop", "microwave", "air_fryer", "pressure_cooker", "slow_cooker",
	"blender", "food_processor", "stand_mixer", "wok", "grill",
}

// Allowance is the active minutes a cook may spend on a weekend day or on a
// batch cook day (KIT-1), else on a weekday.
func (k Kitchen) Allowance(weekend bool) int {
	if weekend {
		return k.MinutesWeekend
	}
	return k.MinutesWeekday
}

func (k Kitchen) validate() error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", domain.ErrInvalidKitchen, msg) }
	for _, m := range []int{k.MinutesWeekday, k.MinutesWeekend} {
		if m < MinAllowanceMinutes || m > MaxAllowanceMinutes {
			return bad(fmt.Sprintf("minutes must be between %d and %d", MinAllowanceMinutes, MaxAllowanceMinutes))
		}
	}
	switch k.Skill {
	case "", SkillBeginner, SkillHomeCook, SkillConfident:
	default:
		return bad("unknown skill")
	}
	switch k.Heat {
	case "", HeatNone, HeatMild, HeatMedium, HeatHot:
	default:
		return bad("unknown heat")
	}
	for i, t := range k.Equipment {
		if !slices.Contains(EquipmentTools, t) || slices.Contains(k.Equipment[:i], t) {
			return bad("unknown or repeated equipment " + t)
		}
	}
	if len(k.Leanings) > MaxLeanings {
		return bad(fmt.Sprintf("at most %d leanings", MaxLeanings))
	}
	for _, l := range k.Leanings {
		if l == "" || utf8.RuneCountInString(l) > MaxLeaningRunes {
			return bad("a leaning must be 1 to 40 characters")
		}
	}
	return nil
}

// normalised trims the leanings and drops blanks and repeats; nil lists
// become empty ones.
func (k Kitchen) normalised() Kitchen {
	out := make([]string, 0, len(k.Leanings))
	for _, l := range k.Leanings {
		l = strings.TrimSpace(l)
		if l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	k.Leanings = out
	if k.Equipment == nil {
		k.Equipment = []string{}
	}
	return k
}

// UpdateKitchen validates and persists the kitchen data.
func (s *Service) UpdateKitchen(ctx context.Context, k Kitchen) (*Profile, error) {
	k = k.normalised()
	if err := k.validate(); err != nil {
		return nil, err
	}
	return s.repo.UpdateKitchen(ctx, k)
}
