package importer

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// WarningChefCap: the month's Chef cap is reached; the import's chef_match
// call ran anyway because the user asked for it (COST-2).
const WarningChefCap = "chef_cap_reached"

// Matcher is food.Matcher, the one matching path (MATCH-4).
type Matcher interface {
	Match(ctx context.Context, req food.MatchRequest) (*food.MatchResult, error)
}

var (
	doubleOpenRE  = regexp.MustCompile(`\(\s*\(`)
	doubleCloseRE = regexp.MustCompile(`\)\s*\)`)
	noteRefRE     = regexp.MustCompile(`(?i)[(,;]?\s*\bnote\s+\d+\b(\s*sub)?\s*\)?`)
	boneSkinRE    = regexp.MustCompile(`(?i)\b(boneless|skinless)\b,?\s*`)
	rangeRE       = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*[-–]\s*(\d+(?:[.,]\d+)?)`)
	juiceOfRE     = regexp.MustCompile(`(?i)^juice of (\S+) (lime|lemon)s?\b`)
	cupMetricRE   = regexp.MustCompile(`(?i)\b(cups?|tbsp|tsp)\s*/\s*(\d+(?:[.,]\d+)?)\s*(ml|g)\b`)
	noteMetricRE  = regexp.MustCompile(`(?i)(?:^|[\s(;,])(\d+(?:[.,]\d+)?)\s*(g|ml)\b`)
	orSplitRE     = regexp.MustCompile(`(?i)\s+(?:or|oder)\s+`)
	andSplitRE    = regexp.MustCompile(`(?i)\s*(?:,|\s&\s|\sand\s|\sund\s)\s*`)
	// priceRE is a price a recipe site adds to a line: "($0.32)", "(1,20 €)".
	priceRE = regexp.MustCompile(`(?i)\s*\(\s*(?:[$€£]\s?\d[\d.,]*|\d[\d.,]*\s?(?:[$€£]|usd|eur|euro))\s*\)`)
	// barePriceRE is a price tag without brackets, trailing or inline:
	// "onion $0.47", "Ghee 1,20 €".
	barePriceRE = regexp.MustCompile(`(?i)\s+(?:[$€£]\s?\d+(?:[.,]\d+)*|\d+(?:[.,]\d+)*\s?(?:[$€£]|usd\b|eur\b|euro\b))(?:\s*/\s*[a-z]+\b|\*)*(?:\s|$|[,;)])`)
	// looseSeasoningRE is a seasoning measured in no unit: "a tiny pinch
	// cayenne", "a few turns of black pepper", "eine Prise Salz".
	looseSeasoningRE = regexp.MustCompile(`(?i)^(?:(?:an?|ein|eine)\s+)?(?:(?:tiny|small|little|big|kleine?|winzige)\s+)?(?:(?:pinch|dash|splash|sprinkle|prise|spritzer|hauch)|(?:few|ein paar|\d+)\s+(?:turns|grinds|twists|dashes|cranks?|umdrehungen))(?:\s+(?:of|von))?\s+(\S.*)$`)
	// canSizeRE is a can stated by its contents: "13oz. can X", "2 (15 oz)
	// cans X": the count and the ounces make the grams.
	canSizeRE = regexp.MustCompile(`(?i)^(?:(\d+(?:\.\d+)?)\s+)?\(?\s*(\d+(?:[.,]\d+)?)\s*(?:oz|ounces?)\.?\s*\)?\s*(?:cans?|tins?)\b\s*(?:of\s+)?(\S.*)$`)
	// inchGingerRE is fresh ginger measured in inches of root.
	inchGingerRE = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)[-\s]*inch(?:\s+piece)?(?:\s+of)?\s+((?:fresh\s+)?ginger\b.*)$`)
	residueRE    = regexp.MustCompile(`^[\s)(*\-–,;]+|[\s)(*\-–,;]+$`)
	// rangeToRE is a range written with a word: "3½ to 4 lbs.", "2 bis 3 EL".
	rangeToRE = regexp.MustCompile(`(?i)\b(\d+(?:[.,]\d+)?[½¼¾⅓⅔⅛]?)\s+(?:to|bis)\s+(\d+(?:[.,]\d+)?(?:[½¼¾⅓⅔⅛]|\b))`)
	// ozGramsRE is a weight stated in ounces and grams: "(5 oz, 140 g)"; the grams win.
	ozGramsRE = regexp.MustCompile(`(?i)\(\s*\d+(?:[.,]\d+)?\s*oz\.?\s*,\s*(\d+(?:[.,]\d+)?\s*g)\s*\)`)
	// gingerCmRE is fresh ginger counted as a piece and measured in cm: "1 Stück frischer Ingwer (ca. 2 cm)".
	gingerCmRE = regexp.MustCompile(`(?i)^(?:(\d+)\s+)?(?:stück|stücke|stk\.?|piece|pieces)\s+((?:frische[rn]?\s+)?(?:ingwer|ginger)\b[^(]*?)\s*\(\s*(?:ca\.?|etwa|about)?\s*(\d+(?:[.,]\d+)?)\s*cm\s*\)$`)
	// jeListRE is a German "je N a, b X" line: N of each of the listed kinds.
	jeListRE    = regexp.MustCompile(`(?i)^je\s+(\d+(?:[.,]\d+)?)\s+(\S.*)$`)
	listSplitRE = regexp.MustCompile(`(?i)\s*(?:,|\sund\s|\soder\s)\s*`)
)

