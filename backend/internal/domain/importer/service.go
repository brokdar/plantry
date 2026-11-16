package importer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// Fetcher retrieves an HTML document from a URL. Implemented by adapters/httpfetch.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (html, finalURL string, err error)
}

// JSONLDExtractor pulls a schema.org Recipe out of raw HTML.
// Implemented by adapters/jsonld.
type JSONLDExtractor interface {
	ExtractRecipe(html string) (*RawRecipe, error)
}

// Resolver resolves a raw ingredient line against the local food catalogue
// and external sources. Implemented by domain/food.Resolver.
type Resolver interface {
	Lookup(ctx context.Context, barcode, query, lang string, limit int) ([]food.Candidate, int, error)
}

// PortionStore reads and writes a leaf's unit → grams portions. Finalize
// resolves count lines through it. Implemented by food.Service.
type PortionStore interface {
	Get(ctx context.Context, id int64) (*food.Food, error)
	ListPortions(ctx context.Context, foodID int64) ([]food.Portion, error)
	UpsertPortion(ctx context.Context, p *food.Portion) error
}

// Service orchestrates recipe import: fetch → JSON-LD → LLM fallback → parse lines.
type Service struct {
	fetcher     Fetcher
	jsonld      JSONLDExtractor
	llmResolver llm.Resolver // may be nil — LLM fallback then returns ErrAIProviderMissing.
	resolver    Resolver
	portions    PortionStore
	recorder    *airun.Recorder // may be nil: the LLM fallback is then not recorded
}

// NewService builds an import Service. llmResolver may be nil; when set, it
// is consulted per-request so configuration changes take effect without a
// restart.
func NewService(f Fetcher, j JSONLDExtractor, llmResolver llm.Resolver, r Resolver, portions PortionStore) *Service {
	return &Service{fetcher: f, jsonld: j, llmResolver: llmResolver, resolver: r, portions: portions}
}

// WithRecorder records each LLM fallback extraction as an import_fallback AI run.
func (s *Service) WithRecorder(rec *airun.Recorder) *Service {
	s.recorder = rec
	return s
}

// ExtractInput selects the source for extraction. Exactly one of URL or HTML
// must be non-empty.
type ExtractInput struct {
	URL  string
	HTML string
	Options
}

// Options tune one Extract call.
type Options struct {
	// NoLLMFallback skips the LLM when the page has no JSON-LD recipe: the
	// call then fails with domain.ErrImportNoRecipe at no AI cost (discovery
	// verification, D9).
	NoLLMFallback bool
}

// Extract loads a page (or uses the supplied HTML), extracts the recipe, and
// returns a draft ready for review. The returned draft always has ingredient
// lines parsed into amounts and units when possible.
func (s *Service) Extract(ctx context.Context, in ExtractInput) (*Draft, error) {
	if (in.URL == "" && in.HTML == "") || (in.URL != "" && in.HTML != "") {
		return nil, fmt.Errorf("%w: provide exactly one of url or html", domain.ErrInvalidInput)
	}

	htmlBody := in.HTML
	sourceURL := in.URL
	host := ""
	var warnings []string
	if htmlBody == "" {
		u, err := url.Parse(in.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return nil, &ExtractError{Stage: StageFetch, Err: domain.ErrImportInvalidURL}
		}
		host = u.Hostname()
		body, final, err := s.fetcher.Fetch(ctx, in.URL)
		if err != nil {
			return nil, &ExtractError{Stage: StageFetch, Host: host, Err: err}
		}
		htmlBody = body
		if final != "" {
			sourceURL = final
			if redirectedElsewhere(in.URL, final) {
				warnings = append(warnings, WarningRedirectedOtherRecipe)
			}
		}
	}

	rec, err := s.jsonld.ExtractRecipe(htmlBody)
	if err == nil && len(rec.RecipeIngredient) > 0 {
		return s.recipeToDraft(rec, sourceURL, "jsonld", warnings), nil
	}
	if err != nil && !errors.Is(err, ErrNoRecipe) {
		slog.DebugContext(ctx, "import: json-ld extraction error", "host", host, "error", err)
	}

	if in.NoLLMFallback {
		return nil, &ExtractError{Stage: StageParse, Host: host, Err: domain.ErrImportNoRecipe}
	}

	rec, llmErr := s.llmExtract(ctx, htmlBody)
	switch {
	case errors.Is(llmErr, domain.ErrAIProviderMissing):
		// No JSON-LD recipe and no assistant to read the page: tell the user
		// about the page, not about server configuration.
		return nil, &ExtractError{Stage: StageParse, Host: host, Err: domain.ErrImportNoRecipeNoAI}
	case llmErr != nil:
		return nil, &ExtractError{Stage: StageFallback, Host: host, Err: llmErr}
	case rec == nil || len(rec.RecipeIngredient) == 0:
		return nil, &ExtractError{Stage: StageFallback, Host: host, Err: domain.ErrImportNoRecipe}
	}
	return s.recipeToDraft(rec, sourceURL, "llm", warnings), nil
}

