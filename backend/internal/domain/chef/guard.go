// Package chef holds the Chef's capped background work: the cap guard every
// capped run asks first (COST-1..3) and the job runner that runs queued
// capped work.
package chef

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// CappedKinds are the run kinds whose cost counts against chef.monthly_cap
// (COST-1). Planning and chat are not capped.
var CappedKinds = []airun.Kind{
	airun.KindChefAttributes, airun.KindChefMatch, airun.KindChefImportCheck,
	airun.KindChefFind, airun.KindChefHelp, airun.KindChefLibraryReview,
}

// Capped reports whether k is a capped kind.
func Capped(k airun.Kind) bool {
	for _, c := range CappedKinds {
		if c == k {
			return true
		}
	}
	return false
}

// Reason says why a capped run may not run.
type Reason string

const (
	// ReasonNoProvider: no usable AI provider is configured.
	ReasonNoProvider Reason = "no_provider"
	// ReasonOff: chef.enabled is off (ON-1).
	ReasonOff Reason = "off"
	// ReasonUnpriced: the model has no known price (COST-3).
	ReasonUnpriced Reason = "unpriced"
	// ReasonCap: this month's capped spend reached the cap (COST-2).
	ReasonCap Reason = "cap"
)

// Decision is the guard's answer for one capped run.
type Decision struct {
	Run bool
	// Warn: requested work at the cap runs after the user saw a warning.
	Warn bool
	// Reason is set when Run is false, or ReasonCap with Warn.
	Reason Reason
}

// Settings is what the guard reads from the settings service.
type Settings interface {
	ChefEnabled(ctx context.Context) (bool, error)
	ChefMonthlyCap(ctx context.Context) (float64, error)
	EffectiveAI(ctx context.Context) (settings.AIConfig, error)
}

// Guard decides whether a capped run may run.
type Guard struct {
	Settings Settings
	Spend    airun.SpendStore
	// Prices returns ai.price_overrides.
	Prices func(ctx context.Context) (map[string]airun.Price, error)
	Now    func() time.Time
}

// MonthSpend is this month's capped spend against the cap (COST-4).
type MonthSpend struct {
	Month    string  `json:"month"`
	SpentUSD float64 `json:"spent_usd"`
	CapUSD   float64 `json:"cap_usd"`
	AtCap    bool    `json:"at_cap"`
}

// Spent returns this month's capped spend and the cap.
func (g *Guard) Spent(ctx context.Context) (MonthSpend, error) {
	now := g.Now()
	spent, err := airun.Spend(ctx, g.Spend, now, CappedKinds)
	if err != nil {
		return MonthSpend{}, err
	}
	capUSD, err := g.Settings.ChefMonthlyCap(ctx)
	if err != nil {
		return MonthSpend{}, fmt.Errorf("monthly cap: %w", err)
	}
	return MonthSpend{Month: now.Format("2006-01"), SpentUSD: spent, CapUSD: capUSD, AtCap: spent >= capUSD}, nil
}

// Check decides one run of kind. requested is work the user asked for;
// everything else is autonomous. Help always runs (HELP-8). Otherwise a
// provider and the switch are needed; autonomous work also needs a priced
// model and spend under the cap, while requested work at the cap runs with
// Warn set.
func (g *Guard) Check(ctx context.Context, kind airun.Kind, requested bool) (Decision, error) {
	if !Capped(kind) {
		return Decision{}, fmt.Errorf("chef guard: %q is not a capped kind", kind)
	}
	if kind == airun.KindChefHelp {
		return Decision{Run: true}, nil
	}
	ai, err := g.Settings.EffectiveAI(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("chef guard: ai settings: %w", err)
	}
	if ai.Provider == "" || (ai.APIKey == "" && ai.Provider != "fake") {
		return Decision{Reason: ReasonNoProvider}, nil
	}
	on, err := g.Settings.ChefEnabled(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("chef guard: switch: %w", err)
	}
	if !on {
		return Decision{Reason: ReasonOff}, nil
	}
	if !requested {
		overrides, err := g.Prices(ctx)
		if err != nil {
			return Decision{}, fmt.Errorf("chef guard: price overrides: %w", err)
		}
		if !airun.Priced(ai.Model, overrides) {
			return Decision{Reason: ReasonUnpriced}, nil
		}
	}
	spend, err := g.Spent(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("chef guard: %w", err)
	}
	switch {
	case !spend.AtCap:
		return Decision{Run: true}, nil
	case requested:
		return Decision{Run: true, Warn: true, Reason: ReasonCap}, nil
	default:
		return Decision{Reason: ReasonCap}, nil
	}
}