const (
	gramsPerOz         = 28.35
	gramsPerInchGinger = 6.0 // a thumb-width inch of fresh root
)

// spoonCupUnits are volume units whose line may also state metric weight
// ("1 cup (100g) cheese"): the metric amount wins.
var spoonCupUnits = setOf("cup", "cups", "tbsp", "tsp", "el", "tl")

// headNouns complete a one-word alternative: "sunflower or vegetable oil"
// is sunflower oil or vegetable oil.
var headNouns = setOf("oil", "broth", "stock", "bread", "chillies", "chilies", "peppers", "pepper",
	"cheese", "vinegar", "sugar", "flour", "rice", "tortillas", "öl", "brühe",
	"tomatoes", "tomato", "onions", "potatoes", "mushrooms", "beans", "lentils", "tomaten", "zwiebeln")

// prepareLine rewrites what the line parser cannot read (MATCH-1):
// doubled parentheses and note references, a range as its midpoint,
// "juice of 1/2 lime", "1/2 cup / 125 ml" (metric wins), cut words.
func prepareLine(raw string) string {
	s := strings.TrimSpace(raw)
	if m := canSizeRE.FindStringSubmatch(s); m != nil {
		n := 1.0
		if m[1] != "" {
			n, _ = strconv.ParseFloat(m[1], 64)
		}
		oz, _ := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", "."), 64)
		s = strconv.FormatFloat(math.Round(n*oz*gramsPerOz), 'f', -1, 64) + " g " + m[3]
	}
	if m := inchGingerRE.FindStringSubmatch(s); m != nil {
		in, _ := strconv.ParseFloat(m[1], 64)
		s = strconv.FormatFloat(math.Round(in*gramsPerInchGinger), 'f', -1, 64) + " g " + m[2]
	}
	if m := gingerCmRE.FindStringSubmatch(s); m != nil {
		cm, _ := strconv.ParseFloat(strings.ReplaceAll(m[3], ",", "."), 64)
		n := 1.0
		if m[1] != "" {
			n, _ = strconv.ParseFloat(m[1], 64)
		}
		s = strconv.FormatFloat(math.Round(n*cm/2.54*gramsPerInchGinger), 'f', -1, 64) + " g " + m[2]
	}
	s = ozGramsRE.ReplaceAllString(s, "($1)")
	s = rangeToRE.ReplaceAllStringFunc(s, func(m string) string {
		p := rangeToRE.FindStringSubmatch(m)
		a, okA := mixedNumber(p[1])
		b, okB := mixedNumber(p[2])
		if !okA || !okB || b <= a {
			return m
		}
		return strconv.FormatFloat((a+b)/2, 'f', -1, 64)
	})
	if m := looseSeasoningRE.FindStringSubmatch(s); m != nil {
		s = m[1] + ", to taste"
	}
	s = doubleOpenRE.ReplaceAllString(s, "(")
	s = doubleCloseRE.ReplaceAllString(s, ")")
	s = noteRefRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "*", "")
	s = boneSkinRE.ReplaceAllString(s, "")
	s = rangeRE.ReplaceAllStringFunc(s, func(m string) string {
		p := rangeRE.FindStringSubmatch(m)
		a, errA := strconv.ParseFloat(strings.ReplaceAll(p[1], ",", "."), 64)
		b, errB := strconv.ParseFloat(strings.ReplaceAll(p[2], ",", "."), 64)
		if errA != nil || errB != nil || b <= a {
			return m
		}
		return strconv.FormatFloat((a+b)/2, 'f', -1, 64)
	})
	s = juiceOfRE.ReplaceAllString(strings.TrimSpace(s), "$1 $2 juice")
	s = cupMetricRE.ReplaceAllString(s, "$2 $3")
	return s
}

