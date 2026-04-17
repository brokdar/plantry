// Package jsonld extracts schema.org Recipe nodes from HTML documents.
//
// The parser is tolerant of the shape variations seen in the wild:
//   - a single top-level JSON object with @type == "Recipe"
//   - a top-level array of objects
//   - @graph-wrapped arrays
//   - @type as a string or as an array containing "Recipe" (chefkoch)
//   - recipeInstructions as string, []string, []HowToStep, or []HowToSection
//   - image as string, []string, or ImageObject with a url field
package jsonld

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// ErrNoRecipe is re-exported for callers that want a local reference. The
// canonical value lives in domain/importer.
var ErrNoRecipe = importer.ErrNoRecipe

// Extractor implements the importer.JSONLDExtractor port.
type Extractor struct{}

// ExtractRecipe finds the first schema.org Recipe embedded in the given HTML body.
// Returns importer.ErrNoRecipe when no Recipe can be located.
func (Extractor) ExtractRecipe(htmlBody string) (*importer.RawRecipe, error) {
	return ExtractRecipe(htmlBody)
}

// scriptRE captures the body of every <script type="application/ld+json"> tag.
// The match is non-greedy so multiple script tags on the same page each yield a capture.
var scriptRE = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// ExtractRecipe is the package-level variant that does not require an Extractor value.
func ExtractRecipe(htmlBody string) (*importer.RawRecipe, error) {
	matches := scriptRE.FindAllStringSubmatch(htmlBody, -1)
	if len(matches) == 0 {
		return nil, importer.ErrNoRecipe
	}

	for _, m := range matches {
		body := strings.TrimSpace(m[1])
		if body == "" {
			continue
		}
		// Many sites wrap the JSON in HTML comments to hide it from non-JS parsers.
		body = strings.TrimPrefix(body, "<!--")
		body = strings.TrimSuffix(body, "-->")
		body = strings.TrimSpace(body)

		var value any
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			continue
		}

		if node := findRecipeNode(value); node != nil {
			return decodeRecipe(node), nil
		}
	}
	return nil, importer.ErrNoRecipe
}

// findRecipeNode recursively searches v for the first map with @type matching "Recipe".
// @type may be a string or an array; both are recognized.
func findRecipeNode(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		if isRecipe(t["@type"]) {
			return t
		}
		// Search @graph explicitly; it's a common top-level shape.
		if g, ok := t["@graph"]; ok {
			if node := findRecipeNode(g); node != nil {
				return node
			}
		}
		// Fall back to scanning every nested value.
		for _, child := range t {
			if node := findRecipeNode(child); node != nil {
				return node
			}
		}
	case []any:
		for _, child := range t {
			if node := findRecipeNode(child); node != nil {
				return node
			}
		}
	}
	return nil
}

func isRecipe(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, "Recipe")
	case []any:
		for _, inner := range t {
			if s, ok := inner.(string); ok && strings.EqualFold(s, "Recipe") {
				return true
			}
		}
	}
	return false
}

// decodeRecipe extracts known fields from the Recipe node, normalizing shape variations.
func decodeRecipe(node map[string]any) *importer.RawRecipe {
	r := &importer.RawRecipe{
		Name:             stringOf(node["name"]),
		Description:      stringOf(node["description"]),
		URL:              stringOf(node["url"]),
		Image:            normalizeImage(node["image"]),
		RecipeYield:      stringOf(node["recipeYield"]),
		RecipeIngredient: stringSlice(node["recipeIngredient"]),
		Keywords:         collectTags(node),
		AuthorName:       extractAuthorName(node["author"]),
	}

	r.RecipeYieldNumber = ParseYield(r.RecipeYield)
	r.TotalMinutes, _ = ParseISODuration(stringOf(node["totalTime"]))
	r.PrepMinutes, _ = ParseISODuration(stringOf(node["prepTime"]))
	r.CookMinutes, _ = ParseISODuration(stringOf(node["cookTime"]))
	// If a site publishes only totalTime, use it as the cook time — matches the
	// old Plantry behavior and keeps chefkoch recipes sensibly populated.
	if r.PrepMinutes == 0 && r.CookMinutes == 0 && r.TotalMinutes > 0 {
		r.CookMinutes = r.TotalMinutes
	}
	r.RecipeInstructions = normalizeInstructions(node["recipeInstructions"])

	return r
}

// collectTags merges recipeCategory, recipeCuisine, and keywords into a single
// deduplicated tag list — chefkoch splits its metadata across all three.
func collectTags(node map[string]any) []string {
	var out []string
	seen := map[string]bool{}
	for _, key := range []string{"recipeCategory", "recipeCuisine", "keywords"} {
		for _, tag := range normalizeKeywords(node[key]) {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{strings.TrimSpace(t)}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	}
	return nil
}

func normalizeImage(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, normalizeImage(item)...)
		}
		return out
	case map[string]any:
		if url := stringOf(t["url"]); url != "" {
			return []string{url}
		}
	}
	return nil
}

func normalizeKeywords(v any) []string {
	switch t := v.(type) {
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []any:
		return stringSlice(t)
	}
	return nil
}

func normalizeInstructions(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		// Split long paragraphs on blank lines, otherwise return as single step.
		parts := strings.Split(t, "\n\n")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []any:
		var out []string
		for _, item := range t {
			switch node := item.(type) {
			case string:
				s := strings.TrimSpace(node)
				if s != "" {
					out = append(out, s)
				}
			case map[string]any:
				typ, _ := node["@type"].(string)
				switch typ {
				case "HowToSection":
					// Recurse into itemListElement.
					out = append(out, normalizeInstructions(node["itemListElement"])...)
				case "HowToStep", "":
					if s := stringOf(node["text"]); s != "" {
						out = append(out, s)
						continue
					}
					if s := stringOf(node["name"]); s != "" {
						out = append(out, s)
					}
				default:
					if s := stringOf(node["text"]); s != "" {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
	return nil
}

func extractAuthorName(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		return stringOf(t["name"])
	case []any:
		for _, item := range t {
			if name := extractAuthorName(item); name != "" {
				return name
			}
		}
	}
	return ""
}

// isoDurationRE matches ISO 8601 durations as emitted by recipe sites. Supports
// both the compact schema.org form (PT30M, PT1H30M) and the verbose form that
// chefkoch.de uses (P0DT0H10M). Weeks/months/years are not relevant for recipes.
var isoDurationRE = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParseISODuration parses "PT30M", "PT1H", "PT1H30M", "PT90S", and the verbose
// "P0DT0H10M" form chefkoch.de emits, into whole minutes. Returns (0, false)
// when the input is empty or malformed.
func ParseISODuration(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	m := isoDurationRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	days, _ := strconv.Atoi(m[1])
	hours, _ := strconv.Atoi(m[2])
	minutes, _ := strconv.Atoi(m[3])
	seconds, _ := strconv.Atoi(m[4])
	total := days*24*60 + hours*60 + minutes + seconds/60
	if total == 0 && days == 0 && hours == 0 && minutes == 0 && seconds == 0 {
		return 0, false
	}
	return total, true
}

// yieldNumberRE grabs the first run of digits from a yield string like "4 Portionen".
var yieldNumberRE = regexp.MustCompile(`(\d+)`)

// ParseYield extracts the leading integer from a recipeYield string, falling back to 1.
func ParseYield(s string) int {
	m := yieldNumberRE.FindStringSubmatch(s)
	if m == nil {
		return 1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 1
	}
	return n
}