// Import stages named in ExtractError and the import log.
const (
	StageFetch    = "fetch"
	StageParse    = "parse_jsonld"
	StageFallback = "fallback"
	StageResolve  = "resolve"
)

// ExtractError tags an Extract failure with the stage that failed and the
// host of the page, so the handler can name the host to the user and log the
// stage. It unwraps to the domain sentinel.
type ExtractError struct {
	Stage string
	Host  string
	Err   error
}

func (e *ExtractError) Error() string {
	return fmt.Sprintf("import %s (%s): %v", e.Stage, e.Host, e.Err)
}
func (e *ExtractError) Unwrap() error { return e.Err }

// ResolveLine wraps the food resolver used from the per-row lookup endpoint
// in the review step. The recommended-index return value is not exposed.
func (s *Service) ResolveLine(ctx context.Context, query, lang string) ([]food.Candidate, error) {
	if clean, _ := CleanName(query); clean != "" {
		query = clean
	}
	results, _, err := s.resolver.Lookup(ctx, "", query, lang, 5)
	return results, err
}

// FinalizeInput is what the review step submits after the user has resolved
// every ingredient line. Each row resolves to a child food (leaf) for the new
// composed food.
type FinalizeInput struct {
	Name              string
	Role              string
	ReferencePortions float64
	PrepMinutes       *int
	CookMinutes       *int
	Notes             *string
	Tags              []string
	Instructions      []FinalizedInstruction
	Children          []FinalizedChild
}

// ResolutionExisting / ResolutionSkip / ResolutionNew classify a per-row decision.
const (
	ResolutionExisting = "existing"
	ResolutionSkip     = "skip"
	ResolutionNew      = "new"
)

// FinalizedChild is one resolved ingredient row: pick an existing food,
// create a new one before POSTing, or skip the row entirely.
type FinalizedChild struct {
	Resolution string
	FoodID     int64
	Amount     float64
	Unit       string
	// PortionGrams is the grams per count unit the user confirmed in the
	// review ("1 piece ≈ 120 g"). nil keeps the leaf's portion, or seeds a
	// missing one from units.EstimateCountGrams. Ignored for mass and volume.
	PortionGrams *float64
}

// FinalizedInstruction is a reviewed cooking step ready to write.
type FinalizedInstruction struct {
	StepNumber int
	Text       string
}

// FinalizedFood is the JSON payload the importer returns after finalize. It is
// shaped to be POSTed directly to /api/foods (kind=composed).
type FinalizedFood struct {
	Name              string               `json:"name"`
	Kind              string               `json:"kind"`
	Role              string               `json:"role"`
	ReferencePortions float64              `json:"reference_portions"`
	PrepMinutes       *int                 `json:"prep_minutes"`
	CookMinutes       *int                 `json:"cook_minutes"`
	Notes             *string              `json:"notes"`
	Children          []FinalizedFoodChild `json:"children"`
	Instructions      []FinalizedFoodStep  `json:"instructions"`
	Tags              []string             `json:"tags"`
}