// StripPrice removes a price annotation such as "($0.32)" from a line.
func StripPrice(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ") // a site's &nbsp; before the tag
	s = priceRE.ReplaceAllString(s, "")
	s = barePriceRE.ReplaceAllStringFunc(s, func(m string) string {
		if end := m[len(m)-1]; end == ',' || end == ';' || end == ')' {
			return string(end)
		}
		return " "
	})
	return strings.Join(strings.Fields(s), " ")
}

// ParseLines parses and cleans a recipe's ingredient lines (MATCH-1): one
// draft row per food, so "Salz und Pfeffer" and "1 tsp EACH paprika and
// cumin" become two rows; alternatives ("Ghee (oder Butterschmalz)") keep
// the first as the name; a line without an amount is to taste.
func ParseLines(raw []string, lang string) []DraftIngredient {
	out := make([]DraftIngredient, 0, len(raw))
	for _, line := range raw {
		out = append(out, ParseLine(line, lang)...)
	}
	return out
}

// ParseLine parses and cleans one ingredient line into one row per food.
func ParseLine(raw, lang string) []DraftIngredient {
	raw = StripPrice(raw)
	if lines := expandJeList(raw); lines != nil {
		var rows []DraftIngredient
		for _, l := range lines {
			for _, r := range ParseLine(l, lang) {
				r.RawText = raw
				rows = append(rows, r)
			}
		}
		return rows
	}
	prepared := prepareLine(raw)
	var p DraftIngredient
	if lang == "en" {
		p = ParseLineEN(prepared)
	} else {
		p = ParseLineDE(prepared)
	}
	p.RawText = raw
	p.Name = cleanResidue(p.Name)
	p.Note = cleanResidue(p.Note)
	metricFromNote(&p)

	var rows []DraftIngredient
	switch name, isEach := strings.CutPrefix(p.Name, "EACH "); {
	case isEach || strings.HasPrefix(p.Name, "each "):
		if !isEach {
			name = strings.TrimPrefix(p.Name, "each ")
		}
		full := name
		if p.Note != "" && !strings.Contains(p.Note, "(") {
			full += ", " + p.Note
			p.Note = ""
		}
		rows = splitCompound(p, full)
	case p.Amount == 0 && andSplitRE.MatchString(p.Name) && !orSplitRE.MatchString(p.Name):
		rows = splitCompound(p, p.Name)
	default:
		rows = []DraftIngredient{p}
	}
	for i := range rows {
		splitAlternatives(&rows[i])
		if rows[i].Amount == 0 {
			noAmount(&rows[i])
		}
		rows[i].DietClass = diet.Classify(rows[i].Name)
	}
	return rows
}

// mixedNumber reads "4", "3,5" and "3½".
func mixedNumber(tok string) (float64, bool) {
	r := []rune(tok)
	frac := 0.0
	if f, ok := unicodeFractions[r[len(r)-1]]; ok && len(r) > 1 {
		frac, tok = f, string(r[:len(r)-1])
	}
	v, ok := parseSingleNumber(tok)
	return v + frac, ok
}

