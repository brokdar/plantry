package finds

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Ranking constants (FIND-10, tunable).
const (
	PriorRating = 4.0
	PriorVotes  = 30.0
	MinVotes    = 10
	TierKitchen = 1.0  // test kitchen
	TierBlog    = 0.95 // vetted blog
	TierUser    = 0.8  // user-submitted
	// PenaltyFactor lowers a site's score per "looks wrong" (NEW-4).
	PenaltyFactor = 0.9
)

// testKitchens are editorial sites with their own test kitchen;
// userSites publish recipes users submit. Every other site is a vetted blog.
var (
	testKitchens = []string{"bbcgoodfood.com", "eatsmarter.de", "kuechengoetter.de", "einfachbacken.de", "bonappetit.com", "epicurious.com", "seriouseats.com", "cooking.nytimes.com", "essen-und-trinken.de"}
	userSites    = []string{"chefkoch.de", "allrecipes.com"}
)

// SiteTier is the source tier of a host (FIND-10).
func SiteTier(site string) float64 {
	switch {
	case siteIn(site, testKitchens):
		return TierKitchen
	case siteIn(site, userSites):
		return TierUser
	}
	return TierBlog
}

// Editorial sites keep a candidate under MinVotes (FIND-10).
func Editorial(site string) bool { return SiteTier(site) == TierKitchen }

func siteIn(site string, list []string) bool {
	for _, d := range list {
		if site == d || strings.HasSuffix(site, "."+d) {
			return true
		}
	}
	return false
}

// Candidate is one fetched version.
type Candidate struct {
	Source Source
	Draft  *importer.Draft
}

// Score is Bayesian rating × tier × completeness (FIND-10). Without a
// rating the prior stands.
func Score(rating *float64, votes int, tier, completeness float64) float64 {
	r := 0.0
	if rating != nil {
		r = *rating
	}
	n := float64(max(votes, 0))
	bayes := (PriorRating*PriorVotes + r*n) / (PriorVotes + n)
	return bayes * tier * completeness
}

// Completeness is the share of name, ingredients, steps, yield and time the
// draft has.
func Completeness(d *importer.Draft) float64 {
	have := 0
	for _, ok := range []bool{
		d.Name != "", len(d.Ingredients) > 0, len(d.Instructions) > 0, d.ReferencePortions > 0,
		d.TotalMinutes != nil || d.PrepMinutes != nil || d.CookMinutes != nil,
	} {
		if ok {
			have++
		}
	}
	return float64(have) / 5
}

// Rank scores the candidates, drops those under MinVotes unless editorial,
// lowers penalised sites, and orders best first (ties: URL).
func Rank(cands []Candidate, penalties map[string]int) []Candidate {
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Source.Votes < MinVotes && !Editorial(c.Source.Site) {
			continue
		}
		c.Source.Score = Score(c.Source.Rating, c.Source.Votes, SiteTier(c.Source.Site), Completeness(c.Draft)) *
			math.Pow(PenaltyFactor, float64(penalties[c.Source.Site]))
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b Candidate) int {
		if c := cmp.Compare(b.Source.Score, a.Source.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.Source.URL, b.Source.URL)
	})
	return out
}
