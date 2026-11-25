package flavor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// Labelling queues the assistant's attribute labelling as capped
// chef_attributes jobs (ATTR-2, ON-2): one job per saved recipe, and the
// one-time pass over the library's unlabelled recipes in batches.
type Labelling struct {
	labeler *Labeler
	library Library
	queue   Queue
	chef    Switch
	price   Pricer
	// workers is how many jobs run at once (for the duration estimate).
	workers int
}

// Library lists the recipes the library pass labels.
type Library interface {
	UnlabelledRecipes(ctx context.Context) ([]int64, error)
}

// Queue enqueues a capped job (chef.Runner).
type Queue interface {
	Enqueue(ctx context.Context, kind airun.Kind, payload string, requested bool) (int64, error)
}

// Switch reads chef.enabled.
type Switch interface {
	ChefEnabled(ctx context.Context) (bool, error)
}

// Pricer prices tokens at the configured model; nil when it has no price.
type Pricer func(ctx context.Context, input, output int64) (*float64, error)

// NewLabelling creates the labelling queue.
func NewLabelling(labeler *Labeler, library Library, queue Queue, chef Switch, price Pricer, workers int) *Labelling {
	return &Labelling{labeler: labeler, library: library, queue: queue, chef: chef, price: price, workers: max(workers, 1)}
}

// ErrChefOff: the library pass needs chef.enabled on.
var ErrChefOff = errors.New("the Chef is off")

// labelJob is a chef_attributes job's payload.
type labelJob struct {
	FoodIDs []int64 `json:"food_ids"`
}

// Handle runs a chef_attributes job.
func (s *Labelling) Handle(ctx context.Context, payload string) error {
	var job labelJob
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		return fmt.Errorf("label job payload: %w", err)
	}
	return s.labeler.Label(ctx, job.FoodIDs)
}

func (s *Labelling) enqueue(ctx context.Context, ids []int64) error {
	b, err := json.Marshal(labelJob{FoodIDs: ids})
	if err != nil {
		return err
	}
	_, err = s.queue.Enqueue(ctx, airun.KindChefAttributes, string(b), false)
	return err
}

// Saved queues the labelling of a saved composed food when the Chef is on
// (ATTR-2: saved, imported or edited).
func (s *Labelling) Saved(ctx context.Context, foodID int64) error {
	on, err := s.chef.ChefEnabled(ctx)
	if err != nil || !on {
		return err
	}
	return s.enqueue(ctx, []int64{foodID})
}

// Estimate is what the library pass will label and roughly cost (ON-2).
type Estimate struct {
	Recipes int `json:"recipes"`
	Calls   int `json:"calls"`
	// CostUSD is nil when the configured model has no price.
	CostUSD *float64 `json:"cost_usd"`
	Minutes int      `json:"minutes"`
}

// Rough sizes for the estimate: tokens a recipe's labels take in the reply
// and seconds one call takes.
const (
	outputTokensPerRecipe = 160
	secondsPerCall        = 30
)

// batches splits the library's unlabelled recipes into jobs.
func (s *Labelling) batches(ctx context.Context) ([][]int64, error) {
	ids, err := s.library.UnlabelledRecipes(ctx)
	if err != nil {
		return nil, err
	}
	var out [][]int64
	for len(ids) > 0 {
		n := min(LabelBatch, len(ids))
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	return out, nil
}

// Estimate prices the library pass from the prompts it would send (input
// tokens as characters / 4) and a fixed reply size per recipe.
func (s *Labelling) Estimate(ctx context.Context) (Estimate, error) {
	batches, err := s.batches(ctx)
	if err != nil {
		return Estimate{}, err
	}
	var est Estimate
	var input int64
	for _, ids := range batches {
		items, err := s.labeler.load(ctx, ids)
		if err != nil {
			return Estimate{}, err
		}
		if len(items) == 0 {
			continue
		}
		req := labelRequest("", items)
		chars := len(req.System) + len(req.Messages[0].Content[0].Text) + len(req.Tools[0].Schema) + len(req.Tools[0].Description)
		input += int64(chars / 4)
		est.Recipes += len(items)
		est.Calls++
	}
	if est.Calls == 0 {
		zero := 0.0
		est.CostUSD = &zero
		return est, nil
	}
	est.CostUSD, err = s.price(ctx, input, int64(est.Recipes*outputTokensPerRecipe))
	if err != nil {
		return Estimate{}, err
	}
	rounds := (est.Calls + s.workers - 1) / s.workers
	est.Minutes = max(1, (rounds*secondsPerCall+59)/60)
	return est, nil
}

// LabelLibrary queues the one-time pass over the library's unlabelled
// recipes and returns how many jobs it queued. The Chef must be on.
func (s *Labelling) LabelLibrary(ctx context.Context) (int, error) {
	on, err := s.chef.ChefEnabled(ctx)
	if err != nil {
		return 0, err
	}
	if !on {
		return 0, ErrChefOff
	}
	batches, err := s.batches(ctx)
	if err != nil {
		return 0, err
	}
	for i, ids := range batches {
		if err := s.enqueue(ctx, ids); err != nil {
			return i, fmt.Errorf("queue library labelling: %w", err)
		}
	}
	return len(batches), nil
}
