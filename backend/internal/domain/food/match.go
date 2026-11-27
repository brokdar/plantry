package food

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// Match statuses of one ingredient line (spec MATCH-2).
const (
	// MatchMatched: a confident match.
	MatchMatched = "matched"
	// MatchClosest: an immaterial seasoning line uses the closest food of the
	// same kind (research section 3.2 "Materiality").
	MatchClosest = "closest"
	// MatchUnmatched: no confident match; Reason says why, Tried what ran.
	MatchUnmatched = "unmatched"
)

// Reasons a line stays unmatched.
const (
	ReasonNoCandidate = "no_candidate"
	ReasonState       = "state_mismatch"
	ReasonKcal        = "implausible_kcal"
	ReasonGrams       = "no_grams"
	ReasonNutrition   = "no_nutrition"
)

const (
	matchSearchLimit   = 5
	matchMaxCandidates = 12
	matchParallel      = 4
	// immaterialGrams and immaterialShare bound a seasoning line that may use
	// the closest food: under 5 g and under 2 % of the recipe's kcal.
	immaterialGrams = 5.0
	immaterialShare = 0.02
)

// MatchLine is one cleaned ingredient line (MATCH-1 done by the importer).
type MatchLine struct {
	Name         string
	Alternatives []string
	Note         string
	Amount       float64
	Unit         string
	// PortionGrams is the importer's grams per count unit, nil when unknown.
	PortionGrams *float64
	// ToTaste: the line has no amount ("Salz", "nach Geschmack") and needs none.
	ToTaste bool
}

// MatchRequest is one recipe's lines.
type MatchRequest struct {
	Lang  string
	Lines []MatchLine
	// Requested is work the user asked for (an import): the cap guard warns
	// at the cap instead of refusing.
	Requested bool
	// LocalOnly matches against the catalogue only: no FDC, OFF or LLM.
	LocalOnly bool
	// Ref names the recipe in the run scope (its source URL).
	Ref string
}

// LineMatch is the outcome for one line.
type LineMatch struct {
	Status     string      `json:"status"`
	Reason     string      `json:"reason,omitempty"`
	Tried      []string    `json:"tried"`
	Candidates []Candidate `json:"candidates"`
	// Pick indexes Candidates; -1 when unmatched.
	Pick int `json:"pick"`
	// Grams of the whole line, nil for a to-taste line or when unknown.
	Grams *float64 `json:"grams"`
	// PortionGrams is the grams per count unit the match used, if any.
	PortionGrams *float64 `json:"portion_grams,omitempty"`
	// Name is the line name or the alternative that matched.
	Name string `json:"name"`

	entry   *Synonym
	state   string
	toTaste bool
	// best is the ladder's confident candidate index, -1 when none.
	best int
}

// MatchResult is a recipe's lines in order.
type MatchResult struct {
	Lines []LineMatch
	// CapWarning: the month's Chef cap is reached; the requested match ran.
	CapWarning bool
}

// MatchGuard is the Chef cap guard for one chef_match run: run reports
// whether the call may run, warn that it runs at the cap.
type MatchGuard func(ctx context.Context, requested bool) (run, warn bool, err error)

// LangSearcher searches Open Food Facts by name in a language.
type LangSearcher interface {
	SearchByNameLang(ctx context.Context, query, lang string, limit int) ([]Candidate, error)
}

// Matcher is the one matching path for imports, Discover and later Chef
// finds (MATCH-4): clean lines in, catalogue -> FDC -> OFF, confidence
// checks, and with a provider one chef_match call per recipe that picks
// among the fetched candidates.
type Matcher struct {
	repo  Repository
	fdc   FoodProvider // may be nil
	off   LangSearcher // may be nil
	llm   llm.Resolver
	runs  *airun.Recorder
	guard MatchGuard
}

// NewMatcher builds a Matcher without a provider.
func NewMatcher(repo Repository, fdc FoodProvider, off LangSearcher) *Matcher {
	return &Matcher{repo: repo, fdc: fdc, off: off}
}

