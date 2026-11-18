package agent

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
