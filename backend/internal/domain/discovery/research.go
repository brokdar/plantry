package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Research call shape (D1, D2, D7).
const (
	searchMaxUses    = 5
	maxContinuations = 3
	researchTokens   = 8000
)

// findings is what the research agent returned.
type findings struct {
	Candidates []Candidate
	// Searches counts the web searches the provider ran (D4: grounded > 0).
	Searches int
	Repaired bool
}

// research asks the model for candidates: one call, continued after each
// pause_turn (at most three times), and one repair call when the reply holds
// no readable candidates JSON (D5). An unreadable repair fails with
// domain.ErrDiscoveryInvalidOutput, a provider error with
// domain.ErrAIProviderFailed.
func research(ctx context.Context, logger *slog.Logger, client llm.Client, model string, search bool, system string, req Request) (findings, error) {
	r := llm.Request{
		Model:     model,
		System:    system,
		Messages:  []llm.Message{userText(userMessage(req))},
		MaxTokens: researchTokens,
	}
	if search {
		r.WebSearch = &llm.WebSearch{MaxUses: searchMaxUses}
	}
	var f findings
	resp, err := converse(ctx, client, &r, &f.Searches)
	if err != nil {
		return f, err
	}
	reply := textOf(resp.Message)
	cands, perr := parseCandidates(reply)
	if perr != nil {
		logger.WarnContext(ctx, "discovery.research repair", "error", perr,
			"stop_reason", resp.StopReason, "reply", truncateRunes(reply, maxLoggedReply))
		f.Repaired = true
		r.Messages = append(r.Messages, userText(repairMessage))
		if resp, err = converse(ctx, client, &r, &f.Searches); err != nil {
			return f, err
		}
		if cands, perr = parseCandidates(textOf(resp.Message)); perr != nil {
			return f, fmt.Errorf("%w: %w", domain.ErrDiscoveryInvalidOutput, perr)
		}
	}
	f.Candidates = cands
	return f, nil
}

// converse sends r and resends it with the assistant's blocks while the
// provider pauses the turn (D2). Every reply joins r.Messages.
func converse(ctx context.Context, client llm.Client, r *llm.Request, searches *int) (*llm.Response, error) {
	for i := 0; ; i++ {
		resp, err := stream(ctx, client, *r)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %w", domain.ErrAIProviderFailed, err)
		}
		*searches += resp.Usage.WebSearchRequests
		if len(resp.Message.Content) > 0 {
			r.Messages = append(r.Messages, resp.Message)
		}
		if resp.StopReason != llm.StopReasonPauseTurn || i == maxContinuations {
			return resp, nil
		}
	}
}

func stream(ctx context.Context, client llm.Client, r llm.Request) (*llm.Response, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()
	resp, err := client.Stream(ctx, r, events)
	<-done
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty response")
	}
	return resp, nil
}

// parseCandidates reads the {"candidates": [...]} object the reply ends
// with, around any prose or code fence. Candidates without a URL are
// dropped; at most six are kept.
func parseCandidates(text string) ([]Candidate, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, errors.New("no JSON object in the reply")
	}
	var out struct {
		Candidates *[]Candidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &out); err != nil {
		return nil, err
	}
	if out.Candidates == nil {
		return nil, errors.New(`no "candidates" in the reply`)
	}
	var cands []Candidate
	for _, c := range *out.Candidates {
		c.URL = strings.TrimSpace(c.URL)
		if c.URL == "" {
			continue
		}
		cands = append(cands, c)
		if len(cands) == maxCandidates {
			break
		}
	}
	return cands, nil
}

// maxLoggedReply bounds the reply text a repair log line carries.
const maxLoggedReply = 500

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func textOf(m llm.Message) string {
	var b strings.Builder
	for _, c := range m.Content {
		if c.Type == llm.ContentTypeText {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func userText(s string) llm.Message {
	return llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: s}}}
}
