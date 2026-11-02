package discovery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Daily is the answer of the daily endpoints (D12): a suggestion, a running
// run, or neither (daily off, no provider, or today's run failed: silent).
type Daily struct {
	Suggestion *Suggestion
	RunID      int64
}

// dayKey is the shown_on value of a day.
func dayKey(day time.Time) string { return day.Format(time.DateOnly) }

// Daily returns the suggestion shown on day; else assigns the oldest
// pending one; else reports the running run; else starts a daily run —
// unless a run for day failed already.
func (s *Service) Daily(ctx context.Context, day time.Time) (Daily, error) {
	if on, err := s.dailyAvailable(ctx); !on || err != nil {
		return Daily{}, err
	}
	cur, err := s.d.Store.Repo.ShownOn(ctx, dayKey(day))
	if err != nil {
		return Daily{}, err
	}
	if cur != nil {
		if cur.Status != StatusPending {
			// The user acted on today's suggestion: silent for the day.
			return Daily{}, nil
		}
		return Daily{Suggestion: cur}, nil
	}
	return s.next(ctx, day, 0)
}

// Another replaces the suggestion shown on day with the next pending one
// (the replaced one stays pending) or, when there is none, starts a daily
// run whose first suggestion becomes day's.
func (s *Service) Another(ctx context.Context, day time.Time) (Daily, error) {
	if on, err := s.dailyAvailable(ctx); !on || err != nil {
		return Daily{}, err
	}
	cur, err := s.d.Store.Repo.ShownOn(ctx, dayKey(day))
	if err != nil {
		return Daily{}, err
	}
	var after int64
	if cur != nil {
		if cur.Status != StatusPending {
			return Daily{}, nil
		}
		after = cur.ID
	}
	return s.next(ctx, day, after)
}

// next shows the next pending suggestion after the id on day, or falls back
// to a run.
func (s *Service) next(ctx context.Context, day time.Time, after int64) (Daily, error) {
	key := dayKey(day)
	sug, err := s.d.Store.Repo.ShowNext(ctx, key, after)
	if err != nil || sug != nil {
		return Daily{Suggestion: sug}, err
	}
	if id, ok := s.runner.current(); ok {
		return Daily{RunID: id}, nil
	}
	failed, err := s.d.Store.Repo.DailyRunFailed(ctx, key)
	if err != nil || failed {
		return Daily{}, err
	}
	cuisines, err := s.d.Prefs.DiscoveryCuisines(ctx)
	if err != nil {
		return Daily{}, fmt.Errorf("discovery cuisines: %w", err)
	}
	// After Another (after != 0) the run's first suggestion takes the day;
	// the shown one keeps it until then (see firstShownOn).
	id, err := s.Run(ctx, Request{Mode: ModeDaily, Cuisine: DailyCuisine(cuisines, day), Day: key, replaceShown: after != 0})
	switch {
	case errors.Is(err, domain.ErrDiscoveryRunning):
		// Another run took the slot meanwhile.
		id, _ = s.runner.current()
		return Daily{RunID: id}, nil
	case errors.Is(err, domain.ErrAIProviderMissing), errors.Is(err, domain.ErrAIProviderFailed):
		// Silent for the user, but not for the operator.
		s.d.Logger.WarnContext(ctx, "discovery.daily run not started", "error", err)
		return Daily{}, nil
	case err != nil:
		return Daily{}, err
	}
	return Daily{RunID: id}, nil
}

// dailyAvailable reports whether the daily suggestion is on and a provider
// is configured. A provider that fails to resolve counts as none (the daily
// popup is silent).
func (s *Service) dailyAvailable(ctx context.Context) (bool, error) {
	on, err := s.d.Prefs.DiscoveryDailyEnabled(ctx)
	if err != nil {
		return false, fmt.Errorf("discovery daily setting: %w", err)
	}
	if !on {
		return false, nil
	}
	if _, _, err := s.d.LLM.Current(ctx); err != nil {
		if !errors.Is(err, llm.ErrProviderMissing) {
			s.d.Logger.WarnContext(ctx, "discovery.daily provider unavailable", "error", err)
		}
		return false, nil
	}
	return true, nil
}

