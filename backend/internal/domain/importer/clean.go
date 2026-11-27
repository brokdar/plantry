package importer

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Draft warning codes the review translates.
const (
	// WarningRedirectedOtherRecipe: the page redirected to a different recipe
	// than the URL named (a wrong slug with a valid id).
	WarningRedirectedOtherRecipe = "redirected_other_recipe"
	// WarningTimesFromSteps: the page published no times; they are the sum of
	// the durations the steps mention.
	WarningTimesFromSteps = "times_from_steps"
	// WarningSuspiciousUnit marks a line whose unit is probably a typo.
	WarningSuspiciousUnit = "suspicious_unit"
)

var (
	htmlBreakRE = regexp.MustCompile(`(?i)<\s*(br|/p|/li|/div)\s*/?>`)
	htmlTagRE   = regexp.MustCompile(`<[^>]*>`)
)

// stripHTML turns an HTML fragment into plain text: block breaks become
// spaces, tags are dropped and entities decoded ("Für die <strong>Muffins</strong>").
func stripHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return s
	}
	s = htmlBreakRE.ReplaceAllString(s, " ")
	s = htmlTagRE.ReplaceAllString(s, "")
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(strings.ReplaceAll(html.UnescapeString(s), "\u00a0", " "), " "))
}

var (
	nameSuffixRE   = regexp.MustCompile(`(?i)\s+(rezepte?|recipes?)\s*$`)
	nameByAuthorRE = regexp.MustCompile(`^(.+?)\s+[-–|]\s+.*\s(?:von|by)\s+\S+$`)
	tagADayRE      = regexp.MustCompile(`(?i)^\d+ of \d+-a-day$`)
	tagRecipesRE   = regexp.MustCompile(`(?i)\s(rezepte|recipes)$`)
)

// cleanRecipeName drops what sites append to a title: " Rezept" and a
// " - tagline von author" credit.
func cleanRecipeName(name string) string {
	name = strings.TrimSpace(stripHTML(name))
	if m := nameByAuthorRE.FindStringSubmatch(name); m != nil {
		name = m[1]
	}
	if c := nameSuffixRE.ReplaceAllString(name, ""); c != "" {
		name = c
	}
	return name
}

// cleanTags drops page keywords that are no tags: the author, "1 of
// 5-a-day" badges and category links ("Chili con carne Rezepte").
func cleanTags(tags []string, author string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(stripHTML(t))
		switch {
		case t == "", tagADayRE.MatchString(t), tagRecipesRE.MatchString(t),
			author != "" && strings.EqualFold(t, author):
			continue
		}
		out = append(out, t)
	}
	return out
}

// prepWords start the preparation text of an ingredient line ("onion
// roughly chopped"); everything from the first one on is a note, not the
// food's name.
var prepWords = setOf(
	"roughly", "finely", "coarsely", "thinly", "freshly", "chopped", "sliced", "diced",
	"peeled", "grated", "minced", "crushed", "deseeded", "seeded", "drained", "rinsed",
	"halved", "quartered", "shredded", "torn", "trimmed", "cubed", "melted", "softened",
	"beaten", "toasted", "to", "for", "plus", "cut", "into", "separated", "fein", "grob", "gehackt", "gewürfelt",
	"geschält", "gerieben", "gehobelt", "geschnitten", "gewaschen", "zerdrückt",
	"geraspelt", "gehäutet", "entkernt", "abgetropft", "zum",
)

// sizeWords are size adjectives dropped from the front of a name.
var sizeWords = setOf(
	"small", "medium", "large", "big", "medium-sized", "large-sized", "small-sized",
	"kleine", "kleiner", "kleines", "große", "großer", "großes", "grosse", "mittelgroße",
	"mittelgroßer", "mittelgroßes", "mittlere", "mittlerer", "mittleres", "dicke",
)