// FinalizedFoodChild mirrors one food_components row on the target food.
type FinalizedFoodChild struct {
	ChildID     int64   `json:"child_id"`
	Amount      float64 `json:"amount"`
	Unit        string  `json:"unit"`
	Grams       float64 `json:"grams"`
	GramsSource string  `json:"grams_source"`
	SortOrder   int     `json:"sort_order"`
}

// FinalizedFoodStep is one reviewed instruction step.
type FinalizedFoodStep struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

// Finalize validates the reviewed draft and returns a payload that POSTs
// directly to /api/foods. Its only writes are leaf portions: a count line
// (D19) resolves its grams from the leaf's portion for that unit, which is
// created from units.EstimateCountGrams (or the reviewed PortionGrams) when missing.
func (s *Service) Finalize(ctx context.Context, in FinalizeInput) (*FinalizedFood, error) {
	if in.Name == "" || in.Role == "" || in.ReferencePortions <= 0 {
		return nil, fmt.Errorf("%w: name, role, reference_portions required", domain.ErrInvalidInput)
	}

	out := &FinalizedFood{
		Name:              in.Name,
		Kind:              string(food.KindComposed),
		Role:              in.Role,
		ReferencePortions: in.ReferencePortions,
		PrepMinutes:       in.PrepMinutes,
		CookMinutes:       in.CookMinutes,
		Notes:             in.Notes,
		Tags:              append([]string{}, in.Tags...),
		Instructions:      make([]FinalizedFoodStep, 0, len(in.Instructions)),
		Children:          make([]FinalizedFoodChild, 0, len(in.Children)),
	}

	for _, ins := range in.Instructions {
		out.Instructions = append(out.Instructions, FinalizedFoodStep(ins))
	}

	sort := 0
	for _, row := range in.Children {
		switch row.Resolution {
		case ResolutionSkip:
			continue
		case ResolutionExisting, ResolutionNew:
			if row.FoodID == 0 {
				return nil, fmt.Errorf("%w: missing food_id for %s row", domain.ErrImportInvalidResolution, row.Resolution)
			}
			if row.Amount <= 0 {
				return nil, fmt.Errorf("%w: amount must be > 0", domain.ErrImportInvalidResolution)
			}
		default:
			return nil, fmt.Errorf("%w: unknown resolution %q", domain.ErrImportInvalidResolution, row.Resolution)
		}
		normalizedUnit := units.Normalize(row.Unit)
		if normalizedUnit == "" {
			return nil, fmt.Errorf("%w: unit required", domain.ErrImportInvalidResolution)
		}
		if units.IsCount(normalizedUnit) {
			if err := s.ensurePortion(ctx, row.FoodID, normalizedUnit, row.PortionGrams); err != nil {
				return nil, err
			}
		}
		grams, source, err := food.ResolveGrams(ctx, s.portions, row.FoodID, row.Amount, normalizedUnit, 0)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, FinalizedFoodChild{
			ChildID:     row.FoodID,
			Amount:      row.Amount,
			Unit:        normalizedUnit,
			Grams:       grams,
			GramsSource: source,
			SortOrder:   sort,
		})
		sort++
	}

	if len(out.Children) == 0 {
		return nil, fmt.Errorf("%w: at least one child food required", domain.ErrInvalidInput)
	}
	return out, nil
}

