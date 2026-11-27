// Package fdclocal is an offline, embedded copy of the generic USDA
// FoodData Central foods (SR Legacy and Foundation), so ingredient matching
// works without an FDC API key. foods.tsv.gz is built by tools/fdcgen (see
// there to regenerate); USDA FDC data is public domain (CC0 1.0).
package fdclocal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

//go:embed foods.tsv.gz
var foodsGz []byte

// Catalogue searches the embedded foods. The index is built on first use.
type Catalogue struct {
	once  sync.Once
	err   error
	foods []entry
	byID  map[int]int32
	index map[string][]int32 // token -> foods naming it
}

type entry struct {
	id       int
	desc     string
	nutr     [8]float64 // kcal, protein, fat, carbs, fiber, sugar, sodium mg, saturated fat
	has      uint8      // bit i set when nutr[i] is reported
	tokens   []string   // distinct tokens of desc
	head     string     // first token: the food ("Vinegar, cider")
	prepared bool       // a cooking or preparation word
	raw      bool
	branded  bool
	portions []food.PortionInfo
}

// New returns the embedded catalogue.
func New() *Catalogue { return &Catalogue{} }

func (c *Catalogue) load() error {
	c.once.Do(func() { c.err = c.parse() })
	return c.err
}

func (c *Catalogue) parse() error {
	zr, err := gzip.NewReader(bytes.NewReader(foodsGz))
	if err != nil {
		return fmt.Errorf("fdclocal: %w", err)
	}
	c.byID = map[int]int32{}
	c.index = map[string][]int32{}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		switch {
		case f[0] == "F" && len(f) == 12:
			id, err := strconv.Atoi(f[1])
			if err != nil {
				return fmt.Errorf("fdclocal: food id %q: %w", f[1], err)
			}
			e := entry{id: id, desc: f[3]}
			for i, s := range f[4:] {
				if s == "" {
					continue
				}
				v, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return fmt.Errorf("fdclocal: food %d: %w", id, err)
				}
				e.nutr[i], e.has = v, e.has|1<<i
			}
			e.tokens = tokens(e.desc)
			if len(e.tokens) > 0 {
				e.head = e.tokens[0]
			}
			for _, t := range e.tokens {
				e.prepared = e.prepared || preparedWords[t]
				e.raw = e.raw || t == "raw"
			}
			e.branded = brandCaps(e.desc)
			idx := int32(len(c.foods))
			c.foods = append(c.foods, e)
			c.byID[id] = idx
			for _, t := range e.tokens {
				c.index[t] = append(c.index[t], idx)
			}
		case f[0] == "P" && len(f) == 5:
			id, _ := strconv.Atoi(f[1])
			g, err := strconv.ParseFloat(f[4], 64)
			i, ok := c.byID[id]
			if err != nil || !ok {
				continue
			}
			c.foods[i].portions = append(c.foods[i].portions, food.PortionInfo{RawUnit: f[2], Modifier: f[3], GramWeight: g})
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("fdclocal: %w", err)
	}
	return nil
}