func setOf(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

// CleanName returns an ingredient name without leading size words, a leading
// "of", and trailing preparation text; the preparation text comes back as
// prep so the caller can keep it as a note.
func CleanName(name string) (clean, prep string) {
	words := strings.Fields(name)
	for len(words) > 1 {
		low := strings.ToLower(words[0])
		_, size := sizeWords[low]
		if !size && low != "of" && low != "von" {
			break
		}
		words = words[1:]
	}
	for i := 1; i < len(words); i++ {
		if prepStartsAt(words, i) {
			return strings.Join(words[:i], " "), strings.Join(words[i:], " ")
		}
	}
	return strings.Join(words, " "), ""
}

// connectorWords introduce free note text ("oil for frying").
var connectorWords = setOf("to", "for", "plus", "zum", "into")

// prepStartsAt reports whether words[i:] is preparation text rather than
// part of the name: the word is a prep word that follows a comma or
// is a connector, or everything from it on is prep words (up to a connector,
// after which any text is note). "Frozen chopped spinach" stays whole.
func prepStartsAt(words []string, i int) bool {
	low := strings.ToLower(strings.Trim(words[i], ",;"))
	if _, ok := prepWords[low]; !ok {
		return false
	}
	if _, ok := connectorWords[low]; ok || strings.HasSuffix(words[i-1], ",") {
		return true
	}
	for _, w := range words[i:] {
		w = strings.ToLower(strings.Trim(w, ",;"))
		if _, ok := connectorWords[w]; ok {
			return true
		}
		if _, ok := prepWords[w]; !ok && w != "and" && w != "und" {
			return false
		}
	}
	return true
}

// redirectedElsewhere reports whether fetching requested ended on a page
// whose slug shares too little with the requested one, as when a site
// redirects by recipe id to a different recipe.
func redirectedElsewhere(requested, final string) bool {
	a, errA := url.Parse(requested)
	b, errB := url.Parse(final)
	if errA != nil || errB != nil || strings.TrimRight(a.Path, "/") == strings.TrimRight(b.Path, "/") {
		return false
	}
	sa, sb := slugWords(a.Path), slugWords(b.Path)
	if len(sa) == 0 || len(sb) == 0 {
		return false
	}
	shared := 0
	for w := range sa {
		if _, ok := sb[w]; ok {
			shared++
		}
	}
	return float64(shared)/float64(min(len(sa), len(sb))) < 0.5
}

func slugWords(path string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool {
		return (r < 'a' || r > 'z') && !strings.ContainsRune("äöüß", r)
	}) {
		if len(w) > 2 && w != "rezept" && w != "recipe" && w != "recipes" && w != "rezepte" {
			out[w] = struct{}{}
		}
	}
	return out
}

var stepDurationRE = regexp.MustCompile(`(?i)(\d+)(?:\s*[-–]\s*(\d+))?\s*(min(?:ute[n]?|s)?\.?|std\.?|stunden?|h|hours?|hrs?)\b`)

var altDurationRE = regexp.MustCompile(`(?i)(?:\b(?:or|oder|alternativ|bis zu)|/)\s*(?:ca\.?\s*)?$`)

// maxStepMinutes is the longest total a recipe is believed to take; more is
// a typo or a stray number.
const maxStepMinutes = 24 * 60

// minutesFromSteps sums the durations the steps mention ("20 Min", "6-7
// Stunden": the upper bound). Per step only the longest counts, so a short
// duration inside a long one ("30 Min backen, nach 15 Min wenden") is not
// added, and alternatives ("oder 90 Min") are skipped. It returns 0 when no
// step names one or the total exceeds 24 hours.
func minutesFromSteps(steps []string) int {
	total := 0
	for _, step := range steps {
		longest := 0
		for _, loc := range stepDurationRE.FindAllStringSubmatchIndex(step, -1) {
			if altDurationRE.MatchString(step[:loc[0]]) {
				continue
			}
			n, _ := strconv.Atoi(step[loc[2]:loc[3]])
			if loc[4] >= 0 {
				n, _ = strconv.Atoi(step[loc[4]:loc[5]])
			}
			if !strings.HasPrefix(strings.ToLower(step[loc[6]:loc[7]]), "min") {
				n *= 60
			}
			longest = max(longest, n)
		}
		total += longest
	}
	if total > maxStepMinutes {
		return 0
	}
	return total
}