// WithPicker enables the chef_match call: resolver, run log and cap guard.
func (m *Matcher) WithPicker(resolver llm.Resolver, runs *airun.Recorder, guard MatchGuard) *Matcher {
	m.llm, m.runs, m.guard = resolver, runs, guard
	return m
}

// Match matches every line of one recipe.
func (m *Matcher) Match(ctx context.Context, req MatchRequest) (*MatchResult, error) {
	lines, err := m.Gather(ctx, req)
	if err != nil {
		return nil, err
	}
	return m.Decide(ctx, req, lines)
}

// Gather runs the ladder for every line: candidates and the confident
// candidate without a provider.
func (m *Matcher) Gather(ctx context.Context, req MatchRequest) ([]LineMatch, error) {
	leaves, err := listLeavesOf(ctx, m.repo)
	if err != nil {
		return nil, err
	}
	out := make([]LineMatch, len(req.Lines))
	sem := make(chan struct{}, matchParallel)
	var wg sync.WaitGroup
	for i, l := range req.Lines {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			out[i] = m.gatherLine(ctx, req, leaves, l)
		}()
	}
	wg.Wait()
	return out, nil
}

// Decide settles each line: the provider's pick when it is confident, else
// the ladder's; then the materiality rule for seasoning lines.
func (m *Matcher) Decide(ctx context.Context, req MatchRequest, lines []LineMatch) (*MatchResult, error) {
	res := &MatchResult{Lines: lines}
	for i := range lines {
		lines[i].Pick = lines[i].best
	}
	if !req.LocalOnly && m.guard != nil && m.llm != nil && hasCandidates(lines) {
		run, warn, err := m.guard(ctx, req.Requested)
		if err != nil {
			return nil, fmt.Errorf("match: cap guard: %w", err)
		}
		if run {
			res.CapWarning = warn
			for i, p := range m.pick(ctx, req, lines) {
				if p >= 0 && p < len(lines[i].Candidates) && lines[i].check(lines[i].Candidates[p]) == "" {
					lines[i].Pick = p
				}
			}
		}
	}
	recipeKcal := 0.0
	for i := range lines {
		l := &lines[i]
		if l.Pick >= 0 {
			l.Status, l.Reason = MatchMatched, ""
			if c := l.Candidates[l.Pick]; c.Kcal100g != nil && l.Grams != nil {
				recipeKcal += *c.Kcal100g * *l.Grams / 100
			}
		}
	}
	for i := range lines {
		l := &lines[i]
		if l.Pick < 0 {
			l.closest(recipeKcal)
		}
	}
	return res, nil
}

func hasCandidates(lines []LineMatch) bool {
	for _, l := range lines {
		if len(l.Candidates) > 0 {
			return true
		}
	}
	return false
}

// gatherLine runs the ladder for one line: the name first, then each
// alternative, until one is confident. Candidates of every name tried are
// kept for the review and the provider.
func (m *Matcher) gatherLine(ctx context.Context, req MatchRequest, leaves []catalogueLeaf, l MatchLine) LineMatch {
	names := append([]string{l.Name}, l.Alternatives...)
	var all []LineMatch
	for _, name := range names {
		lm := m.gatherName(ctx, req, leaves, l, name)
		all = append(all, lm)
		if lm.best >= 0 {
			break
		}
	}
	out := all[len(all)-1]
	if out.best < 0 {
		out = all[0]
	}
	tried, cands := []string{}, []Candidate{}
	if out.best >= 0 {
		cands = append(cands, out.Candidates...)
	}
	for _, lm := range all {
		tried = append(tried, lm.Tried...)
		if lm.best < 0 {
			cands = append(cands, lm.Candidates...)
		}
	}
	out.Tried = tried
	out.Candidates = dedupeCandidates(cands)
	if len(out.Candidates) > matchMaxCandidates {
		out.Candidates = out.Candidates[:matchMaxCandidates]
	}
	return out
}

