// Package settings provides a runtime, DB-backed store for user-editable
// application options with env-var fallback. Values are resolved in the order
// database > environment > default. API keys and similarly sensitive values
// are encrypted at rest via adapters/crypto.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Source describes where the effective value for a setting comes from.
type Source string

const (
	SourceDB      Source = "db"
	SourceEnv     Source = "env"
	SourceDefault Source = "default"
)

// Sentinel errors.
var (
	ErrUnknownKey  = errors.New("unknown settings key")
	ErrReadOnlyKey = errors.New("settings key is read-only")
	ErrInvalidKind = errors.New("invalid value for settings key")
)

// Kind describes the canonical type of a setting's value.
type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
	KindEnum   Kind = "enum"
)

// Spec describes a single setting: its kind, whether it stores a secret that
// must be encrypted at rest, optional env-var fallback, and the default used
// when neither db nor env contributes.
type Spec struct {
	Key        string
	Kind       Kind
	Encrypted  bool
	EnvVar     string
	Default    string
	AllowEmpty bool
	Enum       []string // valid values when Kind == KindEnum
	ReadOnly   bool     // true for infra/system info surfaced in /api/settings/system
	// Min and Max constrain KindInt values. Zero values are ignored unless
	// HasRange is true.
	HasRange bool
	Min      int
	Max      int
	// Validate, when set, checks a KindString value's shape.
	Validate func(raw string) error
}

// Value is a resolved setting result.
type Value struct {
	Key           string
	Raw           string
	Source        Source
	IsSecret      bool
	MaskedPreview string
	EnvAlsoSet    bool
	ReadOnly      bool
}

// Mask renders a secret with only the first and last few chars visible. The
// full value is replaced with a fixed-width mask when too short to safely
// preview.
func Mask(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) < 8 {
		return "********"
	}
	return raw[:3] + "****" + raw[len(raw)-4:]
}

// AIConfig is the effective AI configuration the resolver needs to build an
// LLM client.
type AIConfig struct {
	Provider        string
	Model           string
	APIKey          string
	FakeScript      string
	RateLimitPerMin int
}

// Editable keys — must stay in sync with the registry in specs.go.
const (
	KeyAIProvider   = "ai.provider"
	KeyAIModel      = "ai.model"
	KeyAIAPIKey     = "ai.api_key"
	KeyAIRateLimit  = "ai.rate_limit_per_min"
	KeyAIFakeScript = "ai.fake_script"
	KeyFDCAPIKey    = "fdc.api_key"

	KeyPlanAnchor      = "plan.anchor"
	KeyPlanShoppingDay = "plan.shopping_day"

	KeySetupCompleted             = "setup.completed"
	KeyPlanMaxCookOccasionsPerDay = "plan.max_cook_occasions_per_day"
	KeyPlanBatchCooking           = "plan.batch_cooking"
	KeyPlanDefaultKeepDays        = "plan.default_keep_days"
	KeyPlanFreezingEnabled        = "plan.freezing_enabled"
	KeyPlanFreezerKeepDays        = "plan.freezer_keep_days"
	KeyPlanVarietyWeeks           = "plan.variety_weeks"
	KeyChefNewDishesPerWeek       = "chef.new_dishes_per_week"
	KeyPlanMaxDishMealsPerWeek    = "plan.max_dish_meals_per_week"
	KeyDiscoveryDailyEnabled      = "discovery.daily_enabled"
	KeyDiscoveryCuisines          = "discovery.cuisines"
	KeyAIRunRetentionDays         = "ai.run_retention_days"
	KeyAIPriceOverrides           = "ai.price_overrides"
	KeyAIReasoningEffortChat      = "ai.reasoning_effort.chat"
	KeyAIReasoningEffortGenerate  = "ai.reasoning_effort.generate"
	KeyAIProgrammaticTools        = "ai.programmatic_tools"
	KeyChefEnabled                = "chef.enabled"
	KeyChefMonthlyCap             = "chef.monthly_cap"
	KeyChefSites                  = "chef.sites"
	KeyPlanPlateFit               = "plan.plate_fit"
	KeyChefBatchVariation         = "chef.batch_variation"
	KeyChefLibraryReview          = "chef.library_review_enabled"
)

// maxCuisines and maxCuisineLen bound discovery.cuisines (D19).
const (
	maxCuisines   = 10
	maxCuisineLen = 40
)

