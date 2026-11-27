package finds

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Cross-check constants (FIND-11, tunable).
const (
	// OutlierDeviations is how many robust deviations make an outlier.
	OutlierDeviations = 2.0
	// minSpread keeps a near-zero spread from flagging tiny differences: the
	// deviation is at least this share of the median.
	minSpread = 0.1
	// Single source (FIND-11): at least this rating from this many votes.
	singleRating = 4.5
	singleVotes  = 200
)

// Cross-check metrics (flag codes "outlier_<metric>").
const (
	MetricGramsPerServing = "grams_per_serving"
	MetricSaltPerKg       = "salt_per_kg"
	MetricOvenTemp        = "oven_temp"
	MetricTotalMinutes    = "total_minutes"
	MetricLiquidStarch    = "liquid_to_starch"
)

var metrics = []string{MetricGramsPerServing, MetricSaltPerKg, MetricOvenTemp, MetricTotalMinutes, MetricLiquidStarch}

// CrossCheck flags the base's metrics that lie beyond OutlierDeviations
// robust deviations (median absolute deviation) from all versions. It never
// averages: the base keeps its own values. It needs at least three versions
// with the metric.
func CrossCheck(base *importer.Draft, versions []*importer.Draft) []Flag {
	var flags []Flag
	b := Metrics(base)
	all := make([]map[string]float64, 0, len(versions))
	for _, v := range versions {
		all = append(all, Metrics(v))
	}
	for _, m := range metrics {
		bv, ok := b[m]
		if !ok {
			continue
		}
		var xs []float64
		for _, a := range all {
			if v, ok := a[m]; ok {
				xs = append(xs, v)
			}
		}
		if len(xs) < 3 {
			continue
		}
		med := median(xs)
		devs := make([]float64, len(xs))
		for i, x := range xs {
			devs[i] = math.Abs(x - med)
		}
		spread := math.Max(1.4826*median(devs), minSpread*math.Abs(med))
		if spread > 0 && math.Abs(bv-med) > OutlierDeviations*spread {
			flags = append(flags, Flag{Code: "outlier_" + m, Value: round1(bv), Typical: round1(med)})
		}
	}
	return flags
}

// SingleSourceEnough: one version suffices at ≥ 4.5 from ≥ 200 votes when
// the dish is not baked (FIND-11; the gates and a light adaptation are
// checked by the caller).
func SingleSourceEnough(s Source, d *importer.Draft) bool {
	return s.Rating != nil && *s.Rating >= singleRating && s.Votes >= singleVotes && !Baking(d)
}

var bakingWords = []string{"bread", "cake", "muffin", "cookie", "brownie", "pastry", "brot", "kuchen", "torte", "keks", "plätzchen", "gebäck"}

// Baking reports whether the dish is baked goods by its name.
func Baking(d *importer.Draft) bool {
	n := strings.ToLower(d.Name)
	return slices.ContainsFunc(bakingWords, func(w string) bool { return strings.Contains(n, w) })
}

var (
	tempRE      = regexp.MustCompile(`(\d{2,3})\s*(?:°|grad|degrees)\s*(c|f)?`)
	saltWords   = []string{"salt", "salz"}
	liquidWords = []string{"water", "broth", "stock", "milk", "wasser", "brühe", "bruehe", "fond", "milch", "coconut milk", "kokosmilch"}
	starchWords = []string{"rice", "pasta", "flour", "lentil", "reis", "nudel", "mehl", "linsen", "spaghetti", "potato", "kartoffel"}
)

// Metrics computes the cross-check metrics a draft has.
func Metrics(d *importer.Draft) map[string]float64 {
	out := map[string]float64{}
	var total, salt, liquid, starch float64
	for _, in := range d.Ingredients {
		g := lineGrams(in)
		total += g
		name := strings.ToLower(in.Name)
		switch {
		case hasWord(name, saltWords):
			salt += g
		case hasWord(name, liquidWords):
			liquid += g
		case hasWord(name, starchWords):
			starch += g
		}
	}
	if total > 0 && d.ReferencePortions > 0 {
		out[MetricGramsPerServing] = total / d.ReferencePortions
		if salt > 0 {
			out[MetricSaltPerKg] = salt / total * 1000
		}
	}
	if liquid > 0 && starch > 0 {
		out[MetricLiquidStarch] = liquid / starch
	}
	if t := ovenTemp(d.Instructions); t > 0 {
		out[MetricOvenTemp] = t
	}
	if m := totalMinutes(d); m > 0 {
		out[MetricTotalMinutes] = float64(m)
	}
	return out
}

// lineGrams is the line's weight in grams (ml counts as grams), 0 when
// unknown.
func lineGrams(in importer.DraftIngredient) float64 {
	switch {
	case in.ToTaste:
		return 0
	case in.Unit == "g" || in.Unit == "ml":
		return in.Amount
	case in.PortionGrams != nil:
		return in.Amount * *in.PortionGrams
	}
	return 0
}

func hasWord(name string, words []string) bool {
	return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(name, w) })
}

func ovenTemp(steps []string) float64 {
	best := 0.0
	for _, s := range steps {
		for _, m := range tempRE.FindAllStringSubmatch(strings.ToLower(s), -1) {
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			if m[2] == "f" {
				v = (v - 32) * 5 / 9
			}
			if v >= 100 && v <= 300 {
				best = math.Max(best, v)
			}
		}
	}
	return best
}

func totalMinutes(d *importer.Draft) int {
	if d.TotalMinutes != nil {
		return *d.TotalMinutes
	}
	m := 0
	if d.PrepMinutes != nil {
		m += *d.PrepMinutes
	}
	if d.CookMinutes != nil {
		m += *d.CookMinutes
	}
	return m
}

func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
