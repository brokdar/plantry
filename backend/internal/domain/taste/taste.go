// Package taste is the household's taste model (spec §5.11, TASTE-1..7, 9,
// 10): append-only signals blamed onto recipe attributes, taste items with
// shrunk and fading evidence, confirmed statements that outrank what was
// learned, and the score the planner reads. Never items are not taste items:
// they live in the kitchen's never_items and only the user makes them.
package taste

import (
	"math"
	"slices"
	"strings"
	"time"
)

// Level is the attribute level a taste item is about (TASTE-1).
type Level string

// Levels.
const (
	LevelHero      Level = "hero"
	LevelCuisine   Level = "cuisine"
	LevelFlavour   Level = "flavour"
	LevelTechnique Level = "technique"
	LevelFormat    Level = "format"
)

// Valid reports whether l is a known level.
func (l Level) Valid() bool {
	switch l {
	case LevelHero, LevelCuisine, LevelFlavour, LevelTechnique, LevelFormat:
		return true
	}
	return false
}

// Polarity is which way an item leans. Never is not a polarity: never items
// live in the kitchen (TASTE-7).
type Polarity string

// Polarities.
const (
	Love      Polarity = "love"
	RatherNot Polarity = "rather_not"
)

// Valid reports whether p is love or rather not.
func (p Polarity) Valid() bool { return p == Love || p == RatherNot }

// Scope is how long an item holds: always, or for one plan window.
type Scope string

// Scopes.
const (
	ScopeAlways Scope = "always"
	ScopeWindow Scope = "window"
)

// Source is where an item's polarity comes from.
type Source string

// Sources, weakest first. A statement or a user edit sets the polarity
// directly and outranks learned evidence (TASTE-5).
const (
	SourceLearned   Source = "learned"
	SourceStatement Source = "statement"
	SourceUser      Source = "user"
)

// SignalKind is the event a signal came from (TASTE-2).
type SignalKind string

// Signal kinds.
const (
	SignalLoved        SignalKind = "loved"
	SignalDisliked     SignalKind = "disliked"
	SignalFavourite    SignalKind = "favourite"
	SignalPlateAdded   SignalKind = "plate_added"
	SignalPlateRemoved SignalKind = "plate_removed"
	SignalReviewSwap   SignalKind = "review_swap"
	SignalNotForMe     SignalKind = "not_for_me"
	SignalTipApplied   SignalKind = "tip_applied"
	SignalTipDismissed SignalKind = "tip_dismissed"
	SignalMigrated     SignalKind = "migrated"
)

// Signal weights (TASTE-2; tunable starting values). Evidence units, spread
// over attributes by Blame.
const (
	WeightLoved         = 1.0
	WeightDisliked      = -1.5
	WeightFavourite     = 1.5
	WeightPlateAdded    = 0.6
	WeightPlateRemoved  = -0.3
	WeightReviewRemoved = -0.5
	WeightReviewAdded   = 0.3
	WeightNotForMe      = -1.0 // reason taste or ingredients
	WeightNotForMeOther = -0.3 // no reason
	WeightTipApplied    = 0.3
	WeightTipDismissed  = -0.2
	// WeightMigrated is a like or dislike tag of the old profile: weak
	// evidence (TASTE-10), below the learned threshold on its own.
	WeightMigrated = 0.5
)

// Scoring constants (TASTE-4, TASTE-6; tunable).
const (
	// Prior is the shrinkage: score = evidence / (n + Prior).
	Prior = 3.0
	// LikeHalfLife and DislikeHalfLife are how fast learned evidence fades.
	LikeHalfLife    = 120 * 24 * time.Hour // about 4 months
	DislikeHalfLife = 240 * 24 * time.Hour // about 8 months
	// Threshold is the learned score from which an item counts as loved
	// (or, negated, as rather not). With Prior 3 no single signal of
	// TASTE-2 reaches it alone, and no single signal takes an item from
	// one side to the other (taste_test.go).
	Threshold = 0.2
)