// gatherName runs catalogue -> FDC -> OFF for one name of a line.
func (m *Matcher) gatherName(ctx context.Context, req MatchRequest, leaves []catalogueLeaf, l MatchLine, name string) LineMatch {
	keys := lineKeys(name, l.Note)
	e := synonyms.lookup(keys)
	if e == nil && len(keys) > 0 {
		e = synonyms.suffix(keys[0])
	}
	lm := LineMatch{Name: name, entry: e, best: -1, Pick: -1, Tried: []string{}, Candidates: []Candidate{}, toTaste: l.ToTaste}
	lm.state = lineState(e, name, l)
	lm.Grams, lm.PortionGrams = lineGrams(l, e)
	reason := ReasonNoCandidate

	// 1. Catalogue: the entry's names, then the exact key; prefix and
	// contains matches are candidates only.
	lm.Tried = append(lm.Tried, "catalogue")
	var confident []Candidate
	for _, lf := range leaves {
		if catalogueIdentity(lf, e, keys) {
			confident = append(confident, localCandidate(lf.food))
		}
	}
	if len(keys) > 0 {
		fuzzy, _ := localCandidates(leaves, keys[0])
		confident = append(confident, fuzzy...)
	}
	lm.Candidates = dedupeCandidates(confident)
	for i, c := range lm.Candidates {
		if i >= len(confident) || !catalogueIdentityName(c.Name, e, keys) {
			continue
		}
		if r := lm.check(c); r == "" {
			lm.best = i
			return lm
		} else if reason == ReasonNoCandidate {
			reason = r
		}
	}
	if req.LocalOnly {
		lm.Reason = reason
		return lm
	}

	// 2. FDC with the entry's English term, or the name of an English line.
	if q := fdcQuery(e, keys, req.Lang); q != "" && m.fdcUsable(ctx) {
		lm.Tried = append(lm.Tried, "fdc:"+q)
		cs, err := m.fdc.SearchByName(ctx, q, matchSearchLimit)
		if err != nil {
			slog.WarnContext(ctx, "match: fdc search failed", "query", q, "error", err)
			lm.Tried[len(lm.Tried)-1] += " (failed)"
		}
		fillMissingKcal(cs)
		enrichExistingIDs(leaves, cs)
		if i, r := lm.add(rankExternal(cs, q, preferOf(e), lm.state)); i >= 0 {
			lm.best = i
			return lm
		} else if r != "" && reason == ReasonNoCandidate {
			reason = r
		}
	}

	// 3. Open Food Facts for branded, regional and composite products.
	if m.off != nil {
		q := name
		if e != nil && e.OFF != "" {
			q = e.OFF
		}
		lang := req.Lang
		if lang != "de" {
			lang = "en"
		}
		lm.Tried = append(lm.Tried, "off:"+lang+":"+q)
		cs, err := m.off.SearchByNameLang(ctx, q, lang, matchSearchLimit)
		if err != nil {
			slog.WarnContext(ctx, "match: off search failed", "query", q, "error", err)
			lm.Tried[len(lm.Tried)-1] += " (failed)"
		}
		fillMissingKcal(cs)
		if i, r := lm.add(rankExternal(cs, matchKey(q), "", lm.state)); i >= 0 {
			lm.best = i
			return lm
		} else if r != "" && reason == ReasonNoCandidate {
			reason = r
		}
	}
	lm.Reason = reason
	return lm
}

// add appends ranked external candidates; the first identity-passing one
// that is confident is returned with its index, else the first reason.
func (lm *LineMatch) add(ranked []rankedCandidate) (int, string) {
	reason := ""
	best := -1
	for _, rc := range ranked {
		lm.Candidates = append(lm.Candidates, rc.c)
		if !rc.identity || best >= 0 {
			continue
		}
		if r := lm.check(rc.c); r == "" {
			best = len(lm.Candidates) - 1
		} else if reason == "" {
			reason = r
		}
	}
	return best, reason
}

