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
	KindClassify       Kind = "classify"
	KindImportFallback Kind = "import_fallback"
	KindLookup         Kind = "lookup"
	// KindUntracked marks a metered call made without a started run.
	KindUntracked Kind = "untracked"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindGenerate, KindRegenerate, KindChat, KindDiscovery, KindClassify, KindImportFallback, KindLookup, KindUntracked:
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
	ErrorKey         string
	RepairAttempts   int
	ViolationsJSON   string
	ScoreJSON        string
	Notes            string
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
}

// Store persists runs, steps and system prompts. Implementations are safe for
// concurrent use. Keeping Step.Seq unique per run is the caller's
// responsibility.
type Store interface {
	CreateRun(ctx context.Context, start RunStart) (int64, error)
	FinishRun(ctx context.Context, end RunEnd) error
	AddStep(ctx context.Context, step Step) error
	UpsertPrompt(ctx context.Context, hash, text string) error
	UpsertToolSchema(ctx context.Context, hash, json string) error
	// PruneSteps deletes the steps of runs started before before and returns
	// how many were deleted. Runs, prompts and tool schemas stay.
	PruneSteps(ctx context.Context, before time.Time) (int64, error)
}
