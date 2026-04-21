package importer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
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

// Resolver resolves an ingredient name against the local catalogue and external sources.
// Implemented by domain/ingredient.Resolver.
type Resolver interface {
	Lookup(ctx context.Context, barcode, query, lang string, limit int) ([]ingredient.Candidate, int, error)
}

// Service orchestrates recipe import: fetch → JSON-LD → LLM fallback → parse lines.
type Service struct {
	fetcher     Fetcher
	jsonld      JSONLDExtractor
	llmResolver llm.Resolver // may be nil — LLM fallback then returns ErrAIProviderMissing.
	resolver    Resolver
}

// NewService builds an import Service. llmResolver may be nil; when set, it
// is consulted per-request so configuration changes take effect without a
// restart.
func NewService(f Fetcher, j JSONLDExtractor, llmResolver llm.Resolver, r Resolver) *Service {
	return &Service{fetcher: f, jsonld: j, llmResolver: llmResolver, resolver: r}
}

// ExtractInput selects the source for extraction. Exactly one of URL or HTML
// must be non-empty.
type ExtractInput struct {
	URL  string
	HTML string
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
	if htmlBody == "" {
		body, final, err := s.fetcher.Fetch(ctx, in.URL)
		if err != nil {
			return nil, err
		}
		htmlBody = body
		if final != "" {
			sourceURL = final
		}
	}

	// First-choice: JSON-LD extraction.
	rec, err := s.jsonld.ExtractRecipe(htmlBody)
	if err == nil && len(rec.RecipeIngredient) > 0 {
		return s.recipeToDraft(rec, sourceURL, "jsonld"), nil
	}

	// Fallback: LLM extraction.
	if s.llmResolver == nil {
		if errors.Is(err, ErrNoRecipe) {
			return nil, domain.ErrImportNoRecipe
		}
		return nil, domain.ErrAIProviderMissing
	}

	rec, llmErr := s.llmExtract(ctx, htmlBody)
	if llmErr != nil {
		return nil, llmErr
	}
	if rec == nil || len(rec.RecipeIngredient) == 0 {
		return nil, domain.ErrImportNoRecipe
	}
	return s.recipeToDraft(rec, sourceURL, "llm"), nil
}

// ResolveLine is a thin wrapper around ingredient.Resolver used from the
// per-row lookup endpoint in the review step. The recommended-index return
// value is not exposed to the importer review UI.
func (s *Service) ResolveLine(ctx context.Context, query, lang string) ([]ingredient.Candidate, error) {
	results, _, err := s.resolver.Lookup(ctx, "", query, lang, 5)
	return results, err
}

// FinalizeInput is what the review step submits after the user has resolved
// every ingredient line.
type FinalizeInput struct {
	Name              string
	Role              string
	ReferencePortions float64
	PrepMinutes       *int
	CookMinutes       *int
	Notes             *string
	Tags              []string
	Instructions      []FinalizedInstruction
	Ingredients       []FinalizedIngredient
}

// ResolutionExisting / ResolutionSkip / ResolutionNew classify a per-row decision.
const (
	ResolutionExisting = "existing"
	ResolutionSkip     = "skip"
	ResolutionNew      = "new"
)

type FinalizedIngredient struct {
	Resolution   string
	IngredientID int64
	Amount       float64
	Unit         string
}

type FinalizedInstruction struct {
	StepNumber int
	Text       string
}

// FinalizedComponent mirrors the shape of componentRequest in handlers/components.go
// so callers can POST it directly to /api/components without reshaping.
type FinalizedComponent struct {
	Name              string                         `json:"name"`
	Role              string                         `json:"role"`
	ReferencePortions float64                        `json:"reference_portions"`
	PrepMinutes       *int                           `json:"prep_minutes"`
	CookMinutes       *int                           `json:"cook_minutes"`
	Notes             *string                        `json:"notes"`
	Ingredients       []FinalizedComponentIngredient `json:"ingredients"`
	Instructions      []FinalizedComponentStep       `json:"instructions"`
	Tags              []string                       `json:"tags"`
}

type FinalizedComponentIngredient struct {
	IngredientID int64   `json:"ingredient_id"`
	Amount       float64 `json:"amount"`
	Unit         string  `json:"unit"`
	Grams        float64 `json:"grams"`
	SortOrder    int     `json:"sort_order"`
}

type FinalizedComponentStep struct {
	StepNumber int    `json:"step_number"`
	Text       string `json:"text"`
}

// Finalize validates the reviewed draft and returns a payload shaped like
// componentRequest. No DB writes happen here; persistence is the caller's job
// via POST /api/components.
func (s *Service) Finalize(_ context.Context, in FinalizeInput) (*FinalizedComponent, error) {
	if in.Name == "" || in.Role == "" || in.ReferencePortions <= 0 {
		return nil, fmt.Errorf("%w: name, role, reference_portions required", domain.ErrInvalidInput)
	}

	out := &FinalizedComponent{
		Name:              in.Name,
		Role:              in.Role,
		ReferencePortions: in.ReferencePortions,
		PrepMinutes:       in.PrepMinutes,
		CookMinutes:       in.CookMinutes,
		Notes:             in.Notes,
		Tags:              append([]string{}, in.Tags...),
		Instructions:      make([]FinalizedComponentStep, 0, len(in.Instructions)),
		Ingredients:       make([]FinalizedComponentIngredient, 0, len(in.Ingredients)),
	}

	for _, ins := range in.Instructions {
		out.Instructions = append(out.Instructions, FinalizedComponentStep(ins))
	}

	sort := 0
	for _, row := range in.Ingredients {
		switch row.Resolution {
		case ResolutionSkip:
			continue
		case ResolutionExisting, ResolutionNew:
			if row.IngredientID == 0 {
				return nil, fmt.Errorf("%w: missing ingredient_id for %s row", domain.ErrImportInvalidResolution, row.Resolution)
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
		// Seed Grams via the universal default when possible so the review UI
		// can show a reasonable total; the component service re-resolves this
		// canonically at POST time (honoring ingredient-specific portions).
		var grams float64
		if def, ok := units.LookupDefault(normalizedUnit); ok {
			grams = row.Amount * def.Grams
		}
		out.Ingredients = append(out.Ingredients, FinalizedComponentIngredient{
			IngredientID: row.IngredientID,
			Amount:       row.Amount,
			Unit:         normalizedUnit,
			Grams:        grams,
			SortOrder:    sort,
		})
		sort++
	}

	if len(out.Ingredients) == 0 {
		return nil, fmt.Errorf("%w: at least one ingredient required", domain.ErrInvalidInput)
	}
	return out, nil
}

func (s *Service) recipeToDraft(rec *RawRecipe, sourceURL, method string) *Draft {
	d := &Draft{
		Name:              rec.Name,
		Description:       rec.Description,
		SourceURL:         sourceURL,
		Instructions:      append([]string{}, rec.RecipeInstructions...),
		Tags:              append([]string{}, rec.Keywords...),
		ExtractMethod:     method,
		ReferencePortions: float64(rec.RecipeYieldNumber),
		Warnings:          []string{},
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

	d.Language = DetectLanguage(rec.RecipeIngredient)
	d.Ingredients = make([]DraftIngredient, 0, len(rec.RecipeIngredient))
	for _, line := range rec.RecipeIngredient {
		var parsed DraftIngredient
		if d.Language == "en" {
			parsed = ParseLineEN(line)
		} else {
			parsed = ParseLineDE(line)
		}
		d.Ingredients = append(d.Ingredients, parsed)
	}
	return d
}
