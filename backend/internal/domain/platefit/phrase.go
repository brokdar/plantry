package platefit

import (
	"slices"
	"strings"
)

// clashWords and gapWords name a raised dimension in a short phrase for the
// assistant and the chooser.
var (
	clashWords = map[Dimension]string{
		CuisineFit:  "mixed cuisines",
		Proportions: "two starches or two mains",
		Richness:    "too rich",
	}
	gapWords = map[Dimension]string{
		TasteBalance: "a sour note",
		Fresh:        "a fresh element",
		Texture:      "crunch",
		Colour:       "colour",
		Temperature:  "something cool",
		Pairing:      "a side that pairs",
		SauceStarch:  "a starch for the sauce",
	}
)

// ClashWords names the clash d ("" for a dimension that never clashes).
func ClashWords(d Dimension) string { return clashWords[d] }

// Phrase is r in a few words: "fits" when nothing is raised, else the
// clashes and the major gaps ("clash: too rich; lacks: crunch").
func Phrase(r Reading) string {
	var clashes, lacks []string
	for _, d := range r.Raised() {
		if w, ok := clashWords[d]; ok && slices.Contains(r.Clashes, d) {
			clashes = append(clashes, w)
		} else if w, ok := gapWords[d]; ok {
			lacks = append(lacks, w)
		}
	}
	var parts []string
	if len(clashes) > 0 {
		parts = append(parts, "clash: "+strings.Join(clashes, ", "))
	}
	if len(lacks) > 0 {
		parts = append(parts, "lacks: "+strings.Join(lacks, ", "))
	}
	if len(parts) == 0 {
		return "fits"
	}
	return strings.Join(parts, "; ")
}
