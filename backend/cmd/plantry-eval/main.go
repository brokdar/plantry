// Command plantry-eval runs the offline eval suite of plan generation
// (spec §8.4): every scenario N times on a fresh in-process database through
// the real generation service, a JSONL report, a summary and, with
// -compare, a per-component diff against an earlier report.
//
// The LLM chooser plans with the fake client, scripted per choice, unless
// PLANTRY_EVAL_LIVE=1 is set; then it calls the provider named by
// PLANTRY_AI_PROVIDER (anthropic or openai) with PLANTRY_AI_API_KEY and the
// model of -model (default PLANTRY_AI_MODEL).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/eval"
)

func main() {
	// The migrations log through slog; keep stdout for the report.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Getenv, gitSHA, time.Now); err != nil {
		fmt.Fprintln(os.Stderr, "plantry-eval:", err)
		os.Exit(1)
	}
}

// errFailed reports runs that failed an expectation; the report is still
// written.
var errFailed = errors.New("runs failed expectations")

func run(ctx context.Context, args []string, stdout io.Writer, getenv func(string) string, sha func() string, now func() time.Time) error {
	fs := flag.NewFlagSet("plantry-eval", flag.ContinueOnError)
	scenarios := fs.String("scenarios", "", "comma-separated scenario names (default: all)")
	chooser := fs.String("chooser", "heuristic", "heuristic or llm")
	runs := fs.Int("runs", eval.DefaultRuns, "runs per scenario")
	model := fs.String("model", "", "LLM model (live: default PLANTRY_AI_MODEL; scripted: a label, default fake)")
	version := fs.String("prompt-version", "", "prompt version label of the report (default: the chooser's)")
	out := fs.String("out", "plantry-eval.jsonl", "JSONL report path")
	compare := fs.String("compare", "", "an earlier JSONL report to diff against")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runs < 1 {
		return fmt.Errorf("-runs must be at least 1")
	}
	var names []string
	if *scenarios != "" {
		names = strings.Split(*scenarios, ",")
	}
	scs, err := eval.LoadScenarios(names...)
	if err != nil {
		return err
	}
	var prev []eval.Record
	if *compare != "" {
		if prev, err = readReport(*compare); err != nil {
			return err
		}
	}
	cfg := eval.Config{
		Chooser: airun.Chooser(*chooser), Runs: *runs, Model: *model, PromptVersion: *version,
		GitSHA: sha(), Now: now,
	}
	if cfg.Chooser == airun.ChooserLLM && getenv("PLANTRY_EVAL_LIVE") == "1" {
		if cfg.Live, cfg.Model, err = live(getenv, *model); err != nil {
			return err
		}
	}
	recs, err := eval.Run(ctx, cfg, scs)
	if err != nil {
		return err
	}
	if err := writeReport(*out, recs); err != nil {
		return err
	}
	if err := eval.WriteSummary(stdout, recs); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Wrote %d records to %s\n", len(recs), *out); err != nil {
		return err
	}
	if prev != nil {
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
		if err := eval.WriteCompare(stdout, prev, recs); err != nil {
			return err
		}
	}
	failed := 0
	for _, r := range recs {
		if len(r.Failed()) > 0 {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d %w", failed, len(recs), errFailed)
	}
	return nil
}

// live builds the real provider's client from the environment.
func live(getenv func(string) string, model string) (*eval.Live, string, error) {
	provider, key := getenv("PLANTRY_AI_PROVIDER"), getenv("PLANTRY_AI_API_KEY")
	if model == "" {
		model = getenv("PLANTRY_AI_MODEL")
	}
	if key == "" || model == "" {
		return nil, "", errors.New("live mode needs PLANTRY_AI_API_KEY and -model or PLANTRY_AI_MODEL")
	}
	switch provider {
	case "anthropic":
		return &eval.Live{Provider: provider, Client: anthropic.New(key)}, model, nil
	case "openai":
		return &eval.Live{Provider: provider, Client: openai.New(key)}, model, nil
	}
	return nil, "", fmt.Errorf("live mode needs PLANTRY_AI_PROVIDER anthropic or openai, not %q", provider)
}

func readReport(path string) ([]eval.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	recs, err := eval.ReadJSONL(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return recs, nil
}

func writeReport(path string, recs []eval.Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := eval.WriteJSONL(f, recs); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// gitSHA is the build's VCS revision, else the checkout's HEAD, else
// "unknown".
func gitSHA() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return s.Value[:12]
			}
		}
	}
	if out, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}
