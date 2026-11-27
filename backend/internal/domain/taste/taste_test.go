package taste_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

var t0 = time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)

func learned(e taste.Evidence) taste.Item {
	return taste.Item{Level: taste.LevelHero, Key: "salmon", Scope: taste.ScopeAlways, Source: taste.SourceLearned, Evidence: e, UpdatedAt: t0}
}

// strongest are the largest weight and share a single TASTE-2 signal puts
// on one item: a plate signal blames at most HeroShare (one hero, no sides),
// a tip its whole weight on one flavour dimension, a Not for me with one
// named ingredient its whole weight on that hero.
var strongest = []struct {
	name     string
	w, share float64
}{
	{"loved plate", taste.WeightLoved * taste.HeroShare, taste.HeroShare},
	{"disliked plate", taste.WeightDisliked * taste.HeroShare, taste.HeroShare},
	{"favourite", taste.WeightFavourite * taste.HeroShare, taste.HeroShare},
	{"plate added", taste.WeightPlateAdded * taste.HeroShare, taste.HeroShare},
	{"plate removed", taste.WeightPlateRemoved * taste.HeroShare, taste.HeroShare},
	{"review removed", taste.WeightReviewRemoved * taste.HeroShare, taste.HeroShare},
	{"tip applied", taste.WeightTipApplied, 1},
	{"tip dismissed", taste.WeightTipDismissed, 1},
	{"migrated like", taste.WeightMigrated, 1},
	{"not for me, one ingredient", taste.WeightNotForMe, 1},
}

// AC-13 / TASTE-4: one signal never flips an item. From the weakest loved
// (or rather-not) state at any n, the strongest single signal against it
// does not reach the other side.
func TestOneSignalNeverFlips(t *testing.T) {
	for _, s := range strongest {
		for _, n := range []float64{0, 0.4, 1, 3, 10, 50} {
			if s.w < 0 {
				// The weakest loved item at n: score exactly Threshold.
				it := learned(taste.Evidence{Pos: taste.Threshold * (n + taste.Prior), PosN: n})
				assert.Equal(t, taste.Love, it.Effective(t0), "%s n=%v: starts loved", s.name, n)
				it.Evidence = it.Evidence.Add(s.w, s.share)
				assert.NotEqual(t, taste.RatherNot, it.Effective(t0), "%s n=%v: one signal flips a loved item", s.name, n)
			} else {
				it := learned(taste.Evidence{Neg: -taste.Threshold * (n + taste.Prior), NegN: n})
				assert.Equal(t, taste.RatherNot, it.Effective(t0), "%s n=%v: starts rather not", s.name, n)
				it.Evidence = it.Evidence.Add(s.w, s.share)
				assert.NotEqual(t, taste.Love, it.Effective(t0), "%s n=%v: one signal flips a disliked item", s.name, n)
			}
		}
	}
}

// AC-13: one disliked plate does not flip a liked item (the plate's main
// with one hero, the hero loved from three loved plates).
func TestOneDislikedPlateKeepsALikedItem(t *testing.T) {
	var e taste.Evidence
	for range 3 {
		e = e.Add(taste.WeightLoved*taste.HeroShare, taste.HeroShare)
	}
	it := learned(e)
	assert.Equal(t, taste.Love, it.Effective(t0))
	it.Evidence = it.Evidence.Add(taste.WeightDisliked*taste.HeroShare, taste.HeroShare)
	assert.NotEqual(t, taste.RatherNot, it.Effective(t0))
	assert.Greater(t, it.Learned(t0), 0.0, "still leans positive")
}

// No implicit plate signal makes an item lean on its own: one is never
// enough (TASTE-4), consistent ones are.
func TestImplicitSignalsAccumulate(t *testing.T) {
	for _, s := range strongest[:6] {
		it := learned(taste.Evidence{}.Add(s.w, s.share))
		assert.Equal(t, taste.Polarity(""), it.Effective(t0), "%s: one signal makes an item", s.name)
	}
	var e taste.Evidence
	for range 4 {
		e = e.Add(taste.WeightDisliked*taste.HeroShare, taste.HeroShare)
	}
	assert.Equal(t, taste.RatherNot, learned(e).Effective(t0), "four dislikes do")
}

// AC-13 / TASTE-5: a confirmed statement sets the item and outranks the
// evidence, flipping a loved item.
func TestStatementFlipsAndOutranks(t *testing.T) {
	var e taste.Evidence
	for range 10 {
		e = e.Add(taste.WeightLoved*taste.HeroShare, taste.HeroShare)
	}
	it := learned(e)
	assert.Equal(t, taste.Love, it.Effective(t0))
	it.Source, it.Polarity = taste.SourceStatement, taste.RatherNot
	assert.Equal(t, taste.RatherNot, it.Effective(t0))
	assert.InDelta(t, -1, it.Score(t0), 1e-9)
}

