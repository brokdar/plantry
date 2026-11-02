package discovery

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// PromptVersion is the run log's prompt_version of discovery runs. Bump it
// on every change to the research prompt, the user message or the repair
// message.
const PromptVersion = "d1"

// Prompt bounds (D8).
const (
	promptBudget     = 12000 // characters
	maxCandidates    = 6
	rejectedInPrompt = 20
	recipesInPrompt  = 300
	leavesInPrompt   = 300
)

// PromptInput is the research context (D8).
type PromptInput struct {
	Request  Request
	Profile  *profile.Profile
	Rejected []Suggestion
	// RecipeNames are the user's recipes, newest first; LeafNames the
	// ingredients used in them, most used first (known mode only).
	RecipeNames []string
	LeafNames   []string
}

// RenderPrompt is the research system prompt. It stays within 12 000
// characters: the recipe names, then the ingredient names, then the
// rejected suggestions are cut from their end until it fits.
func RenderPrompt(in PromptInput) string {
	for {
		s := renderPrompt(in)
		over := utf8.RuneCountInString(s) - promptBudget
		if over <= 0 {
			return s
		}
		switch {
		case len(in.RecipeNames) > 0:
			in.RecipeNames = cutLines(in.RecipeNames, over)
		case len(in.LeafNames) > 0:
			in.LeafNames = cutLines(in.LeafNames, over)
		case len(in.Rejected) > 0:
			in.Rejected = in.Rejected[:len(in.Rejected)-1]
		default:
			// Only the user's own notes can still be this long.
			return string([]rune(s)[:promptBudget])
		}
	}
}

// cutLines drops names from the end of a "- name" list until about n
// characters are gone.
func cutLines(names []string, n int) []string {
	for len(names) > 0 && n > 0 {
		n -= utf8.RuneCountInString(names[len(names)-1]) + 3
		names = names[:len(names)-1]
	}
	return names
}

func renderPrompt(in PromptInput) string {
	p := in.Profile
	if p == nil {
		p = &profile.Profile{}
	}
	var b strings.Builder
	b.WriteString("You are Plantry's recipe scout. Find real recipe pages that suit this household and are new to it.\n\n")

	b.WriteString("Household\n")
	fmt.Fprintf(&b, "- Diet: %s\n", orNone(string(p.DietType), "not set"))
	fmt.Fprintf(&b, "- Allergies and restrictions: %s\n", orNone(strings.Join(p.DietaryRestrictions, ", "), "none"))
	if p.SystemPrompt != nil && strings.TrimSpace(*p.SystemPrompt) != "" {
		fmt.Fprintf(&b, "- Notes from the user: %s\n", strings.TrimSpace(*p.SystemPrompt))
	}
	fmt.Fprintf(&b, "- Disliked tags: %s\n", orNone(strings.Join(dislikes(p), ", "), "none"))
	lang := language(p.Locale)
	fmt.Fprintf(&b, "- Language: %s. Prefer recipe pages in %s.\n\n", lang, lang)

	b.WriteString("Task\n")
	b.WriteString(modeLine(in.Request))
	b.WriteString("\n")

	if len(in.Rejected) > 0 {
		b.WriteString("\nRecently rejected suggestions (avoid similar ones):\n")
		for _, s := range in.Rejected {
			fmt.Fprintf(&b, "- %s (%s)\n", s.Draft.Name, reasonText(s.Reason))
		}
	}
	if in.Request.Mode == ModeKnown && len(in.LeafNames) > 0 {
		b.WriteString("\nIngredients the user cooks with (most used first):\n")
		writeList(&b, in.LeafNames)
	}
	if len(in.RecipeNames) > 0 {
		b.WriteString("\nRecipes the user already has (suggest none of these or near-duplicates):\n")
		writeList(&b, in.RecipeNames)
	}

	b.WriteString("\nRules\n")
	b.WriteString("- Prefer pages that carry schema.org Recipe JSON-LD.\n")
	b.WriteString("- Give each page's exact URL. Suggest only pages that exist.\n")
	fmt.Fprintf(&b, "- Return at most %d candidates.\n", maxCandidates)
	fmt.Fprintf(&b, "- End your reply with this JSON object and nothing after it: "+
		`{"candidates": [{"url": "...", "title": "...", "cuisine": "...", "why": "..."}]}`+
		". \"why\" is one short sentence in %s on why the recipe suits the user.\n", lang)
	return b.String()
}

func modeLine(r Request) string {
	switch r.Mode {
	case ModeExplore:
		return fmt.Sprintf("Find dinner recipes from the %s cuisine for the user to explore.", r.Cuisine)
	case ModeKnown:
		line := "Find dinner recipes made mostly from ingredients the user already cooks with."
		if r.Focus != "" {
			line += fmt.Sprintf(" Feature this ingredient: %s.", r.Focus)
		}
		return line
	default:
		if r.Cuisine == "" {
			return "Find dinner recipes from a cuisine the user has not cooked yet."
		}
		return fmt.Sprintf("Today's cuisine is %s. Find dinner recipes from it.", r.Cuisine)
	}
}

// userMessage opens the research conversation; it names the mode's cuisine
// or focus.
func userMessage(r Request) string {
	switch r.Mode {
	case ModeExplore:
		return fmt.Sprintf("Find recipes to explore: %s cuisine.", r.Cuisine)
	case ModeKnown:
		if r.Focus == "" {
			return "Find recipes with ingredients I know."
		}
		return fmt.Sprintf("Find recipes with ingredients I know, featuring %s.", r.Focus)
	default:
		if r.Cuisine == "" {
			return "Find today's recipe suggestions."
		}
		return fmt.Sprintf("Find today's recipe suggestions (cuisine: %s).", r.Cuisine)
	}
}

// repairMessage asks for the JSON once more after an unreadable reply (D5).
const repairMessage = `Your reply was not valid JSON in the required shape. Reply with only the JSON object {"candidates": [{"url": "...", "title": "...", "cuisine": "...", "why": "..."}]}, nothing else.`

// DailyCuisine is the cuisine of the day: the list rotated by the date's day
// number (D8). An empty list gives "".
func DailyCuisine(cuisines []string, day time.Time) string {
	if len(cuisines) == 0 {
		return ""
	}
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	n := d.Unix() / 86400
	return cuisines[int(n%int64(len(cuisines)))]
}

func writeList(b *strings.Builder, names []string) {
	for _, n := range names {
		fmt.Fprintf(b, "- %s\n", n)
	}
}

func dislikes(p *profile.Profile) []string {
	var out []string
	switch v := p.Preferences[profile.PreferenceKeyDislikes].(type) {
	case []string:
		out = v
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func language(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "de") {
		return "German"
	}
	return "English"
}

func reasonText(r *Reason) string {
	if r == nil {
		return "no reason given"
	}
	switch *r {
	case ReasonTooLong:
		return "takes too long"
	case ReasonIngredients:
		return "ingredients"
	case ReasonTaste:
		return "taste"
	}
	return string(*r)
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}