// SearchByName implements food.FoodProvider. Foods naming every query word
// come first; among them the food the description starts with ("Vinegar,
// cider" for "cider vinegar"), raw before prepared unless the query asks
// for a cooked food, and fewer extra words (generic before specific).
func (c *Catalogue) SearchByName(_ context.Context, query string, limit int) ([]food.Candidate, error) {
	if err := c.load(); err != nil {
		return nil, err
	}
	q := tokens(query)
	if len(q) == 0 || limit <= 0 {
		return []food.Candidate{}, nil
	}
	cooked := false
	for _, t := range q {
		cooked = cooked || preparedWords[t]
	}
	// "raw" and "fresh" name no food: they rank, they do not match.
	hits := map[int32]int{}
	for _, t := range q {
		if (t == "raw" || t == "fresh") && len(q) > 1 {
			continue
		}
		for _, i := range c.index[t] {
			hits[i]++
		}
	}
	type scored struct {
		i       int32
		matched int
		score   float64
	}
	ranked := make([]scored, 0, len(hits))
	for i, n := range hits {
		e := &c.foods[i]
		s := -0.5 * float64(len(e.tokens)-n)
		if slices.Contains(q, e.head) {
			s += 3
		}
		switch {
		case cooked && e.prepared:
			s += 2
		case !cooked && e.prepared:
			s -= 2
		case !cooked && e.raw:
			s++
		}
		if e.branded {
			s -= 3
		}
		ranked = append(ranked, scored{i, n, s})
	}
	slices.SortFunc(ranked, func(a, b scored) int {
		switch {
		case a.matched != b.matched:
			return b.matched - a.matched
		case a.score != b.score:
			if a.score > b.score {
				return -1
			}
			return 1
		}
		return c.foods[a.i].id - c.foods[b.i].id
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]food.Candidate, len(ranked))
	for k, r := range ranked {
		out[k] = c.foods[r.i].candidate()
	}
	return out, nil
}

// GetByID implements food.FoodByIDProvider.
func (c *Catalogue) GetByID(_ context.Context, fdcID int) (food.Candidate, error) {
	if err := c.load(); err != nil {
		return food.Candidate{}, err
	}
	i, ok := c.byID[fdcID]
	if !ok {
		return food.Candidate{}, fmt.Errorf("%w: fdc food %d not in the offline catalogue", domain.ErrNotFound, fdcID)
	}
	return c.foods[i].candidate(), nil
}

// GetFoodPortions implements food.PortionProvider: grams per one unit.
func (c *Catalogue) GetFoodPortions(_ context.Context, fdcID int) ([]food.PortionInfo, error) {
	if err := c.load(); err != nil {
		return nil, err
	}
	i, ok := c.byID[fdcID]
	if !ok {
		return []food.PortionInfo{}, nil
	}
	return slices.Clone(c.foods[i].portions), nil
}

func (e *entry) candidate() food.Candidate {
	v := func(i int) *float64 {
		if e.has&(1<<i) == 0 {
			return nil
		}
		x := e.nutr[i]
		return &x
	}
	sodium := v(6)
	if sodium != nil {
		*sodium /= 1000 // mg -> g, as adapters/fdc reports it
	}
	return food.Candidate{
		Name: e.desc, SourceName: e.desc, Source: food.SourceFDC, FdcID: e.id,
		Kcal100g: v(0), Protein100g: v(1), Fat100g: v(2), Carbs100g: v(3),
		Fiber100g: v(4), Sugar100g: v(5), Sodium100g: sodium, SaturatedFat100g: v(7),
	}
}

// preparedWords mark a cooked or prepared food in an FDC description.
var preparedWords = map[string]bool{
	"cooked": true, "boiled": true, "baked": true, "roasted": true, "fried": true, "grilled": true,
	"broiled": true, "braised": true, "stewed": true, "steamed": true, "simmered": true, "microwaved": true,
	"prepared": true, "heated": true, "toasted": true, "breaded": true, "batter": true, "drained": true,
}

var stopWords = map[string]bool{
	"and": true, "or": true, "with": true, "without": true, "of": true, "in": true,
	"the": true, "a": true, "to": true, "includes": true, "include": true, "from": true,
}

// tokens lowercases s, splits it into words, drops stop words and
// duplicates and reduces plurals, so "Tomatoes" and "tomato" meet.
func tokens(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := words[:0]
	for _, w := range words {
		if stopWords[w] {
			continue
		}
		w = stem(w)
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func stem(w string) string {
	switch {
	case len(w) <= 3:
		return w
	case strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "oes"), strings.HasSuffix(w, "ches"), strings.HasSuffix(w, "shes"), strings.HasSuffix(w, "sses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return w[:len(w)-1]
	}
	return w
}

// brandCaps: a word of three or more capitals ("SILK", "PILLSBURY") marks
// a branded SR Legacy entry.
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
