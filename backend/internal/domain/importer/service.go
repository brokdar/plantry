package importer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
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
	matcher     Matcher // may be nil: drafts are then not matched
	portions    PortionStore
	recorder    *airun.Recorder // may be nil: the LLM fallback is then not recorded
	checker     *Checker        // may be nil: no import check
}

// NewService builds an import Service. llmResolver may be nil; when set, it
// is consulted per-request so configuration changes take effect without a
// restart.
func NewService(f Fetcher, j JSONLDExtractor, llmResolver llm.Resolver, r Resolver, portions PortionStore) *Service {
	return &Service{fetcher: f, jsonld: j, llmResolver: llmResolver, resolver: r, portions: portions}
}

// WithMatcher sets the matching path for drafts and ResolveLine.
func (s *Service) WithMatcher(m Matcher) *Service {
	s.matcher = m
	return s
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
	// Match runs the matcher over the lines (the import review); discovery
	// verification leaves it off and annotates locally.
	Match bool
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
		logStage(ctx, StageFetch, "host", host, "bytes", len(body), "redirected", final != "" && redirectedElsewhere(in.URL, final))
		if final != "" {
			sourceURL = final
			if redirectedElsewhere(in.URL, final) {
				warnings = append(warnings, WarningRedirectedOtherRecipe)
			}
		}
	}

	rec, err := s.jsonld.ExtractRecipe(htmlBody)
	if err == nil && len(rec.RecipeIngredient) > 0 {
		logStage(ctx, StageParse, "host", host, "outcome", "found", "ingredients", len(rec.RecipeIngredient))
		return s.draftOf(ctx, rec, sourceURL, "jsonld", warnings, host, in.Match), nil
	}
	outcome := "no_recipe"
	switch {
	case err != nil && !errors.Is(err, ErrNoRecipe):
		outcome = "error"
		slog.DebugContext(ctx, "import: json-ld extraction error", "host", host, "error", err)
	case err == nil:
		outcome = "no_ingredients"
	}
	logStage(ctx, StageParse, "host", host, "outcome", outcome, "error", errText(err), "fallback_allowed", !in.NoLLMFallback)

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
	return s.draftOf(ctx, rec, sourceURL, "llm", warnings, host, in.Match), nil
}

// draftOf is recipeToDraft plus the `resolve` stage line.
func (s *Service) draftOf(ctx context.Context, rec *RawRecipe, sourceURL, method string, warnings []string, host string, match bool) *Draft {
	d := s.recipeToDraft(ctx, rec, sourceURL, method, warnings)
	if match {
		s.matchDraft(ctx, d)
	}
	logStage(ctx, StageResolve, "host", host, "method", method, "ingredients", len(d.Ingredients),
		"instructions", len(d.Instructions), "warnings", len(d.Warnings))
	return d
}