// AC-13 / TASTE-7: implicit evidence never makes a never item: however many
// dislikes, a learned item is at most "rather not".
func TestImplicitNeverImpossible(t *testing.T) {
	var e taste.Evidence
	for range 200 {
		e = e.Add(taste.WeightDisliked, 1)
	}
	it := learned(e)
	assert.Equal(t, taste.RatherNot, it.Effective(t0))
	assert.GreaterOrEqual(t, it.Score(t0), -1.0)
}

// TASTE-6: learned evidence fades (likes in about 4 months, dislikes in 8);
// statements do not fade.
func TestFading(t *testing.T) {
	month := 30 * 24 * time.Hour
	var like, dislike taste.Evidence
	for range 5 {
		like = like.Add(taste.WeightLoved*taste.HeroShare, taste.HeroShare)
		dislike = dislike.Add(-taste.WeightLoved*taste.HeroShare, taste.HeroShare)
	}
	l, d := learned(like), learned(dislike)
	assert.InDelta(t, like.Pos/2, like.Faded(4*month).Pos, 1e-9, "a like halves in 4 months")
	assert.InDelta(t, dislike.Neg/2, dislike.Faded(8*month).Neg, 1e-9, "a dislike halves in 8 months")
	later := t0.Add(6 * month)
	assert.Less(t, l.Learned(later), l.Learned(t0))
	assert.Greater(t, -d.Learned(later), l.Learned(later), "dislikes fade slower")
	assert.Equal(t, taste.Love, l.Effective(t0))
	assert.Equal(t, taste.Polarity(""), l.Effective(t0.Add(24*month)), "an old like fades to neutral")

	stated := l
	stated.Source, stated.Polarity = taste.SourceStatement, taste.Love
	assert.InDelta(t, 1, stated.Score(t0.Add(60*month)), 1e-9)
}

// TASTE-6: a window item ends with its window.
func TestWindowItemEndsWithWindow(t *testing.T) {
	w := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	it := taste.Item{Scope: taste.ScopeWindow, WindowStart: w, Source: taste.SourceStatement, Polarity: taste.RatherNot}
	assert.True(t, it.Active(w))
	assert.False(t, it.Active(w.AddDate(0, 0, 7)))
	assert.True(t, taste.Item{Scope: taste.ScopeAlways}.Active(w.AddDate(0, 0, 7)))
}

// TASTE-3: plate shares and recipe shares.
func TestBlame(t *testing.T) {
	main, side := taste.PlateShares(true, 2)
	assert.InDelta(t, 0.7, main, 1e-9)
	assert.InDelta(t, 0.15, side, 1e-9)
	main, _ = taste.PlateShares(true, 0)
	assert.InDelta(t, 1, main, 1e-9)
	_, side = taste.PlateShares(false, 2)
	assert.InDelta(t, 0.5, side, 1e-9)

	a := flavor.Attributes{Heroes: []string{"Salmon", "Dill"}, Cuisine: "french", Taste: &flavor.Taste{Acid: 2, Umami: 3}, Technique: "bake", Format: "plate"}
	got := map[taste.Ref]float64{}
	for _, b := range taste.Blame(a, 0.7) {
		got[b.Ref] = b.Share
	}
	assert.InDelta(t, 0.14, got[taste.Ref{Level: taste.LevelHero, Key: "salmon"}], 1e-9)
	assert.InDelta(t, 0.105, got[taste.Ref{Level: taste.LevelCuisine, Key: "french"}], 1e-9)
	assert.InDelta(t, 0.0525, got[taste.Ref{Level: taste.LevelFlavour, Key: "umami"}], 1e-9)
	assert.InDelta(t, 0.105, got[taste.Ref{Level: taste.LevelFormat, Key: "plate"}], 1e-9)
	var sum float64
	for _, s := range got {
		sum += s
	}
	assert.InDelta(t, 0.7, sum, 1e-9, "the whole share is blamed")

	neutral := taste.Blame(flavor.Attributes{Cuisine: "neutral"}, 1)
	assert.Empty(t, neutral, "the neutral cuisine is no taste")
}

// The planner profile counts only items that lean; a window item outranks
// the always item of its ref.
func TestProfile(t *testing.T) {
	w := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	weak := learned(taste.Evidence{}.Add(0.4, 0.4))
	weak.Key = "dill"
	thai := taste.Item{Level: taste.LevelCuisine, Key: "thai", Scope: taste.ScopeAlways, Source: taste.SourceStatement, Polarity: taste.Love}
	notThisWeek := taste.Item{Level: taste.LevelCuisine, Key: "thai", Scope: taste.ScopeWindow, WindowStart: w, Source: taste.SourceStatement, Polarity: taste.RatherNot}
	p := taste.NewProfile([]taste.Item{weak, notThisWeek, thai}, t0, w)
	a := flavor.Attributes{Heroes: []string{"Dill"}, Cuisine: "thai"}
	assert.InDelta(t, -taste.AttrShare, p.Recipe(a), 1e-9)
	next := taste.NewProfile([]taste.Item{weak, notThisWeek, thai}, t0, w.AddDate(0, 0, 7))
	assert.InDelta(t, taste.AttrShare, next.Recipe(a), 1e-9)
	assert.True(t, taste.NewProfile(nil, t0, w).Empty())
}