func (m *Matcher) fdcUsable(ctx context.Context) bool {
	if m.fdc == nil {
		return false
	}
	if c, ok := m.fdc.(configurable); ok && !c.Configured(ctx) {
		return false
	}
	return true
}

// check applies MATCH-2 to one candidate: nutrition, same state, plausible
// kcal for the group, and an amount in grams. "" means confident.
func (lm *LineMatch) check(c Candidate) string {
	if c.Kcal100g == nil {
		return ReasonNutrition
	}
	if statesConflict(lm.state, stateOf(c.Name+" "+c.SourceName)) {
		return ReasonState
	}
	if lm.entry != nil {
		if band, ok := kcalBands[lm.entry.Group]; ok && (*c.Kcal100g < band[0] || *c.Kcal100g > band[1]) {
			return ReasonKcal
		}
	}
	if lm.Grams == nil && !lm.toTaste {
		return ReasonGrams
	}
	return ""
}

// closest applies the materiality rule to an unmatched seasoning line: under
// 5 g and under 2 % of the recipe's kcal it takes the first candidate of the
// same group with plausible nutrition.
func (lm *LineMatch) closest(recipeKcal float64) {
	lm.Status = MatchUnmatched
	if lm.entry == nil || !seasoningGroups[lm.entry.Group] {
		return
	}
	grams := 0.0
	if lm.Grams != nil {
		grams = *lm.Grams
	} else if !lm.toTaste {
		return
	}
	if grams >= immaterialGrams {
		return
	}
	band := kcalBands[lm.entry.Group]
	for i, c := range lm.Candidates {
		if c.Kcal100g == nil || *c.Kcal100g < band[0] || *c.Kcal100g > band[1] {
			continue
		}
		if recipeKcal > 0 && *c.Kcal100g*grams/100 >= immaterialShare*recipeKcal {
			continue
		}
		lm.Status, lm.Reason, lm.Pick = MatchClosest, "", i
		return
	}
}