// ParseCuisines parses discovery.cuisines: a JSON array of at most 10
// non-empty, lowercase, trimmed names.
func ParseCuisines(raw string) ([]string, error) {
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil || names == nil {
		return nil, fmt.Errorf("%w: %s must be a JSON array of names", ErrInvalidKind, KeyDiscoveryCuisines)
	}
	if len(names) > maxCuisines {
		return nil, fmt.Errorf("%w: %s holds at most %d names", ErrInvalidKind, KeyDiscoveryCuisines, maxCuisines)
	}
	for _, n := range names {
		if n == "" || n != strings.ToLower(strings.TrimSpace(n)) || utf8.RuneCountInString(n) > maxCuisineLen {
			return nil, fmt.Errorf("%w: %s name %q must be lowercase, trimmed and 1-%d characters", ErrInvalidKind, KeyDiscoveryCuisines, n, maxCuisineLen)
		}
	}
	return names, nil
}

// ParseMonthlyCap parses chef.monthly_cap: a finite, non-negative USD amount.
func ParseMonthlyCap(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative amount in USD", ErrInvalidKind, KeyChefMonthlyCap)
	}
	return v, nil
}

// ChefSite is one entry of the chef.sites allow-list (FIND-7).
type ChefSite struct {
	Domain string `json:"domain"`
	Lang   string `json:"lang"`
	// Enabled sites are searched for finds.
	Enabled bool `json:"enabled"`
	// AIGroupsExempt: robots.txt groups for AI user agents do not block
	// this site (FIND-6).
	AIGroupsExempt bool `json:"ai_groups_exempt"`
}

// defaultChefSites is FIND-7's list: the first eleven on, the rest listed
// but off.
const defaultChefSites = `[` +
	`{"domain":"bbcgoodfood.com","lang":"en","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"budgetbytes.com","lang":"en","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"minimalistbaker.com","lang":"en","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"justonecookbook.com","lang":"en","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"thewoksoflife.com","lang":"en","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"chefkoch.de","lang":"de","enabled":true,"ai_groups_exempt":true},` +
	`{"domain":"einfachbacken.de","lang":"de","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"kuechengoetter.de","lang":"de","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"eatsmarter.de","lang":"de","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"malteskitchen.de","lang":"de","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"zuckerundjagdwurst.com","lang":"de","enabled":true,"ai_groups_exempt":false},` +
	`{"domain":"bonappetit.com","lang":"en","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"epicurious.com","lang":"en","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"kitchenstories.com","lang":"de","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"recipetineats.com","lang":"en","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"essen-und-trinken.de","lang":"de","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"cooking.nytimes.com","lang":"en","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"seriouseats.com","lang":"en","enabled":false,"ai_groups_exempt":false},` +
	`{"domain":"allrecipes.com","lang":"en","enabled":false,"ai_groups_exempt":false}` +
	`]`

// DefaultChefSites are FIND-7's built-in sites: they can be disabled, never
// removed.
func DefaultChefSites() []ChefSite {
	sites, err := ParseChefSites(defaultChefSites)
	if err != nil {
		panic(err) // the constant is checked by a test
	}
	return sites
}

// ParseChefSites parses chef.sites: a JSON array of sites with unique,
// lowercase, trimmed domains and lang en or de.
func ParseChefSites(raw string) ([]ChefSite, error) {
	var sites []ChefSite
	if err := json.Unmarshal([]byte(raw), &sites); err != nil || sites == nil {
		return nil, fmt.Errorf("%w: %s must be a JSON array of sites", ErrInvalidKind, KeyChefSites)
	}
	seen := make(map[string]bool, len(sites))
	for _, s := range sites {
		if s.Domain == "" || s.Domain != strings.ToLower(strings.TrimSpace(s.Domain)) {
			return nil, fmt.Errorf("%w: %s domain %q must be lowercase and trimmed", ErrInvalidKind, KeyChefSites, s.Domain)
		}
		if s.Lang != "en" && s.Lang != "de" {
			return nil, fmt.Errorf("%w: %s lang of %s must be en or de", ErrInvalidKind, KeyChefSites, s.Domain)
		}
		if seen[s.Domain] {
			return nil, fmt.Errorf("%w: %s lists %s twice", ErrInvalidKind, KeyChefSites, s.Domain)
		}
		seen[s.Domain] = true
	}
	return sites, nil
}

// Getter reads one effective setting.
type Getter interface {
	Get(ctx context.Context, key string) (Value, error)
}

// NewDishesPerWeek is chef.new_dishes_per_week as planning uses it
// (THEME-6): the stored value, else 1 with the Chef on and 0 with it off
// (the explore quota's old default).
func NewDishesPerWeek(ctx context.Context, g Getter) (int, error) {
	v, err := g.Get(ctx, KeyChefNewDishesPerWeek)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", KeyChefNewDishesPerWeek, err)
	}
	if v.Source == SourceDefault {
		on, err := g.Get(ctx, KeyChefEnabled)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", KeyChefEnabled, err)
		}
		if ok, _ := strconv.ParseBool(on.Raw); !ok {
			return 0, nil
		}
	}
	n, err := strconv.Atoi(v.Raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", KeyChefNewDishesPerWeek, err)
	}
	return n, nil
}