// expandJeList turns "je 1 kleine rote, gelbe Paprikaschote" into one line
// per kind with the count each ("1 kleine rote Paprikaschote", "1 kleine
// gelbe Paprikaschote"): a size word of the first kind and the noun at the
// end are shared. nil when the line is not such a list.
func expandJeList(raw string) []string {
	m := jeListRE.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return nil
	}
	parts := listSplitRE.Split(m[2], -1)
	if len(parts) < 2 {
		return nil
	}
	last := strings.Fields(parts[len(parts)-1])
	if len(last) < 2 {
		return nil
	}
	noun := last[len(last)-1]
	size := ""
	if first := strings.Fields(parts[0]); len(first) > 1 {
		if _, ok := sizeWords[strings.ToLower(first[0])]; ok {
			size, parts[0] = first[0]+" ", strings.Join(first[1:], " ")
		}
	}
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			p += " " + noun
		}
		out = append(out, m[1]+" "+size+p)
	}
	return out
}

// proposedAmounts are the amounts a quantity-less line of a group that is
// not a seasoning gets (MATCH-6: "oil for frying" needs an amount). The
// review shows them as proposed; the user can change them.
var proposedAmounts = map[string]struct {
	amount float64
	unit   string
}{
	"fat":       {1, "tbsp"},
	"condiment": {1, "tbsp"},
	"sugar":     {1, "tsp"},
}

// noAmount settles a line without an amount: a seasoning is "to taste", a
// food the table knows gets a proposed amount, anything else stays without
// one and the review asks for it.
func noAmount(r *DraftIngredient) {
	group := food.LineGroup(r.Name, r.Note)
	if food.IsToTasteGroup(group) {
		r.ToTaste = true
		return
	}
	if p, ok := proposedAmounts[group]; ok {
		r.Amount, r.Unit, r.Proposed = p.amount, p.unit, true
	}
}

// SettleQuantityless makes a matched line that Finalize would refuse (no
// amount or unit and no matched grams) saveable the way the review does: a
// seasoning or a food of no known group is "to taste", a fat, condiment or
// sugar gets its proposed amount. It reports false when the line stays
// unsaveable (the review must ask for the amount).
func SettleQuantityless(ing *DraftIngredient) bool {
	m := ing.Match
	if m == nil || m.Status != food.MatchMatched || m.Pick < 0 || m.Pick >= len(m.Candidates) {
		return true // an unmatched line is blocked on its own
	}
	if ing.ToTaste || (ing.Amount > 0 && ing.Unit != "") || m.Grams != nil {
		return true
	}
	group := food.LineGroup(m.Candidates[m.Pick].Name, "")
	if group == "" || food.IsToTasteGroup(group) {
		ing.ToTaste = true
		return true
	}
	if p, ok := proposedAmounts[group]; ok {
		ing.Amount, ing.Unit, ing.Proposed = p.amount, p.unit, true
		return true
	}
	return false
}

// SettlePinches makes a seasoning of more than two pinches ("10-15 dashes hot
// sauce" parses to 12.5 pinch) "to taste": a count of pinches that large is
// no amount. A matched food of a non-seasoning group keeps its line.
func SettlePinches(ing *DraftIngredient) {
	if ing.ToTaste || units.Normalize(ing.Unit) != "pinch" || ing.Amount <= 2 {
		return
	}
	if m := ing.Match; m != nil && m.Status == food.MatchMatched && m.Pick >= 0 && m.Pick < len(m.Candidates) {
		if g := food.LineGroup(m.Candidates[m.Pick].Name, ""); g != "" && !food.IsToTasteGroup(g) {
			return
		}
	}
	ing.ToTaste = true
}

