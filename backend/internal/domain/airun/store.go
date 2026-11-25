// Package airun records AI runs (one user-visible AI action) and their steps
// (LLM calls and tool calls) so token usage and failures can be inspected.
package airun

import (
	"context"
	"time"
)

// Kind identifies what triggered a run.
type Kind string

const (
	KindGenerate       Kind = "generate"
	KindRegenerate     Kind = "regenerate"
	KindChat           Kind = "chat"
	KindDiscovery      Kind = "discovery"
	KindImportFallback Kind = "import_fallback"
	KindLookup         Kind = "lookup"
	// KindUntracked marks a metered call made without a started run.
	KindUntracked Kind = "untracked"

	// Chef kinds (capped work, spec COST-1).
	KindChefAttributes    Kind = "chef_attributes"
	KindChefMatch         Kind = "chef_match"
	KindChefImportCheck   Kind = "chef_import_check"
	KindChefFind          Kind = "chef_find"
	KindChefHelp          Kind = "chef_help"
	KindChefLibraryReview Kind = "chef_library_review"
)

// Valid reports whether k is a known kind. ai_runs has no kind CHECK, so
// this is the only guard: the store rejects an invalid kind.
func (k Kind) Valid() bool {
	switch k {
	case KindGenerate, KindRegenerate, KindChat, KindDiscovery, KindImportFallback, KindLookup, KindUntracked,
		KindChefAttributes, KindChefMatch, KindChefImportCheck, KindChefFind, KindChefHelp, KindChefLibraryReview:
		return true
	}
	return false
}

// Status is the terminal (or in-progress) state of a run.
type Status string

const (
	StatusRunning            Status = "running"
	StatusOK                 Status = "ok"
	StatusError              Status = "error"
	StatusIterationsExceeded Status = "iterations_exceeded"
	StatusCancelled          Status = "cancelled"
	StatusPartial            Status = "partial"
)

// StepType distinguishes LLM calls from tool calls within a run.
type StepType string

const (
	StepLLMCall  StepType = "llm_call"
	StepToolCall StepType = "tool_call"
	// StepCodeExecution is code the model ran in the provider's code
	// execution container (programmatic tool calling): args_json is the code,
	// result_json its stdout, stderr and return code.
	StepCodeExecution StepType = "code_execution"
)

// Chooser is what picked a generation run's plates. Chat and lookup runs
// have none (empty).
type Chooser string

const (
	ChooserLLM       Chooser = "llm"
	ChooserHeuristic Chooser = "heuristic"
)

// RunStart holds the fields known when a run begins. A nil ConversationID
// and an empty RequestID, Chooser or ScopeJSON are stored as NULL.
type RunStart struct {
	Kind           Kind
	StartedAt      time.Time
	PromptVersion  string
	WindowStart    *time.Time
	WindowEnd      *time.Time
	Instruction    *string
	ConversationID *int64
	RequestID      string
	Chooser        Chooser
	ScopeJSON      string
	// AppVersion is the build that made the run; empty is NULL.
	AppVersion string
}

// RunEnd holds the totals written when a run finishes. An empty ErrorKey is
// stored as NULL, as is a PromptHash that is empty or not in the prompt store,
// and an empty ViolationsJSON, ScoreJSON or Notes.
type RunEnd struct {
	ID               int64
	Status           Status
	FinishedAt       time.Time
	Provider         string
	Model            string
	PromptHash       string
	Iterations       int
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	// WebSearchRequests sums the searches of the run's llm_calls.
	WebSearchRequests int64
	ReasoningTokens   int64
	// DurationMs is the run's wall time at ms precision.
	DurationMs     int64
	ErrorKey       string
	RepairAttempts int
	ViolationsJSON string
	ScoreJSON      string
	Notes          string
	ScopeJSON      string
	// ContainerID is the run's last code execution container; empty is NULL.
	ContainerID string
	// CostUSD is the run's cost at the prices of the time of the run; nil
	// (NULL) when the model has no price.
	CostUSD *float64
}

// Step is one LLM call or tool call within a run. Empty string fields are
// stored as NULL. ArgsJSON and ResultJSON (tool calls) are capped by the
// store; RequestJSON, ResponseJSON and StopReason (LLM calls) are stored as
// given.
type Step struct {
	RunID        int64
	Seq          int
	Type         StepType
	Name         string
	ArgsJSON     string
	ResultJSON   string
	IsError      bool
	DurationMs   int64
	InputTokens  int64
	OutputTokens int64
	RequestJSON  string
	ResponseJSON string
	StopReason   string
	// WebSearchRequests is the searches this llm_call was billed for.
	WebSearchRequests int64
	// StartedAt is when the step began (stored at ms precision); zero is NULL.
	StartedAt time.Time
	// TTFTMs is the time to the first streamed token or tool call of an
	// llm_call, nil when none arrived.
	TTFTMs           *int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	// ReasoningTokens is the provider-reported reasoning tokens (OpenAI);
	// ThinkingChars the length of the thinking text in the reply (Anthropic,
	// which reports no separate count).
	ReasoningTokens int64
	ThinkingChars   int64
	// ServedModel is the model the provider answered with.
	ServedModel string
	// ProviderRequestID is the provider's id of the call's HTTP request.
	ProviderRequestID string
	// Caller is "direct" or "code_execution" on a tool_call step and
	// CallerToolID the id of the code's server tool use block; empty is NULL.
	Caller       string
	CallerToolID string
}

// Store persists runs, steps and system prompts. Implementations are safe for
// concurrent use. Keeping Step.Seq unique per run is the caller's
// responsibility.
type Store interface {
	// CreateRun rejects a kind that is not Valid with domain.ErrInvalidInput.
	CreateRun(ctx context.Context, start RunStart) (int64, error)
	FinishRun(ctx context.Context, end RunEnd) error
	AddStep(ctx context.Context, step Step) error
	// UpdateStepResult completes a step written before its outcome was known.
	UpdateStepResult(ctx context.Context, runID int64, seq int, result string, isErr bool, durationMs int64) error
	UpsertPrompt(ctx context.Context, hash, text string) error
	UpsertToolSchema(ctx context.Context, hash, json string) error
	// PruneSteps deletes the steps of runs started before before and returns
	// how many were deleted. Runs, prompts and tool schemas stay.
	PruneSteps(ctx context.Context, before time.Time) (int64, error)
}