// StapleRule is when a recipe becomes a staple (THEME-1): at least Sessions
// cook sessions in the last Weeks, a positive signal, and no dislike in the
// last DislikeFreeWeeks. The favourite star pins a staple on its own.
type StapleRule struct{ Sessions, Weeks, DislikeFreeWeeks int }

// Staples is the rule in force (tunable).
var Staples = StapleRule{Sessions: 3, Weeks: 16, DislikeFreeWeeks: 8}

// Evidence is an item's learned evidence: likes and dislikes apart, since
// they fade at different speeds. N counts signals fractionally (TASTE-3).
type Evidence struct {
	Pos, PosN float64
	Neg, NegN float64
}

// Add adds a signal of weight w that counts share towards n.
func (e Evidence) Add(w, share float64) Evidence {
	switch {
	case w > 0:
		e.Pos += w
		e.PosN += share
	case w < 0:
		e.Neg += w
		e.NegN += share
	}
	return e
}

// Faded is e after elapsed time: likes halve every LikeHalfLife, dislikes
// every DislikeHalfLife. A negative elapsed time changes nothing.
func (e Evidence) Faded(elapsed time.Duration) Evidence {
	if elapsed <= 0 {
		return e
	}
	lf := math.Exp2(-float64(elapsed) / float64(LikeHalfLife))
	df := math.Exp2(-float64(elapsed) / float64(DislikeHalfLife))
	return Evidence{Pos: e.Pos * lf, PosN: e.PosN * lf, Neg: e.Neg * df, NegN: e.NegN * df}
}

// Score is the shrunk learned score evidence / (n + Prior), within -1..1.
func (e Evidence) Score() float64 {
	s := (e.Pos + e.Neg) / (e.PosN + e.NegN + Prior)
	return max(-1, min(1, s))
}

// Item is one taste item. Evidence is decayed to UpdatedAt. Polarity is set
// for a statement or user item; a learned item's follows its score.
// WindowStart is the plan window a window-scoped item belongs to.
type Item struct {
	ID          int64
	Level       Level
	Key         string
	Scope       Scope
	WindowStart time.Time
	Source      Source
	Polarity    Polarity
	Evidence    Evidence
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Active reports whether the item holds in the window starting at window:
// an always item does; a window item only in its own window (TASTE-6).
func (it Item) Active(window time.Time) bool {
	if it.Scope != ScopeWindow {
		return true
	}
	return !window.IsZero() && sameDay(it.WindowStart, window)
}

// Learned is the item's learned score at now.
func (it Item) Learned(now time.Time) float64 {
	return it.Evidence.Faded(now.Sub(it.UpdatedAt)).Score()
}

// Score is the item's score at now: +1 or -1 for a statement or user item
// (they outrank evidence and do not fade, TASTE-5/6), else the learned
// score.
func (it Item) Score(now time.Time) float64 {
	if it.Source != SourceLearned {
		if it.Polarity == Love {
			return 1
		}
		return -1
	}
	return it.Learned(now)
}

// Effective is the item's polarity at now: the stated one, or for a learned
// item love from Threshold up, rather not from -Threshold down, else none.
// A learned item is never "never" (TASTE-7).
func (it Item) Effective(now time.Time) Polarity {
	if it.Source != SourceLearned {
		return it.Polarity
	}
	switch s := it.Learned(now); {
	case s >= Threshold:
		return Love
	case s <= -Threshold:
		return RatherNot
	}
	return ""
}

// Ref names an item: its level and key.
type Ref struct {
	Level Level
	Key   string
}

// NormKey is the stored form of an item key: lower case, trimmed, inner
// spaces as underscores ("Middle Eastern" -> "middle_eastern"; hero names
// keep their words: "chicken_breast").
func NormKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), "_")
}

func sameDay(a, b time.Time) bool {
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

// trivial are the format and technique keys that say nothing about a
// household (most dishes are plated, boiled or simmered): the Taste page
// leaves them out, planning still scores them (TASTE-11).
var trivial = map[Level][]string{
	LevelFormat:    {"plate"},
	LevelTechnique: {"boil", "simmer"},
}

// Trivial reports whether the item level/key is one the Taste page leaves out.
func Trivial(level Level, key string) bool {
	return slices.Contains(trivial[level], key)
}
