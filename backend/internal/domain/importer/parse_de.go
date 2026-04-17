// Package importer turns a URL or HTML blob into a recipe Draft, ready for
// user review.
package importer

import (
	"regexp"
	"strconv"
	"strings"
)

// unicodeFractions maps standalone Unicode vulgar fractions to their numeric value.
var unicodeFractions = map[rune]float64{
	'½': 0.5,
	'¼': 0.25,
	'¾': 0.75,
	'⅓': 1.0 / 3.0,
	'⅔': 2.0 / 3.0,
	'⅕': 0.2,
	'⅖': 0.4,
	'⅗': 0.6,
	'⅘': 0.8,
	'⅙': 1.0 / 6.0,
	'⅚': 5.0 / 6.0,
	'⅛': 0.125,
	'⅜': 0.375,
	'⅝': 0.625,
	'⅞': 0.875,
}

// leadingQualifiers are stripped from the start of a line without contributing to amount or note.
var leadingQualifiers = map[string]struct{}{
	"ca.":       {},
	"ca":        {},
	"etwa":      {},
	"ungefähr":  {},
	"ungefaehr": {},
	"circa":     {},
	"zirka":     {},
}

// toTasteMarkers are multi-word qualifiers that set confidence=unparsed and
// land as Note. Keys match lowercased concatenated prefix.
var toTasteMarkers = []string{
	"nach geschmack",
	"nach belieben",
	"nach bedarf",
}

// singleTasteMarkers are single-token qualifiers with the same effect.
var singleTasteMarkers = map[string]string{
	"etwas":    "etwas",
	"wenig":    "wenig",
	"bisschen": "ein bisschen",
}

var whitespaceRE = regexp.MustCompile(`\s+`)

// ParseLineDE parses a German-language ingredient line into a DraftIngredient.
// The parser is best-effort: lines it cannot interpret still produce a
// DraftIngredient with Confidence=ConfidenceUnparsed that the UI can show for
// manual correction.
func ParseLineDE(raw string) DraftIngredient {
	out := DraftIngredient{RawText: raw, Confidence: ConfidenceUnparsed}

	s := strings.TrimSpace(raw)
	s = whitespaceRE.ReplaceAllString(s, " ")
	if s == "" {
		return out
	}

	// Pull parenthetical content into a note candidate and strip it from the working string.
	s, parenNote := extractParenthetical(s)

	// Split on first comma (not inside parens, but parens already stripped).
	main, commaNote := splitMainAndNote(s)

	// Assemble note: parenthetical first, comma-note second, dedup whitespace.
	note := joinNote(parenNote, commaNote)

	// Check qualitative prefixes that preclude amount/unit parsing.
	if tasted, marker := matchToTasteMarker(main); tasted {
		nm := strings.TrimSpace(strings.TrimPrefix(lowerPrefix(main, marker), ""))
		// remove the marker from the head of `main`, preserving original case for name.
		rest := main[len(marker):]
		name := strings.TrimSpace(rest)
		if name == "" {
			name = strings.TrimSpace(nm)
		}
		out.Name = name
		out.Note = mergeNote(marker, note)
		out.Confidence = ConfidenceUnparsed
		return out
	}

	// Strip leading qualifiers like "ca.", "etwa".
	main = stripLeadingQualifiers(main)

	// Check single-token taste marker (e.g. "etwas Pfeffer").
	tokens := strings.Fields(main)
	if len(tokens) > 0 {
		if canonical, ok := singleTasteMarkers[strings.ToLower(tokens[0])]; ok {
			out.Name = strings.TrimSpace(strings.Join(tokens[1:], " "))
			out.Note = mergeNote(canonical, note)
			out.Confidence = ConfidenceUnparsed
			return out
		}
	}

	amount, amountApprox, consumed, ok := parseAmount(tokens)
	if !ok {
		// No amount — whole remainder is the name; note is whatever we already collected.
		out.Name = strings.TrimSpace(main)
		out.Note = note
		return out
	}

	tokens = tokens[consumed:]

	// Peel the unit token, if any.
	unitInfo, unitToken, hasUnit := matchUnit(tokens)
	if hasUnit {
		tokens = tokens[1:]
	}

	name := strings.TrimSpace(strings.Join(tokens, " "))

	// Compound-word suffix match: "Knoblauchzehe(n)" is really a Zehe count.
	// Only fires when no explicit unit was peeled.
	if !hasUnit {
		if info, disp, matched := matchCompoundUnit(name); matched {
			unitInfo = info
			unitToken = disp
			hasUnit = true
		}
	}

	switch {
	case hasUnit:
		out.Amount = amount * unitInfo.FactorToBase
		out.Unit = unitInfo.Canonical
		out.OriginalUnit = preserveUnitCase(unitToken, unitInfo.Display)
		if amountApprox || unitInfo.Approximate {
			out.Confidence = ConfidenceApproximate
		} else {
			out.Confidence = ConfidenceParsed
		}
	default:
		// Unit-less count → treat as Stück.
		out.Amount = amount * stückFactor
		out.Unit = "g"
		out.OriginalUnit = stückDisplay
		out.Confidence = ConfidenceApproximate
	}

	// "3 EL, gestr. Mehl" leaves name="" and note="gestr. Mehl" after the split.
	// When that happens, promote note to name — the real ingredient word sits
	// after the modifier in this chefkoch-style pattern.
	if name == "" && note != "" {
		name, note = note, ""
	}

	out.Name = name
	out.Note = note
	return out
}