// logStage logs one import stage with its outcome (the run's ids when the
// stage runs inside an import_fallback run).
func logStage(ctx context.Context, stage string, attrs ...any) {
	llm.Log(ctx, slog.LevelInfo, "import.stage", append([]any{"stage", stage}, attrs...)...)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	if s := err.Error(); len(s) > 200 {
		return s[:200] + "…"
	}
	return err.Error()
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

// ResolveLine runs one review row's search through the matching path, so a
// re-search finds what the draft would (MATCH-4); without a matcher it is
// the food resolver's lookup. The pick, if any, comes first.
func (s *Service) ResolveLine(ctx context.Context, query, lang string) ([]food.Candidate, error) {
	if s.matcher == nil {
		if clean, _ := CleanName(query); clean != "" {
			query = clean
		}
		results, _, err := s.resolver.Lookup(ctx, "", query, lang, 5)
		return results, err
	}
	rows := ParseLine(query, lang)
	res, err := s.matcher.Match(ctx, food.MatchRequest{Lang: lang, Lines: []food.MatchLine{MatchLineOf(rows[0])}, Requested: true})
	if err != nil {
		return nil, err
	}
	m := res.Lines[0]
	if m.Pick > 0 {
		m.Candidates[0], m.Candidates[m.Pick] = m.Candidates[m.Pick], m.Candidates[0]
	}
	return m.Candidates, nil
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

// ResolutionExisting / ResolutionNew classify a per-row decision. There is
// no skip (MATCH-5): a line is matched, or the review removes it.
const (
	ResolutionExisting = "existing"
	ResolutionNew      = "new"
)

// toTasteAmount is the nominal amount of a to-taste line (1 g): it keeps the
// positive-amount checks and counts in no sum.
const toTasteAmount = 1

// FinalizedChild is one resolved ingredient row: an existing food, or one the
// review created before (with nutrition per 100 g).
type FinalizedChild struct {
	Resolution string
	FoodID     int64
	// ToTaste: a to-taste seasoning line; Amount and Unit are ignored and
	// stored as the nominal 1 g.
	ToTaste bool
	Amount  float64
	Unit    string
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
	ToTaste     bool    `json:"to_taste"`
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
		if err := s.checkHonest(ctx, row); err != nil {
			return nil, err
		}
		if row.ToTaste {
			out.Children = append(out.Children, FinalizedFoodChild{
				ChildID: row.FoodID, Amount: toTasteAmount, Unit: "g", Grams: toTasteAmount,
				GramsSource: food.GramsSourceDirect, SortOrder: sort, ToTaste: true,
			})
			sort++
			continue
		}
		if row.Amount <= 0 {
			return nil, fmt.Errorf("%w: amount must be > 0", domain.ErrImportInvalidResolution)
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

// LineError names the food of the line a P3 refusal is about, so the
// transport can point at it. It unwraps to the domain error.
type LineError struct {
	Err  error
	Food string
}

func (e *LineError) Error() string { return fmt.Sprintf("%v: %s", e.Err, e.Food) }
func (e *LineError) Unwrap() error { return e.Err }

// checkHonest enforces P3 on one row: it names a food, the food is a leaf,
// and the leaf has nutrition per 100 g unless the line is to taste (only a
// seasoning may be, MATCH-6). /api/import/resolve is the P3 gate; POST
// /api/foods stays permissive for manual recipes.
func (s *Service) checkHonest(ctx context.Context, row FinalizedChild) error {
	switch row.Resolution {
	case ResolutionExisting, ResolutionNew:
	default:
		return fmt.Errorf("%w: resolution %q", domain.ErrImportUnmatchedLine, row.Resolution)
	}
	if row.FoodID == 0 {
		return fmt.Errorf("%w: missing food_id", domain.ErrImportUnmatchedLine)
	}
	f, err := s.portions.Get(ctx, row.FoodID)
	if err != nil {
		return fmt.Errorf("load food %d: %w", row.FoodID, err)
	}
	if f.Kind != food.KindLeaf {
		return &LineError{Err: domain.ErrImportNoNutrition, Food: f.Name}
	}
	// A leaf of a known non-seasoning group ("chicken") cannot be to taste;
	// an unknown name is let through, the line's own name proposed it.
	if g := food.LineGroup(f.Name, ""); row.ToTaste && g != "" && !food.IsToTasteGroup(g) {
		return &LineError{Err: domain.ErrImportInvalidResolution, Food: f.Name}
	}
	if !row.ToTaste && (f.Kcal100g == nil || f.Protein100g == nil || f.Fat100g == nil || f.Carbs100g == nil) {
		return &LineError{Err: domain.ErrImportNoNutrition, Food: f.Name}
	}
	return nil
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

func (s *Service) recipeToDraft(ctx context.Context, rec *RawRecipe, sourceURL, method string, warnings []string) *Draft {
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
	d.Ingredients = ParseLines(rec.RecipeIngredient, d.Language)
	for _, parsed := range d.Ingredients {
		if parsed.Confidence == ConfidenceUnparsed || parsed.NeedsWeight || parsed.Warning != "" {
			slog.DebugContext(ctx, "import: line needs review", "raw", parsed.RawText, "confidence", parsed.Confidence,
				"needs_weight", parsed.NeedsWeight, "warning", parsed.Warning)
		}
	}
	return d
}
