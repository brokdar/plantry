package food

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// translateQueryPrompt asks the model to rewrite a (possibly non-English)
// ingredient name as an English FDC search term.
const translateQueryPrompt = `You are a food ingredient translation assistant. For each ingredient name, provide the best English search term to find it in a food nutrition database (like USDA FoodData Central).

Rules:
- Use common English food names, not brand names
- Add "raw" for fresh produce when appropriate (e.g. "Zwiebel" → "onion raw")
- Add "dry" for pasta/grains when appropriate (e.g. "Spaghetti" → "spaghetti dry")
- Keep already-English names as-is but clarify if needed
- Return ONLY a JSON array of strings with the same number of elements as the input
- No markdown, no explanation, just the JSON array`

// pickBestPrompt asks the model to pick the best candidate for each query.
const pickBestPrompt = `You are a food matching assistant. For each ingredient, I provide the original name and a list of candidates from nutrition databases. Pick the BEST matching candidate by returning its 0-based index. Return -1 if no candidate is a reasonable match.

Rules:
- Prefer raw/unprocessed forms over cooked/processed for fresh ingredients
- Prefer generic forms over branded products
- Return ONLY a JSON array of integers with the same number of elements as the number of ingredients
- No markdown, no explanation, just the JSON array`

func translateQuery(ctx context.Context, client llm.Client, model, query string, trace *LookupTrace) string {
	if client == nil || strings.TrimSpace(query) == "" {
		return query
	}

	start := time.Now()
	userMsg := mustJSONString([]string{query})
	raw, err := runOneShot(ctx, client, llm.Request{
		Model:       model,
		System:      translateQueryPrompt,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: userMsg}}}},
		Temperature: 0,
		MaxTokens:   200,
	})
	dur := time.Since(start).Milliseconds()

	if err != nil {
		trace.Add(TraceEntry{
			Step: "ai.translate", Level: TraceLevelWarning,
			Summary: "AI translation failed; using original query", DurationMs: dur,
			Detail: AITranslationDetail{InputQuery: query, Error: err.Error()},
		})
		return query
	}

	cleaned := strings.TrimSpace(stripFences(raw))
	var terms []string
	if jerr := json.Unmarshal([]byte(cleaned), &terms); jerr != nil || len(terms) == 0 {
		trace.Add(TraceEntry{
			Step: "ai.translate", Level: TraceLevelWarning,
			Summary: "AI returned malformed JSON; using original query", DurationMs: dur,
			Detail: AITranslationDetail{InputQuery: query, RawResponse: raw},
		})
		return query
	}

	translated := strings.TrimSpace(terms[0])
	if translated == "" {
		trace.Add(TraceEntry{
			Step: "ai.translate", Level: TraceLevelWarning,
			Summary: "AI returned empty term; using original query", DurationMs: dur,
			Detail: AITranslationDetail{InputQuery: query, RawResponse: raw},
		})
		return query
	}

	trace.Add(TraceEntry{
		Step: "ai.translate", Level: TraceLevelSuccess,
		Summary: fmt.Sprintf("%q → %q", query, translated), DurationMs: dur,
		Detail: AITranslationDetail{InputQuery: query, Translated: translated, RawResponse: raw},
	})
	return translated
}

func pickBest(ctx context.Context, client llm.Client, model, originalQuery string, candidates []Candidate, trace *LookupTrace) int {
	if client == nil || len(candidates) <= 1 {
		return 0
	}

	descriptions := formatCandidateDescriptions(candidates)
	userMsg := fmt.Sprintf(
		`Ingredients:
%s

Candidates for ingredient 1:
%s`,
		mustJSONString([]string{originalQuery}),
		strings.Join(descriptions, "\n"),
	)

	start := time.Now()
	raw, err := runOneShot(ctx, client, llm.Request{
		Model:       model,
		System:      pickBestPrompt,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: userMsg}}}},
		Temperature: 0,
		MaxTokens:   50,
	})
	dur := time.Since(start).Milliseconds()

	if err != nil {
		trace.Add(TraceEntry{
			Step: "ai.pick_best", Level: TraceLevelWarning,
			Summary: "AI pick-best failed; using first result", DurationMs: dur,
			Detail: AIPickBestDetail{OriginalQuery: originalQuery, Candidates: descriptions, Error: err.Error()},
		})
		return 0
	}

	cleaned := strings.TrimSpace(stripFences(raw))
	var picks []int
	if jerr := json.Unmarshal([]byte(cleaned), &picks); jerr != nil || len(picks) == 0 {
		trace.Add(TraceEntry{
			Step: "ai.pick_best", Level: TraceLevelWarning,
			Summary: "AI returned malformed JSON; using first result", DurationMs: dur,
			Detail: AIPickBestDetail{OriginalQuery: originalQuery, Candidates: descriptions, RawResponse: raw},
		})
		return 0
	}

	picked := picks[0]
	if picked < 0 || picked >= len(candidates) {
		trace.Add(TraceEntry{
			Step: "ai.pick_best", Level: TraceLevelInfo,
			Summary:    fmt.Sprintf("AI returned index %d (out of range); using first result", picked),
			DurationMs: dur,
			Detail: AIPickBestDetail{
				OriginalQuery: originalQuery, Candidates: descriptions,
				RawResponse: raw, PickedIndex: picked,
			},
		})
		return 0
	}

	trace.Add(TraceEntry{
		Step: "ai.pick_best", Level: TraceLevelSuccess,
		Summary: fmt.Sprintf("AI picked index %d", picked), DurationMs: dur,
		Detail: AIPickBestDetail{
			OriginalQuery: originalQuery, Candidates: descriptions,
			RawResponse: raw, PickedIndex: picked,
		},
	})
	return picked
}

func formatCandidateDescriptions(candidates []Candidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		name := c.SourceName
		if name == "" {
			name = c.Name
		}
		kcal := ""
		if c.Kcal100g != nil {
			kcal = fmt.Sprintf(", %.0f kcal", *c.Kcal100g)
		}
		out[i] = fmt.Sprintf(`[%d] %q (%s%s)`, i, name, c.Source, kcal)
	}
	return out
}

func runOneShot(ctx context.Context, client llm.Client, req llm.Request) (string, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()

	resp, err := client.Stream(ctx, req, events)
	<-done
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("nil response")
	}

	var sb strings.Builder
	for _, block := range resp.Message.Content {
		if block.Type == llm.ContentTypeText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