// ParseLineEN is a thin English fallback. v1 does not ship a full English
// parser; it recognizes a minimal set of units so the draft still renders.
func ParseLineEN(raw string) DraftIngredient {
	// For v1 we reuse the German pipeline: the token structure is similar
	// (amount unit name) and the alias table covers g/ml already. Callers
	// pass the original language through DetectLanguage, so non-matching
	// English units fall through to unparsed — which is what we want.
	return ParseLineDE(raw)
}

// DetectLanguage classifies a set of ingredient lines as German, English, or unknown
// using a small German stopword heuristic.
func DetectLanguage(lines []string) string {
	if len(lines) == 0 {
		return "unknown"
	}
	deMarkers := []string{
		"teelöffel", "esslöffel", "nach", "zehen", "prise", "stück", "bund",
		" tl ", " el ", "knoblauch", "mehl", "salz", "pfeffer",
	}
	enMarkers := []string{
		"tablespoon", "teaspoon", "cup", "cups", "clove", "cloves", "ounce",
		"pinch", "pound",
	}

	joined := " " + strings.ToLower(strings.Join(lines, " ")) + " "
	de, en := 0, 0
	for _, m := range deMarkers {
		if strings.Contains(joined, m) {
			de++
		}
	}
	for _, m := range enMarkers {
		if strings.Contains(joined, m) {
			en++
		}
	}
	switch {
	case de == 0 && en == 0:
		return "unknown"
	case de >= en:
		return "de"
	default:
		return "en"
	}
}

// -- helpers --

// extractParenthetical removes every parenthetical substring from s and returns
// the rest plus the joined note content. Chefkoch ingredient lines routinely
// carry both a plural marker "(n)" AND a prose comment "(etwa gleich viel ...)".
func extractParenthetical(s string) (rest, note string) {
	var notes []string
	for {
		open := strings.Index(s, "(")
		if open < 0 {
			break
		}
		close := strings.Index(s[open:], ")")
		if close < 0 {
			break
		}
		close += open
		inner := strings.TrimSpace(s[open+1 : close])
		if inner != "" {
			notes = append(notes, inner)
		}
		s = strings.TrimSpace(s[:open] + " " + s[close+1:])
	}
	return whitespaceRE.ReplaceAllString(s, " "), strings.Join(notes, "; ")
}

// splitMainAndNote splits on the first ", " (comma + space) into the working
// "main" string and a trailing note. Bare commas inside numbers like "0,5" are
// not split.
func splitMainAndNote(s string) (main, note string) {
	i := strings.Index(s, ", ")
	if i < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+2:])
}

func joinNote(parts ...string) string {
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, "; ")
}

func mergeNote(lead, existing string) string {
	if existing == "" {
		return lead
	}
	return lead + "; " + existing
}

func stripLeadingQualifiers(s string) string {
	// Repeatedly peel a single leading qualifier token if present.
	for {
		tokens := strings.Fields(s)
		if len(tokens) == 0 {
			return s
		}
		low := strings.ToLower(tokens[0])
		if _, ok := leadingQualifiers[low]; !ok {
			return s
		}
		s = strings.TrimSpace(strings.Join(tokens[1:], " "))
	}
}

// matchToTasteMarker returns (true, marker) when s starts with a multi-word
// to-taste phrase (matching the original casing of s up to marker length).
func matchToTasteMarker(s string) (bool, string) {
	low := strings.ToLower(s)
	for _, m := range toTasteMarkers {
		if strings.HasPrefix(low, m) {
			// Return the matched prefix from the original string to preserve case.
			return true, s[:len(m)]
		}
	}
	return false, ""
}

func lowerPrefix(s, prefix string) string {
	return strings.ToLower(s[:len(prefix)])
}

// parseAmount consumes one or two leading tokens and returns (amount, approximate, consumedTokens, ok).
// Recognized forms:
//   - unicode fraction standalone: "½"
//   - mixed unicode glued:          "1½"
//   - ascii fraction:               "1/2"
//   - mixed ascii (two tokens):     "1 1/2"
//   - decimal comma/dot:            "0,5" / "0.5"
//   - integer:                      "200"
//   - range:                        "1-2" / "1–2" (midpoint, approximate)
func parseAmount(tokens []string) (amount float64, approximate bool, consumed int, ok bool) {
	if len(tokens) == 0 {
		return 0, false, 0, false
	}
	first := tokens[0]

	// Range: contains "-" or "–" between two numeric halves.
	for _, sep := range []string{"-", "–"} {
		if strings.Contains(first, sep) {
			parts := strings.SplitN(first, sep, 2)
			if len(parts) == 2 {
				a, okA := parseSingleNumber(parts[0])
				b, okB := parseSingleNumber(parts[1])
				if okA && okB {
					return (a + b) / 2, true, 1, true
				}
			}
		}
	}

	// Mixed unicode glued, e.g. "1½".
	if v, rest, ok := splitLeadingDigitsAndFraction(first); ok && rest != "" {
		if frac, fok := parseSingleNumber(rest); fok {
			return v + frac, false, 1, true
		}
	}

	// Single-token number (integer, decimal, fraction, unicode fraction).
	if v, ok := parseSingleNumber(first); ok {
		// Check for mixed-ascii: "1" followed by "N/D".
		if len(tokens) >= 2 && isFractionLiteral(tokens[1]) {
			if frac, fok := parseSingleNumber(tokens[1]); fok {
				return v + frac, false, 2, true
			}
		}
		return v, false, 1, true
	}

	return 0, false, 0, false
}