// ensurePortion makes sure leaf foodID has a portion for the count unit. An
// existing portion is kept unless the user reviewed a different weight;
// a missing one is created from reviewed grams or units.EstimateCountGrams.
func (s *Service) ensurePortion(ctx context.Context, foodID int64, unit string, reviewed *float64) error {
	if reviewed != nil && *reviewed <= 0 {
		return fmt.Errorf("%w: portion grams must be > 0", domain.ErrImportInvalidResolution)
	}
	ps, err := s.portions.ListPortions(ctx, foodID)
	if err != nil {
		return fmt.Errorf("list portions of food %d: %w", foodID, err)
	}
	for _, p := range ps {
		if units.Normalize(p.Unit) != unit {
			continue
		}
		if reviewed == nil || *reviewed == p.Grams {
			return nil
		}
		// Keep the stored unit spelling so the upsert replaces this row.
		return s.portions.UpsertPortion(ctx, &food.Portion{FoodID: foodID, Unit: p.Unit, Grams: *reviewed})
	}
	// Nothing reviewed and nothing stored: seed what the review step showed,
	// the estimate for this ingredient (a clove of garlic is not 120 g).
	var grams float64
	if reviewed != nil {
		grams = *reviewed
	} else {
		leaf, err := s.portions.Get(ctx, foodID)
		if err != nil {
			return fmt.Errorf("load food %d for its portion estimate: %w", foodID, err)
		}
		grams = units.EstimateCountGrams(unit, leaf.Name)
		if grams <= 0 {
			return fmt.Errorf("%w: %s needs a weight per %s", domain.ErrImportInvalidResolution, leaf.Name, unit)
		}
	}
	return s.portions.UpsertPortion(ctx, &food.Portion{FoodID: foodID, Unit: unit, Grams: grams})
}

func (s *Service) recipeToDraft(rec *RawRecipe, sourceURL, method string, warnings []string) *Draft {
	steps := make([]string, 0, len(rec.RecipeInstructions))
	for _, step := range rec.RecipeInstructions {
		if step = stripHTML(step); step != "" {
			steps = append(steps, step)
		}
	}
	d := &Draft{
		Name:                 cleanRecipeName(rec.Name),
		Description:          stripHTML(rec.Description),
		SourceURL:            sourceURL,
		Instructions:         dropClosingPleasantry(steps),
		Tags:                 cleanTags(rec.Keywords, rec.AuthorName),
		ExtractMethod:        method,
		ReferencePortions:    float64(rec.RecipeYieldNumber),
		Warnings:             append([]string{}, warnings...),
		SourceKcalPerServing: rec.SourceKcalPerServing,
	}
	if d.ReferencePortions == 0 {
		d.ReferencePortions = 1
	}
	if len(rec.Image) > 0 {
		d.ImageURL = rec.Image[0]
	}
	if rec.PrepMinutes > 0 {
		v := rec.PrepMinutes
		d.PrepMinutes = &v
	}
	if rec.CookMinutes > 0 {
		v := rec.CookMinutes
		d.CookMinutes = &v
	}
	if rec.TotalMinutes > 0 {
		v := rec.TotalMinutes
		d.TotalMinutes = &v
	}
	if d.PrepMinutes == nil && d.CookMinutes == nil && d.TotalMinutes == nil {
		if v := minutesFromSteps(d.Instructions); v > 0 {
			d.CookMinutes = &v
			d.Warnings = append(d.Warnings, WarningTimesFromSteps)
		}
	}

	d.Language = DetectLanguage(rec.RecipeIngredient)
	d.Ingredients = make([]DraftIngredient, 0, len(rec.RecipeIngredient))
	for _, line := range rec.RecipeIngredient {
		var parsed DraftIngredient
		if d.Language == "en" {
			parsed = ParseLineEN(line)
		} else {
			parsed = ParseLineDE(line)
		}
		parsed.DietClass = diet.Classify(parsed.Name)
		if parsed.Confidence == ConfidenceUnparsed || parsed.NeedsWeight || parsed.Warning != "" {
			slog.Debug("import: line needs review", "raw", line, "confidence", parsed.Confidence,
				"needs_weight", parsed.NeedsWeight, "warning", parsed.Warning)
		}
		d.Ingredients = append(d.Ingredients, parsed)
	}
	return d
}
