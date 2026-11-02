package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

const (
	runTimeout = 3 * time.Minute
	// maxStored is how many suggestions one run stores (D12).
	maxStored = 3
	// staleAfter is the age of pending suggestions deleted by a run (D12).
	staleAfter = 30 * 24 * time.Hour
)

// Request starts a discovery run (D7). Cuisine is the daily cuisine (see
// DailyCuisine; empty = one the user has not cooked) or the explore
// cuisine; Focus is the known-mode ingredient. Day (YYYY-MM-DD, daily runs)
// is the day the run is for: its first suggestion is shown that day when
// none is (D12), and a failed run silences that day.
type Request struct {
	Mode    Mode
	Cuisine string
	Focus   string
	Day     string
}

// ProfileReader reads the household profile (profile.Service).
type ProfileReader interface {
	Get(ctx context.Context) (*profile.Profile, error)
}

// Catalogue names the user's recipes and their ingredients (FoodRepo).
type Catalogue interface {
	RecipeNames(ctx context.Context, limit int) ([]string, error)
	LeafNamesByRecipeUse(ctx context.Context, limit int) ([]string, error)
}

// CandidateVerifier is Verifier.
type CandidateVerifier interface {
	Verify(ctx context.Context, candidates []Candidate) ([]Verified, []Discarded, error)
}

// DraftAnnotator is Annotator.
type DraftAnnotator interface {
	Annotate(ctx context.Context, d *importer.Draft) (*Annotations, error)
}

// Preferences are the discovery settings.
type Preferences interface {
	DiscoveryDailyEnabled(ctx context.Context) (bool, error)
	DiscoveryCuisines(ctx context.Context) ([]string, error)
}

// Foods reads and updates saved foods (food.Service): accept copies the
// suggestion's image to the food.
type Foods interface {
	Get(ctx context.Context, id int64) (*food.Food, error)
	Update(ctx context.Context, f *food.Food) error
}

// Deps wires a Service.
type Deps struct {
	LLM       llm.Resolver
	Prefs     Preferences
	Foods     Foods
	Recorder  *airun.Recorder
	Profile   ProfileReader
	Catalogue Catalogue
	Verifier  CandidateVerifier
	Annotator DraftAnnotator
	Store     *Store
	Logger    *slog.Logger
	// Now and Timeout default to time.Now and three minutes.
	Now     func() time.Time
	Timeout time.Duration
}

// Service runs discovery: research → verify → annotate → store, recorded as
// one ai_runs row of kind discovery (D7). Runs are asynchronous and one at
// a time.
type Service struct {
	d      Deps
	runner *runner
}

// NewService returns a discovery service.
func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Timeout == 0 {
		d.Timeout = runTimeout
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{d: d, runner: newRunner()}
}

// Run validates req, starts the run in the background and returns its
// ai_runs id. It fails with domain.ErrDiscoveryInvalidMode for an unknown
// mode, domain.ErrDiscoveryCuisineRequired for an explore run without one,
// domain.ErrAIProviderMissing/ErrAIProviderFailed when no client resolves,
// and domain.ErrDiscoveryRunning while another run is running. The run
// outlives ctx.
func (s *Service) Run(ctx context.Context, req Request) (int64, error) {
	req.Cuisine, req.Focus = strings.TrimSpace(req.Cuisine), strings.TrimSpace(req.Focus)
	switch req.Mode {
	case ModeDaily, ModeKnown:
	case ModeExplore:
		if req.Cuisine == "" {
			return 0, domain.ErrDiscoveryCuisineRequired
		}
	default:
		return 0, fmt.Errorf("%w: %q", domain.ErrDiscoveryInvalidMode, req.Mode)
	}
	client, model, err := s.d.LLM.Current(ctx)
	switch {
	case errors.Is(err, llm.ErrProviderMissing):
		return 0, fmt.Errorf("%w: %w", domain.ErrAIProviderMissing, err)
	case err != nil:
		return 0, fmt.Errorf("%w: %w", domain.ErrAIProviderFailed, err)
	}
	if err := s.runner.acquire(); err != nil {
		return 0, err
	}
	scope, err := json.Marshal(struct {
		Mode    Mode   `json:"mode"`
		Cuisine string `json:"cuisine,omitempty"`
		Focus   string `json:"focus,omitempty"`
		Day     string `json:"day,omitempty"`
	}{req.Mode, req.Cuisine, req.Focus, req.Day})
	if err != nil {
		s.runner.release()
		return 0, err
	}
	runCtx, run := s.d.Recorder.Start(ctx, airun.Start{
		Kind: airun.KindDiscovery, PromptVersion: PromptVersion,
		Instruction: query(req), ScopeJSON: string(scope),
	})
	s.runner.start(runCtx, run.ID(), s.d.Timeout, func(ctx context.Context) {
		s.execute(ctx, run, client, model, req)
	})
	return run.ID(), nil
}

// Wait blocks until no run is running.
func (s *Service) Wait() { s.runner.wait() }

// Shutdown refuses new runs and waits for the running one; when ctx ends
// first the run is cancelled (it ends as an error) and ctx's error returned.
func (s *Service) Shutdown(ctx context.Context) error { return s.runner.shutdown(ctx) }