func splitCompound(p DraftIngredient, names string) []DraftIngredient {
	var rows []DraftIngredient
	for _, n := range andSplitRE.Split(names, -1) {
		if n = cleanResidue(n); n != "" {
			r := p
			r.Name = n
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return []DraftIngredient{p}
	}
	return rows
}

// splitAlternatives keeps the first of "X or Y" as the name and the rest as
// alternatives, also from a note that starts with "oder"/"or".
func splitAlternatives(p *DraftIngredient) {
	parts := orSplitRE.Split(p.Name, -1)
	if len(parts) > 1 {
		last := strings.Fields(parts[len(parts)-1])
		if len(last) >= 2 {
			head := strings.ToLower(last[len(last)-1])
			if _, ok := headNouns[head]; ok {
				for i := range parts[:len(parts)-1] {
					if len(strings.Fields(parts[i])) == 1 {
						parts[i] += " " + last[len(last)-1]
					}
				}
			}
		}
		p.Name = cleanResidue(parts[0])
		for _, a := range parts[1:] {
			if a = cleanResidue(a); a != "" {
				p.Alternatives = append(p.Alternatives, a)
			}
		}
	}
	low := strings.ToLower(p.Note)
	for _, lead := range []string{"oder ", "or "} {
		if strings.HasPrefix(low, lead) {
			alt := p.Note[len(lead):]
			if i := strings.IndexAny(alt, ",;("); i >= 0 {
				alt = alt[:i]
			}
			if alt = cleanResidue(alt); alt != "" {
				p.Alternatives = append(p.Alternatives, alt)
			}
		}
	}
}

// metricFromNote takes a metric amount the line states next to a spoon or
// cup measure ("1 cup (100g) cheese", "2 cups (500ml) stock").
func metricFromNote(p *DraftIngredient) {
	if _, ok := spoonCupUnits[strings.ToLower(p.OriginalUnit)]; !ok {
		return
	}
	m := noteMetricRE.FindStringSubmatch(p.Note)
	if m == nil {
		return
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil || v <= 0 {
		return
	}
	p.Amount, p.Unit, p.OriginalUnit, p.Confidence = v, strings.ToLower(m[2]), strings.ToLower(m[2]), ConfidenceParsed
}

// cleanResidue trims leftover brackets, stars and dashes and a doubled word
// ("eggs eggs").
func cleanResidue(s string) string {
	s = strings.TrimSpace(residueRE.ReplaceAllString(s, ""))
	w := strings.Fields(s)
	if len(w) == 2 && strings.EqualFold(w[0], w[1]) {
		return w[0]
	}
	return strings.Join(w, " ")
}

// MatchLineOf is a draft row as the matcher reads it. A line still without
// an amount matches its food like a to-taste line: the amount is the
// review's to enter (MATCH-6), not a reason the food is unknown.
func MatchLineOf(ing DraftIngredient) food.MatchLine {
	return food.MatchLine{
		Name: ing.Name, Alternatives: ing.Alternatives, Note: ing.Note,
		Amount: ing.Amount, Unit: ing.Unit, PortionGrams: ing.PortionGrams, ToTaste: ing.ToTaste || ing.Amount == 0,
	}
}

// matchDraft matches every row of d in one pass (one chef_match call with a
// provider) and fills each row's match. A count row nobody could weigh
// takes the matched food's weight per piece.
func (s *Service) matchDraft(ctx context.Context, d *Draft) {
	if s.matcher == nil || len(d.Ingredients) == 0 {
		return
	}
	req := food.MatchRequest{Lang: d.Language, Requested: true, Ref: d.SourceURL}
	for _, ing := range d.Ingredients {
		req.Lines = append(req.Lines, MatchLineOf(ing))
	}
	res, err := s.matcher.Match(ctx, req)
	if err != nil {
		logStage(ctx, StageResolve, "match_error", err.Error())
		return
	}
	for i := range d.Ingredients {
		m := res.Lines[i]
		ing := &d.Ingredients[i]
		ing.Match = &m
		if ing.NeedsWeight && m.PortionGrams != nil {
			ing.PortionGrams, ing.NeedsWeight = m.PortionGrams, false
		}
	}
	if res.CapWarning {
		d.Warnings = append(d.Warnings, WarningChefCap)
	}
}
