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

// RunStart holds the fields known when a run begins.
type RunStart struct {
	Kind          Kind
	StartedAt     time.Time
	PromptVersion string
	WindowStart   *time.Time
	WindowEnd     *time.Time
	Instruction   *string
}

// RunEnd holds the totals written when a run finishes. An empty PromptHash
// or ErrorKey is stored as NULL.
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
}

// Step is one LLM call or tool call within a run. An empty ArgsJSON or
// ResultJSON is stored as NULL.
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
}

// Store persists runs, steps and system prompts.
type Store interface {
	CreateRun(ctx context.Context, start RunStart) (int64, error)
	FinishRun(ctx context.Context, end RunEnd) error
	AddStep(ctx context.Context, step Step) error
	UpsertPrompt(ctx context.Context, hash, text string) error
}
