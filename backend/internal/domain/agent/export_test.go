package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// WithToday lets the external tests pin the chat's today, which the past-date
// guard (D7) reads.
var WithToday = withToday

// WithWindow lets the external tests set the chat's plan window, which
// get_plan and regenerate read.
var WithWindow = withWindow

// IssueDelta and WithIssueBaseline expose the `after` block's issue diff.
var (
	IssueDelta        = issueDelta
	WithIssueBaseline = withIssueBaseline
)

// ChatIssues and ChatFacts expose the chat prompt's issue list and fact
// sections to the external tests.
var (
	ChatIssues = chatIssues
	ChatFacts  = chatFacts
)

// DeltaOf and MealResult expose the issue diff and the meal ops' also_changed
// part to the external tests.
var (
	DeltaOf    = deltaOf
	MealResult = mealResult
)

// BatchDates is the dates a batch has collected.
func BatchDates(b *batch) []string { return b.aff.Dates }

// NewBatch is an empty batch.
func NewBatch() *batch { return &batch{} }

var IssueKey = issueKey

// PromptCharBudget is the prompt budget, for the size tests.
const PromptCharBudget = promptCharBudget

// PlainRules is the rewrite of a tool result for the model (no rule codes).
var PlainRules = plainRules

// WithTurn puts a chat turn of the conversation in ctx, as Service.Chat does.
func WithTurn(ctx context.Context, conversationID int64) context.Context {
	return withTurn(ctx, chatTurn{conversationID: conversationID})
}

// WithTurnSeq is WithTurn for the seq-th turn of the conversation: offers made
// in one turn are not the next turn's.
func WithTurnSeq(ctx context.Context, conversationID int64, seq uint64) context.Context {
	return withTurn(ctx, chatTurn{conversationID: conversationID, seq: seq})
}

// Worsened, ModelView and AddDerived expose the report's "worse" rule and the
// model-facing views of a tool result.
var (
	Worsened   = worsened
	ModelView  = modelView
	AddDerived = addDerived
)

// NewWarnings exposes the guard's set of warnings a write makes.
var NewWarnings = newWarnings

// PlainReply exposes the final-reply rewriter.
var PlainReply = plainReply

// DropAfterWeek exposes the trimming of the after block.
var DropAfterWeek = dropAfterWeek

// RefusalTell is the tell of a refused mark, nil for any other error.
func RefusalTell(err error) []string {
	var r markRefusal
	if errors.As(err, &r) {
		return r.tell
	}
	return nil
}

// MarkChange exposes the cause sentence of a day target that moved.
func MarkChange(before, after []string) string { return markChange(before, after, langEN) }

// ComposeReply is the reply a household reads for a turn whose last write
// returned output (nil: no write), with the model's text.
func ComposeReply(model, user string, write bool, output json.RawMessage) string {
	t := replyTurn{user: user, write: write, numbers: map[string]bool{}}
	if output != nil {
		t.block, _ = blockOf(output)
		numbersIn(string(output), t.numbers)
	}
	return composeReply(model, t)
}