// parseSingleNumber parses integers, decimals (comma or dot), ASCII fractions
// "N/D", and single Unicode fractions.
func parseSingleNumber(tok string) (float64, bool) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return 0, false
	}

	// Single unicode fraction character.
	runes := []rune(tok)
	if len(runes) == 1 {
		if v, ok := unicodeFractions[runes[0]]; ok {
			return v, true
		}
	}

	// ASCII fraction "N/D".
	if strings.Contains(tok, "/") && !strings.Contains(tok, " ") {
		parts := strings.SplitN(tok, "/", 2)
		if len(parts) == 2 {
			num, errN := strconv.ParseFloat(parts[0], 64)
			den, errD := strconv.ParseFloat(parts[1], 64)
			if errN == nil && errD == nil && den != 0 {
				return num / den, true
			}
		}
	}

	// Decimal with comma.
	if strings.Contains(tok, ",") {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(tok, ",", "."), 64); err == nil {
			return v, true
		}
	}

	if v, err := strconv.ParseFloat(tok, 64); err == nil {
		return v, true
	}

	return 0, false
}

func isFractionLiteral(tok string) bool {
	if strings.Contains(tok, "/") {
		parts := strings.SplitN(tok, "/", 2)
		if len(parts) != 2 {
			return false
		}
		if _, errN := strconv.ParseFloat(parts[0], 64); errN != nil {
			return false
		}
		if _, errD := strconv.ParseFloat(parts[1], 64); errD != nil {
			return false
		}
		return true
	}
	if r := []rune(tok); len(r) == 1 {
		_, ok := unicodeFractions[r[0]]
		return ok
	}
	return false
}

// splitLeadingDigitsAndFraction splits a token like "1½" into integer "1" and rest "½".
// Returns v, rest, ok. If the token is not a digit-prefixed run, ok=false.
func splitLeadingDigitsAndFraction(tok string) (float64, string, bool) {
	var digits []rune
	rest := tok
	for i, r := range tok {
		if r < '0' || r > '9' {
			rest = tok[i:]
			break
		}
		digits = append(digits, r)
		rest = tok[i+len(string(r)):]
	}
	if len(digits) == 0 {
		return 0, tok, false
	}
	if rest == tok {
		// Whole token was digits.
		return 0, "", false
	}
	v, err := strconv.ParseFloat(string(digits), 64)
	if err != nil {
		return 0, tok, false
	}
	return v, rest, true
}

// compoundUnitSuffixes lists count-style units that commonly appear as the tail
// of a German compound word (e.g. "Knoblauchzehe" ending in "-zehe"). Ordered
// longest-suffix-first so "zehen" beats "zehe" on plural forms.
var compoundUnitSuffixes = []string{
	"knoblauchzehen", "knoblauchzehe", "zehen", "zehe",
	"scheiben", "scheibe",
	"stangen", "stange",
}

// matchCompoundUnit checks whether the (lowercased) first whitespace-delimited
// word of name ends with one of the compound suffixes. Returns the matching
// unitInfo + display form when found.
func matchCompoundUnit(name string) (unitInfo, string, bool) {
	first := strings.ToLower(strings.SplitN(name, " ", 2)[0])
	for _, suf := range compoundUnitSuffixes {
		if len(first) > len(suf) && strings.HasSuffix(first, suf) {
			if info, ok := aliasesDE[suf]; ok {
				return info, info.Display, true
			}
		}
	}
	return unitInfo{}, "", false
}

// matchUnit looks at the first token against the alias table.
// Lookup is case-insensitive and ignores a single trailing period ("Stk." → "stk").
func matchUnit(tokens []string) (unitInfo, string, bool) {
	if len(tokens) == 0 {
		return unitInfo{}, "", false
	}
	raw := tokens[0]
	key := strings.ToLower(raw)
	key = strings.TrimSuffix(key, ".")
	if info, ok := aliasesDE[key]; ok {
		return info, raw, true
	}
	return unitInfo{}, "", false
}

// preserveUnitCase returns the original-case token the user wrote when it differs
// from the display form, falling back to the canonical display otherwise.
func preserveUnitCase(userToken, display string) string {
	if userToken == "" {
		return display
	}
	return userToken
}
