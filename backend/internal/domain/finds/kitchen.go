package finds

import (
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Origin thresholds (FIND-13, tunable).
const (
	lightSwaps  = 2
	lightShare  = 0.15
	mediumSwaps = 4
	mediumShare = 0.30
	// MaxNewSpecialty is how many specialty ingredients new to the household
	// one find may bring (FIND-17, tunable; counted per find).
	MaxNewSpecialty = 2
)

// OriginOf labels an adaptation by its swaps, protein swaps and the share
// of ingredient weight they touched (FIND-13).
func OriginOf(swaps, proteinSwaps int, share float64) Origin {
	switch {
	case swaps <= lightSwaps && proteinSwaps == 0 && share <= lightShare:
		return OriginAdaptedLight
	case swaps <= mediumSwaps && proteinSwaps <= 1 && share <= mediumShare:
		return OriginAdaptedMedium
	}
	return OriginInspired
}

// Requirements are what a recipe asks of the kitchen, as the adapt step
// reports them.
type Requirements struct {
	Equipment     []string `json:"equipment"`
	ActiveMinutes int      `json:"active_minutes"`
	Advanced      bool     `json:"advanced"`
	Specialty     []string `json:"specialty"`
}

// Kitchen problems (FIND-16, FIND-17).
const (
	ProblemTooLong   = "too_long"
	ProblemSkill     = "skill"
	ProblemSpecialty = "specialty"
	// ProblemEquipment is followed by ":" and the tool.
	ProblemEquipment = "equipment"
	// QuestionSkill and "equipment:<tool>" are the KIT-2/3 questions.
	QuestionSkill = "skill"
)

// KitchenFit lists what keeps the recipe from fitting the kitchen (FIND-16:
// a missing tool, active time over the longest allowance, an advanced
// technique for a skill below confident; FIND-17: more than MaxNewSpecialty
// specialty ingredients the household has not used) and the KIT-2/3
// questions not asked yet (an unknown tool counts as absent; an unknown
// skill as home cook).
func KitchenFit(req Requirements, k profile.Kitchen, knownLeaves, asked []string) (problems, questions []string) {
	for _, tool := range req.Equipment {
		if !slices.Contains(profile.EquipmentTools, tool) || slices.Contains(k.Equipment, tool) {
			continue
		}
		problems = append(problems, ProblemEquipment+":"+tool)
		if q := ProblemEquipment + ":" + tool; !slices.Contains(asked, q) {
			questions = append(questions, q)
		}
	}
	allow := max(k.MinutesWeekday, k.MinutesWeekend)
	if allow == 0 {
		allow = profile.DefaultMinutesWeekend
	}
	if req.ActiveMinutes > allow {
		problems = append(problems, ProblemTooLong)
	}
	if req.Advanced && k.Skill != profile.SkillConfident {
		problems = append(problems, ProblemSkill)
		if k.Skill == "" && !slices.Contains(asked, QuestionSkill) {
			questions = append(questions, QuestionSkill)
		}
	}
	if newSpecialty(req.Specialty, knownLeaves) > MaxNewSpecialty {
		problems = append(problems, ProblemSpecialty)
	}
	return problems, questions
}

func newSpecialty(names, known []string) int {
	n := 0
	for _, s := range names {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !slices.ContainsFunc(known, func(k string) bool { return strings.Contains(strings.ToLower(k), s) }) {
			n++
		}
	}
	return n
}
