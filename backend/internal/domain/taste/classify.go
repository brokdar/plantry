package taste

import (
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// flavourWords map words people use for a taste dimension (English and
// German) to the dimension.
var flavourWords = map[string]string{
	"spicy": "heat", "hot": "heat", "heat": "heat", "scharf": "heat", "chili": "heat",
	"sour": "acid", "acidic": "acid", "sauer": "acid", "acid": "acid",
	"sweet": "sweet", "süß": "sweet", "suess": "sweet",
	"salty": "salt", "salzig": "salt", "salt": "salt",
	"bitter": "bitter",
	"umami":  "umami", "savoury": "umami", "savory": "umami", "herzhaft": "umami",
}

// Classify names the item a word is about: a cuisine, technique or format
// of the attribute vocabulary, a taste word as its flavour dimension, else
// a hero ingredient.
func Classify(word string) Ref {
	k := NormKey(word)
	switch {
	case k == "":
		return Ref{}
	case slices.Contains(flavor.Cuisines, k) && k != "neutral":
		return Ref{LevelCuisine, k}
	case slices.Contains(flavor.Techniques, k):
		return Ref{LevelTechnique, k}
	case slices.Contains(flavor.Formats, k):
		return Ref{LevelFormat, k}
	}
	if d, ok := flavourWords[k]; ok {
		return Ref{LevelFlavour, d}
	}
	return Ref{LevelHero, k}
}

// preferencePrefixes are the record_preference key prefixes that state a
// taste ("likes_spicy", "hates_coriander").
var preferencePrefixes = []struct {
	prefix   string
	polarity Polarity
}{
	{"loves_", Love},
	{"love_", Love},
	{"likes_", Love},
	{"like_", Love},
	{"dislikes_", RatherNot},
	{"dislike_", RatherNot},
	{"hates_", RatherNot},
	{"hate_", RatherNot},
}

// PreferenceStatement reads a record_preference key/value as a taste
// statement: a likes_/loves_/dislikes_/hates_ key with the value true. ok
// is false for any other preference.
func PreferenceStatement(key string, value any) (Ref, Polarity, bool) {
	if v, isBool := value.(bool); !isBool || !v {
		return Ref{}, "", false
	}
	k := strings.ToLower(key)
	for _, p := range preferencePrefixes {
		if rest, found := strings.CutPrefix(k, p.prefix); found {
			if ref := Classify(strings.ReplaceAll(rest, "_", " ")); ref.Key != "" {
				return ref, p.polarity, true
			}
		}
	}
	return Ref{}, "", false
}
