package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
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

// WithTurnNamed is WithTurn for a message that names the foods ids (SF3, C-4).
func WithTurnNamed(ctx context.Context, conversationID int64, ids ...int64) context.Context {
	named := map[int64]bool{}
	for _, id := range ids {
		named[id] = true
	}
	return withTurn(ctx, chatTurn{conversationID: conversationID, named: named})
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

// WithoutFacts is a tool result as the model reads it, without the block, the
// offer and the check snapshot of the report.
func WithoutFacts(raw json.RawMessage) json.RawMessage { return withoutFacts(raw) }

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

// ComposeDrops composes a final reply for user and model text and returns the
// sentences it left out with their reasons, as "reason: sentence".
func ComposeDrops(ctx context.Context, user, model string, write bool) []string {
	var drops []lintDrop
	t := replyTurn{user: user, write: write, numbers: map[string]bool{}, drops: &drops}
	if write {
		t.block = &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Fri 27 Nov dinner: A replaced by B.", DE: "Freitag, 27. Nov. Abendessen: A ersetzt durch B."}}}
	}
	composeReply(model, t)
	recordLintDrops(ctx, drops)
	out := make([]string, 0, len(drops))
	for _, d := range drops {
		out = append(out, d.Reason+": "+d.Sentence)
	}
	return out
}

// WithTurnText is WithTurn for a message with this text: the day kcal numbers
// it names (namedDayKcal) reach the tools as Service.Chat sets them.
func WithTurnText(ctx context.Context, conversationID int64, text string, w window.Window) context.Context {
	return withTurn(ctx, chatTurn{conversationID: conversationID, dayKcal: namedDayKcal(text, w, todayFrom(ctx)), away: awayStated.MatchString(diet.Fold(text))})
}

var NamedDayKcal = namedDayKcal

// UnderFloorLine is the EN line of underFloorBlock ("" when nil).
func UnderFloorLine(dayKcal map[string]float64, st planning.State) string {
	if b := underFloorBlock(dayKcal, st); b != nil {
		return b.facts[0].EN
	}
	return ""
}

// MessageLangCode is messageLang as "de" or "en", "" when nothing says;
// LanguageLine takes a code.
func MessageLangCode(texts, foods []string, locale string) string {
	l, ok := messageLang(texts, foods, locale)
	switch {
	case !ok:
		return ""
	case l == langDE:
		return "de"
	}
	return "en"
}

func LanguageLine(code string) string {
	if code == "de" {
		return languageLine(langDE)
	}
	return languageLine(langEN)
}

// WithWindowForTest carries a chat window starting on start to the tools.
func WithWindowForTest(ctx context.Context, start time.Time) context.Context {
	return withWindow(ctx, window.Window{Start: start, End: start.AddDate(0, 0, 6)})
}