// lineKeys lists the normalized names to look up, most specific first: the
// name with each note word before and after it ("rote Paprikaschote"),
// the name, its "/" parts, then the name without leading words.
func lineKeys(name, note string) []string {
	base := matchKey(name)
	if base == "" {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, w := range strings.Fields(matchKey(note)) {
		if len(w) < 3 {
			continue
		}
		add(w + " " + base)
		add(base + " " + w)
	}
	add(base)
	for _, part := range strings.Split(name, "/") {
		add(matchKey(part))
	}
	words := strings.Fields(base)
	for i := 1; i < len(words); i++ {
		add(strings.Join(words[i:], " "))
	}
	for i := len(words) - 1; i > 0; i-- {
		add(strings.Join(words[:i], " "))
	}
	return keys
}

// lineState is the state the line asks for: a state word in the line, a
// can, else the entry's default.
func lineState(e *Synonym, name string, l MatchLine) string {
	if s := stateOf(name + " " + l.Note); s != "" {
		return s
	}
	if u := units.Normalize(l.Unit); u == "can" {
		return StateCanned
	}
	if e != nil {
		return e.State
	}
	return ""
}

// lineGrams converts the line's amount to grams: mass directly, volume by
// the entry's density (metric volume of an unknown food as water), count by
// the entry's weight per piece or the importer's estimate.
func lineGrams(l MatchLine, e *Synonym) (grams, perUnit *float64) {
	if l.Amount <= 0 {
		return nil, nil
	}
	u := units.Normalize(l.Unit)
	if def, ok := units.LookupDefault(u); ok {
		switch def.Kind {
		case units.KindMass:
			g := l.Amount * def.Grams
			return &g, nil
		case units.KindVolume:
			d := 0.0
			switch {
			case e != nil && e.Density > 0:
				d = e.Density
			case e != nil:
				d = groupDensity[e.Group]
				if d == 0 {
					d = 1
				}
			case u == "ml" || u == "l" || u == "cl" || u == "dl":
				d = 1
			}
			if d == 0 {
				return nil, nil
			}
			g := l.Amount * def.Grams * d
			return &g, nil
		}
	}
	per := 0.0
	switch {
	case u == "piece" && e != nil && e.PieceG > 0:
		per = e.PieceG
	case l.PortionGrams != nil && *l.PortionGrams > 0:
		per = *l.PortionGrams
	case e != nil && e.PieceG > 0:
		per = e.PieceG
	}
	if per == 0 {
		return nil, nil
	}
	g := l.Amount * per
	return &g, &per
}

// catalogueIdentity: the leaf is the line's food, by the entry's names or
// the exact key.
func catalogueIdentity(lf catalogueLeaf, e *Synonym, keys []string) bool {
	return catalogueIdentityName(lf.food.Name, e, keys)
}

func catalogueIdentityName(leafName string, e *Synonym, keys []string) bool {
	full, base := matchKey(leafName), baseKey(leafName)
	if e != nil {
		_, a := e.names[full]
		_, b := e.names[base]
		if a || b {
			return true
		}
		// A leaf another entry owns is a different food.
		if o := synonyms.byName[base]; o != nil && o != e {
			return false
		}
	}
	return len(keys) > 0 && (full == keys[0] || (e == nil && base == keys[0]))
}

func fdcQuery(e *Synonym, keys []string, lang string) string {
	switch {
	case e != nil:
		return e.FDC
	case lang == "en" && len(keys) > 0:
		return keys[0]
	}
	return ""
}

func preferOf(e *Synonym) string {
	if e == nil {
		return ""
	}
	return e.FDCPrefer
}

type rankedCandidate struct {
	c        Candidate
	identity bool
	score    float64
}

// externalPenalty marks descriptions that are rarely the generic food.
var externalPenalty = []string{"industrial", "babyfood", "fast foods", "restaurant", "snack", "kit", "pudding", "sandwich"}

// rankExternal orders FDC or OFF results: those naming every query word (or
// the preferred fragment) first, then generic over branded, the line's
// state, fewer extra words.
func rankExternal(cs []Candidate, query, prefer, state string) []rankedCandidate {
	qWords := strings.Fields(matchKey(query))
	out := make([]rankedCandidate, 0, len(cs))
	for _, c := range cs {
		desc := c.SourceName
		if desc == "" {
			desc = c.Name
		}
		dk := " " + matchKey(desc) + " "
		rc := rankedCandidate{c: c, identity: true}
		for _, w := range qWords {
			if _, isState := stateWords[w]; isState {
				continue
			}
			if !strings.Contains(dk, " "+strings.TrimSuffix(w, "s")) {
				rc.identity = false
			}
		}
		if prefer != "" && strings.Contains(dk, matchKey(prefer)) {
			rc.identity = true
			rc.score += 10
		}
		if brandCaps(desc) {
			rc.score -= 5
		}
		for _, p := range externalPenalty {
			if strings.Contains(dk, " "+p) {
				rc.score -= 3
			}
		}
		if s := stateOf(desc); s != "" && s == state {
			rc.score += 2
		} else if statesConflict(s, state) {
			rc.score -= 4
		}
		extra := len(strings.Fields(dk)) - len(qWords)
		rc.score -= 0.5 * math.Max(0, float64(extra))
		if c.Source == SourceFDC && c.SourceName == "" {
			c.SourceName = c.Name
			rc.c = c
		}
		out = append(out, rc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].identity != out[j].identity {
			return out[i].identity
		}
		return out[i].score > out[j].score
	})
	return out
}

// brandCaps reports a word of three or more capital letters ("ON THE
// BORDER", "McDONALD'S"), the mark of a branded FDC entry.
func brandCaps(s string) bool {
	for _, w := range strings.Fields(s) {
		n := 0
		for _, r := range w {
			if unicode.IsUpper(r) {
				n++
			}
		}
		if n >= 3 {
			return true
		}
	}
	return false
}