// Accept marks the suggestion accepted as the saved food foodID (D11) and,
// when the food has no image, copies the suggestion's image to it (D16).
// An unknown food is domain.ErrNotFound; a failed copy is logged only (the
// food's image path is cleared again).
func (s *Service) Accept(ctx context.Context, id, foodID int64) (*Suggestion, error) {
	f, err := s.d.Foods.Get(ctx, foodID)
	if err != nil {
		return nil, err
	}
	sug, err := s.d.Store.Accept(ctx, id, foodID)
	if err != nil {
		return nil, err
	}
	if sug.ImagePath == nil || f.ImagePath != nil || s.d.Store.Images == nil {
		return sug, nil
	}
	// Claim the food's image path first, so a food that got an image since
	// the read keeps its file: only the claimant writes foods/<id>.jpg.
	want := filepath.Join(foodImageCategory, fmt.Sprintf("%d.jpg", foodID))
	claimed, err := s.d.Foods.SetImageIfNone(ctx, foodID, want)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "discovery.accept image path update failed", "food_id", foodID, "error", err)
		return sug, nil
	}
	if !claimed {
		return sug, nil
	}
	if _, err := s.d.Store.Images.Copy(ImageCategory, sug.ID, foodImageCategory, foodID); err != nil {
		s.d.Logger.WarnContext(ctx, "discovery.accept image copy failed", "suggestion_id", id, "food_id", foodID, "error", err)
		if clrErr := s.d.Foods.ClearImageIf(ctx, foodID, want); clrErr != nil {
			s.d.Logger.WarnContext(ctx, "discovery.accept image path clear failed", "food_id", foodID, "error", clrErr)
		}
	}
	return sug, nil
}

// foodImageCategory is the imagestore category of food images.
const foodImageCategory = "foods"

// Reject marks the suggestion rejected (D11) and removes its image (D16).
func (s *Service) Reject(ctx context.Context, id int64, reason *Reason) (*Suggestion, error) {
	return s.d.Store.Reject(ctx, id, reason)
}

// maxListed bounds Suggestions.
const maxListed = 200

// Suggestions lists suggestions by status and/or run, last decided (else
// newest) first, at most 200.
func (s *Service) Suggestions(ctx context.Context, status *Status, runID *int64) ([]Suggestion, error) {
	return s.d.Store.Repo.List(ctx, ListFilter{Status: status, RunID: runID, Limit: maxListed})
}

// RunState is a discovery run's progress for the client.
type RunState struct {
	RunID         int64
	Status        string
	SuggestionIDs []int64
	// Discarded counts the candidates that failed verification.
	Discarded int
	ErrorKey  *string
}

// RunState reports a discovery run or domain.ErrNotFound.
func (s *Service) RunState(ctx context.Context, id int64) (RunState, error) {
	row, err := s.d.Store.Repo.Run(ctx, id)
	if err != nil {
		return RunState{}, err
	}
	sugs, err := s.d.Store.Repo.List(ctx, ListFilter{RunID: &id, Limit: maxListed})
	if err != nil {
		return RunState{}, err
	}
	ids := make([]int64, len(sugs))
	for i, sg := range sugs {
		ids[i] = sg.ID
	}
	return RunState{RunID: row.ID, Status: row.Status, SuggestionIDs: ids, Discarded: discardedIn(row.Notes), ErrorKey: row.ErrorKey}, nil
}

// discardedIn reads the discarded count from a run's notes ("<kept> of
// <total> verified…", see notes); 0 when they hold none.
func discardedIn(notes string) int {
	var kept, total int
	if _, err := fmt.Sscanf(notes, "%d of %d verified", &kept, &total); err != nil {
		return 0
	}
	return total - kept
}

// Discover starts a run requested through the API; a daily run without a
// cuisine gets today's from the rotation (D8).
func (s *Service) Discover(ctx context.Context, req Request, today time.Time) (int64, error) {
	if req.Mode == ModeDaily && strings.TrimSpace(req.Cuisine) == "" {
		cuisines, err := s.d.Prefs.DiscoveryCuisines(ctx)
		if err != nil {
			return 0, fmt.Errorf("discovery cuisines: %w", err)
		}
		req.Cuisine = DailyCuisine(cuisines, today)
	}
	return s.Run(ctx, req)
}
