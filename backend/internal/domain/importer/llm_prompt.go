package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// htmlBodyLimit caps how many characters of stripped HTML we send to the LLM.
const htmlBodyLimit = 60000

// llmExtract runs the LLM fallback pipeline: strip HTML → send prompt → parse JSON.
// Returns a non-nil Recipe on success. Errors surface as domain.ErrAIProviderMissing
// (nil client), domain.ErrImportLLMFailed (invalid JSON after retry), or
// domain.ErrImportNoRecipe (LLM replied {"not_a_recipe": true}).
func (s *Service) llmExtract(ctx context.Context, htmlBody string) (*RawRecipe, error) {
	if s.llm == nil {
		return nil, domain.ErrAIProviderMissing
	}

	stripped := stripHTMLForLLM(htmlBody)
	stripped = truncateMiddle(stripped, htmlBodyLimit)

	userMsg := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{
		{Type: llm.ContentTypeText, Text: buildUserPrompt(stripped)},
	}}

	req := llm.Request{
		Model:       s.llmModel,
		System:      llmSystemPrompt,
		Messages:    []llm.Message{userMsg},
		Temperature: 0,
		MaxTokens:   4096,
	}

	text, err := runLLMOnce(ctx, s.llm, req)
	if err != nil {
		return nil, err
	}

	rec, parseErr := parseLLMResponse(text)
	if parseErr == nil {
		return rec, nil
	}
	// Definitive "no recipe" responses should not trigger a retry.
	if errors.Is(parseErr, domain.ErrImportNoRecipe) {
		return nil, domain.ErrImportNoRecipe
	}

	// One retry with correction.
	req.Messages = append(req.Messages,
		llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}},
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "Your previous response was not valid JSON. Output only the JSON object, nothing else."}}},
	)
	text2, err := runLLMOnce(ctx, s.llm, req)
	if err != nil {
		return nil, err
	}
	rec, parseErr = parseLLMResponse(text2)
	if parseErr != nil {
		if errors.Is(parseErr, domain.ErrImportNoRecipe) {
			return nil, domain.ErrImportNoRecipe
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrImportLLMFailed, parseErr)
	}
	return rec, nil
}

// runLLMOnce sends a single request through the streaming interface, drains the
// event channel, and returns the assembled assistant text.
func runLLMOnce(ctx context.Context, client llm.Client, req llm.Request) (string, error) {
	events := make(chan llm.Event, 16)
	// Drain events in a goroutine — we only care about the final Response.
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()

	resp, err := client.Stream(ctx, req, events)
	<-done
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrImportLLMFailed, err)
	}
	if resp == nil {
		return "", domain.ErrImportLLMFailed
	}

	var sb strings.Builder
	for _, block := range resp.Message.Content {
		if block.Type == llm.ContentTypeText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// parseLLMResponse converts the strict-JSON assistant output into a RawRecipe.
// A "{not_a_recipe: true}" payload is treated as ErrImportNoRecipe.
func parseLLMResponse(text string) (*RawRecipe, error) {
	trimmed := stripCodeFences(strings.TrimSpace(text))
	var payload struct {
		NotARecipe      bool     `json:"not_a_recipe"`
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		ImageURL        string   `json:"image_url"`
		PrepMinutes     *int     `json:"prep_minutes"`
		CookMinutes     *int     `json:"cook_minutes"`
		Servings        int      `json:"servings"`
		Instructions    []string `json:"instructions"`
		IngredientLines []string `json:"ingredient_lines"`
		Tags            []string `json:"tags"`
		Language        string   `json:"language"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("llm: invalid json: %w", err)
	}
	if payload.NotARecipe {
		return nil, domain.ErrImportNoRecipe
	}
	rec := &RawRecipe{
		Name:               payload.Name,
		Description:        payload.Description,
		RecipeIngredient:   payload.IngredientLines,
		RecipeInstructions: payload.Instructions,
		Keywords:           payload.Tags,
		RecipeYieldNumber:  payload.Servings,
	}
	if payload.ImageURL != "" {
		rec.Image = []string{payload.ImageURL}
	}
	if payload.PrepMinutes != nil {
		rec.PrepMinutes = *payload.PrepMinutes
	}
	if payload.CookMinutes != nil {
		rec.CookMinutes = *payload.CookMinutes
	}
	if rec.PrepMinutes > 0 && rec.CookMinutes > 0 {
		rec.TotalMinutes = rec.PrepMinutes + rec.CookMinutes
	}
	return rec, nil
}

const llmSystemPrompt = `You extract a structured recipe from messy HTML. You MUST output only a single JSON object matching the schema, no prose, no markdown, no backticks. If the HTML does not contain a recipe, output {"not_a_recipe": true}.`

func buildUserPrompt(htmlBody string) string {
	return `Extract the recipe from this HTML. Output language: German if the page is German, else English.

Schema:
{
  "name": string,
  "description": string,
  "image_url": string,
  "prep_minutes": number|null,
  "cook_minutes": number|null,
  "servings": number,
  "instructions": string[],
  "ingredient_lines": string[],
  "tags": string[],
  "language": "de"|"en"
}

HTML:
` + htmlBody
}

// scriptStyleRE removes <script>, <style>, and <svg> blocks from HTML.
var scriptStyleRE = regexp.MustCompile(`(?is)<(script|style|svg|noscript)[^>]*>.*?</(?:script|style|svg|noscript)>`)

// tagRE strips any remaining HTML tags.
var tagRE = regexp.MustCompile(`<[^>]+>`)

// whitespaceBlobRE collapses runs of whitespace to a single space.
var whitespaceBlobRE = regexp.MustCompile(`\s+`)

// stripHTMLForLLM returns the visible text of htmlBody with scripts/styles removed.
func stripHTMLForLLM(htmlBody string) string {
	body := scriptStyleRE.ReplaceAllString(htmlBody, " ")
	body = tagRE.ReplaceAllString(body, " ")
	body = whitespaceBlobRE.ReplaceAllString(body, " ")
	return strings.TrimSpace(body)
}

// truncateMiddle keeps the head and tail halves of s when it exceeds limit,
// preserving the ends of the page where recipe content usually sits.
func truncateMiddle(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	half := limit / 2
	return s[:half] + "\n…[truncated]…\n" + s[len(s)-half:]
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line.
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	// Drop trailing fence.
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