// execute runs the pipeline and finishes the run.
func (s *Service) execute(ctx context.Context, run *airun.Run, client llm.Client, model string, req Request) {
	status, errKey, res := airun.StatusError, "error.server", airun.Result{}
	defer func() {
		if p := recover(); p != nil {
			s.d.Logger.ErrorContext(ctx, "discovery.run panic", "run_id", run.ID(), "panic", p)
			status, errKey = airun.StatusError, "error.server"
		}
		run.FinishWith(ctx, status, errKey, res)
	}()
	var err error
	status, res, err = s.pipeline(ctx, run.ID(), client, model, req)
	if err != nil {
		key, _ := domain.MessageKey(err, "discovery")
		if ctx.Err() != nil {
			key = "error.server"
		}
		status, errKey = airun.StatusError, key
		s.d.Logger.WarnContext(ctx, "discovery.run failed", "run_id", run.ID(), "error", err)
		return
	}
	errKey = ""
}

func (s *Service) pipeline(ctx context.Context, runID int64, client llm.Client, model string, req Request) (airun.Status, airun.Result, error) {
	var res airun.Result
	if n, err := s.d.Store.DeleteStale(ctx, s.d.Now().Add(-staleAfter)); err != nil {
		// Cleanup is housekeeping; it must not fail the run.
		s.d.Logger.WarnContext(ctx, "discovery.stale_cleanup failed", "run_id", runID, "error", err)
	} else if n > 0 {
		s.d.Logger.InfoContext(ctx, "discovery.stale_deleted", "count", n)
	}
	in, err := s.promptInput(ctx, req)
	if err != nil {
		return "", res, err
	}
	found, err := research(ctx, client, model, s.webSearch(ctx, client, model), RenderPrompt(in), req)
	if found.Repaired {
		res.RepairAttempts = 1
	}
	if err != nil {
		return "", res, err
	}
	verified, discarded, err := s.d.Verifier.Verify(ctx, found.Candidates)
	if err != nil {
		return "", res, err
	}
	res.Notes = notes(len(verified), len(found.Candidates), discarded)
	if len(verified) == 0 {
		return airun.StatusPartial, res, nil
	}

	verified = verified[:min(len(verified), maxStored)]
	anns := make([]*Annotations, len(verified))
	for i, v := range verified {
		if anns[i], err = s.d.Annotator.Annotate(ctx, v.Draft); err != nil {
			return "", res, err
		}
		anns[i].Why, anns[i].Cuisine = v.Candidate.Why, v.Candidate.Cuisine
	}
	shownOn, err := s.firstShownOn(ctx, req)
	if err != nil {
		return "", res, err
	}
	for i, v := range verified {
		in := NewSuggestion{
			Mode: req.Mode, Query: query(req), Draft: v.Draft, Annotations: *anns[i],
			Grounded: found.Searches > 0, RunID: &runID, ShownOn: shownOn,
		}
		_, err := s.d.Store.Save(ctx, in)
		if errors.Is(err, ErrDayTaken) {
			// A concurrent request took the day meanwhile: keep the paid result.
			in.ShownOn = nil
			_, err = s.d.Store.Save(ctx, in)
		}
		if err != nil {
			return "", res, err
		}
		shownOn = nil
	}
	return airun.StatusOK, res, nil
}

// firstShownOn is the shown_on of a daily run's first suggestion: its day,
// unless a suggestion is shown that day already (D12).
func (s *Service) firstShownOn(ctx context.Context, req Request) (*string, error) {
	if req.Day == "" {
		return nil, nil
	}
	cur, err := s.d.Store.Repo.ShownOn(ctx, req.Day)
	if err != nil || cur != nil {
		return nil, err
	}
	return &req.Day, nil
}

func (s *Service) promptInput(ctx context.Context, req Request) (PromptInput, error) {
	in := PromptInput{Request: req}
	var err error
	if in.Profile, err = s.d.Profile.Get(ctx); err != nil {
		return in, fmt.Errorf("profile: %w", err)
	}
	if in.Rejected, err = s.d.Store.Rejected(ctx, rejectedInPrompt); err != nil {
		return in, fmt.Errorf("rejected suggestions: %w", err)
	}
	if in.RecipeNames, err = s.d.Catalogue.RecipeNames(ctx, recipesInPrompt); err != nil {
		return in, fmt.Errorf("recipe names: %w", err)
	}
	if req.Mode == ModeKnown {
		if in.LeafNames, err = s.d.Catalogue.LeafNamesByRecipeUse(ctx, leavesInPrompt); err != nil {
			return in, fmt.Errorf("leaf names: %w", err)
		}
	}
	return in, nil
}

// webSearch reports whether the provider can search for model (D3); a
// client without the capability port, or a failed check, means knowledge
// mode.
func (s *Service) webSearch(ctx context.Context, client llm.Client, model string) bool {
	caps, ok := client.(llm.Capabilities)
	if !ok {
		return false
	}
	on, err := caps.WebSearch(ctx, model)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "discovery.capability check failed", "model", model, "error", err)
		return false
	}
	return on
}

// notes is the run's notes: "<kept> of <total> verified" and the discard
// reasons with their counts.
func notes(kept, total int, discarded []Discarded) string {
	s := fmt.Sprintf("%d of %d verified", kept, total)
	if len(discarded) == 0 {
		return s
	}
	counts := map[string]int{}
	for _, d := range discarded {
		counts[d.Reason]++
	}
	var parts []string
	for _, r := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s %d", r, counts[r]))
	}
	return s + "; discarded: " + strings.Join(parts, ", ")
}

// query is the run's cuisine or focus text, nil when there is none.
func query(req Request) *string {
	q := req.Cuisine
	if req.Mode == ModeKnown {
		q = req.Focus
	}
	if q == "" {
		return nil
	}
	return &q
}
