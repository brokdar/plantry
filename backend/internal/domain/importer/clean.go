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
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(html.UnescapeString(s), " "))
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
	"beaten", "toasted", "to", "for", "plus", "fein", "grob", "gehackt", "gewürfelt",
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
		if _, ok := prepWords[strings.ToLower(strings.Trim(words[i], ",;"))]; ok {
			return strings.Join(words[:i], " "), strings.Join(words[i:], " ")
		}
	}
	return strings.Join(words, " "), ""
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

// minutesFromSteps sums the durations the steps mention ("20 Min", "6-7
// Stunden": the upper bound). It returns 0 when no step names one.
func minutesFromSteps(steps []string) int {
	total := 0
	for _, step := range steps {
		for _, m := range stepDurationRE.FindAllStringSubmatch(step, -1) {
			n, _ := strconv.Atoi(m[1])
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			}
			if strings.HasPrefix(strings.ToLower(m[3]), "min") {
				total += n
			} else {
				total += n * 60
			}
		}
	}
	return total
}
