package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// mustItem is one fact of a saved change the final reply has to carry: the
// line to add in English and German, and the test for a reply that already
// says it. Built from the report's tell items by mustItems.
type mustItem struct {
	en, de string
	said   func(text string, german bool) bool
}

var (
	mustDay      = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}) holds (\d+) kcal \(was \d+\), (below|above) a good amount for the day(?:, under the day's minimum)? \((\d+)-(\d+) kcal\)\.$`)
	mustWeekPre  = "For the week this means: "
	mustEnergy   = regexp.MustCompile(`^average energy is (\d+)% of the target \(was (\d+)%\); the week's range is (\d+)[–-](\d+)%$`)
	mustMeat     = regexp.MustCompile(`^meat is (\d+) g \(was (\d+) g, limit (?:for the \d+ days you are home: )?(\d+) g\)$`)
	mustAwayMeat = regexp.MustCompile(`^With .* away the week is counted over (\d+) days; the meat limit for those days is (\d+) g and the plan holds (\d+) g, as before(?:; that is (\d+) g over)?\.$`)
	mustShare    = regexp.MustCompile(`^Worse: Weekly (fat|carbs): (\d+)% of the week's energy; the range is (\d+)-(\d+)%\.`)
	mustNoCook   = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}): nothing is cooked\.$`)
	mustDropped  = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}) cooking changes: (.*)\.$`)
	mustMoved    = regexp.MustCompile(`^The (".*?") is now cooked on (\w{3}) (\d+) (\w{3}) at (\w+) instead of (\w{3}) (\d+) (\w{3}) at (\w+)\.$`)
	noLongerCook = regexp.MustCompile(`no longer cooks (".*?")`)
)

var deMeal = map[string]string{"breakfast": "zum Frühstück", "lunch": "zum Mittagessen", "dinner": "zum Abendessen"}

// mustItems turns one tell item into the facts the reply must carry: a day
// outside its range, a week figure that got worse, a cooking that is gone or
// moved, a batch cooked on another day. Any other item gives none.
func mustItems(tell string) []mustItem {
	if resizeLine.MatchString(tell) {
		return []mustItem{{en: tell, de: germanResize(tell), said: func(text string, german bool) bool { return resizeSaid(text, tell, german) }}}
	}
	if p := mustDay.FindStringSubmatch(tell); p != nil {
		return []mustItem{dayItem(p)}
	}
	if rest, ok := strings.CutPrefix(tell, mustWeekPre); ok {
		var figs []string
		for _, f := range strings.Split(strings.TrimSuffix(rest, "."), "; ") {
			// The energy figure holds "; the week's range ...": one figure.
			if n := len(figs); n > 0 && strings.HasPrefix(f, "the week's range") {
				figs[n-1] += "; " + f
				continue
			}
			figs = append(figs, f)
		}
		var out []mustItem
		for _, f := range figs {
			out = append(out, weekItems(f)...)
		}
		return out
	}
	return otherMust(tell)
}

func weekItems(fig string) []mustItem {
	if p := mustEnergy.FindStringSubmatch(fig); p != nil {
		now, was, lo, hi := p[1], p[2], p[3], p[4]
		n, _ := strconv.Atoi(now)
		w, _ := strconv.Atoi(was)
		l, _ := strconv.Atoi(lo)
		h, _ := strconv.Atoi(hi)
		if n >= w && n >= l && n <= h {
			return nil // not worse, still in the band
		}
		return []mustItem{{
			en: fmt.Sprintf("Week: average energy is %s%% of the target (was %s%%); the week's range is %s–%s%%.", now, was, lo, hi),
			de: fmt.Sprintf("Woche: Durchschnitt %s %% des Ziels (vorher %s %%); der Bereich liegt bei %s–%s %%.", now, was, lo, hi),
			said: func(text string, _ bool) bool {
				return regexp.MustCompile(`(^|[^\d.,])` + now + `\s?(%|Prozent|percent)`).MatchString(text)
			},
		}}
	}
	if p := mustMeat.FindStringSubmatch(fig); p != nil {
		now, was, limit := atoi(p[1]), atoi(p[2]), atoi(p[3])
		if now <= limit && now <= was {
			return nil
		}
		return []mustItem{meatItem(now, limit)}
	}
	return nil
}

// meatItem says the week's meat against its limit, with the grams over.
func meatItem(now, limit int) mustItem {
	over := now - limit
	if over <= 0 {
		return mustItem{
			en:   fmt.Sprintf("Week: meat is %d g, the limit is %d g.", now, limit),
			de:   fmt.Sprintf("Woche: Fleisch %d g, Grenze %d g.", now, limit),
			said: func(text string, _ bool) bool { return lineWith(text, meatLine(now)) },
		}
	}
	return mustItem{
		en: fmt.Sprintf("Week: meat is %d g against a limit of %d g, %d g over.", now, limit, over),
		de: fmt.Sprintf("Woche: Fleisch %d g bei höchstens %d g, also %d g darüber.", now, limit, over),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool {
				return numberIn(l, fmt.Sprint(over)) && overWord.MatchString(l)
			})
		},
	}
}

var overWord = regexp.MustCompile(`(?i)\b(over|above|exceed\w*|more than|über\w*|darüber|drüber|mehr)\b`)

func meatLine(now int) func(string) bool {
	return func(l string) bool {
		return regexp.MustCompile(`(?i)meat|fleisch`).MatchString(l) && numberIn(l, fmt.Sprint(now))
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func otherMust(tell string) []mustItem {
	if p := mustAwayMeat.FindStringSubmatch(tell); p != nil {
		if now, limit := atoi(p[3]), atoi(p[2]); now > limit {
			return []mustItem{meatItem(now, limit)}
		}
		return nil
	}
	if p := mustShare.FindStringSubmatch(tell); p != nil {
		kind, pct, lo, hi := p[1], p[2], p[3], p[4]
		en, de, re := "fat", "Fett", `(?i)fat|fett`
		if kind == "carbs" {
			en, de, re = "carbohydrates", "Kohlenhydrate", `(?i)carb|kohlenhydrat`
		}
		return []mustItem{{
			en: fmt.Sprintf("Week: %s is %s%% of the energy (range %s–%s%%), further out than before.", en, pct, lo, hi),
			de: fmt.Sprintf("Woche: %s machen %s %% der Energie aus (Bereich %s–%s %%), weiter außerhalb als vorher.", de, pct, lo, hi),
			said: func(text string, _ bool) bool {
				return lineWith(text, func(l string) bool { return regexp.MustCompile(re).MatchString(l) && numberIn(l, pct) })
			},
		}}
	}
	if p := mustNoCook.FindStringSubmatch(tell); p != nil {
		wd, dd, mon := p[1], p[2], p[3]
		return []mustItem{{
			en: fmt.Sprintf("%s %s %s: nothing is cooked any more.", wd, dd, mon),
			de: fmt.Sprintf("%s: es wird nichts mehr gekocht.", deDay(wd, dd, mon)),
			said: func(text string, german bool) bool {
				return lineWith(text, func(l string) bool { return dayIn(l, wd, dd, mon, german) && noCookWord.MatchString(l) })
			},
		}}
	}
	if p := mustDropped.FindStringSubmatch(tell); p != nil {
		var out []mustItem
		wd, dd, mon := p[1], p[2], p[3]
		for _, d := range noLongerCook.FindAllStringSubmatch(p[4], -1) {
			dish := d[1]
			name := strings.Trim(dish, `"`)
			out = append(out, mustItem{
				en: fmt.Sprintf("%s %s %s: %s is no longer cooked.", wd, dd, mon, dish),
				de: fmt.Sprintf("%s: „%s“ wird nicht mehr gekocht.", deDay(wd, dd, mon), name),
				said: func(text string, german bool) bool {
					return lineWith(text, func(l string) bool {
						return dayIn(l, wd, dd, mon, german) && strings.Contains(strings.ToLower(l), strings.ToLower(firstWord(name))) && noCookWord.MatchString(l)
					})
				},
			})
		}
		return out
	}
	if p := mustMoved.FindStringSubmatch(tell); p != nil {
		dish := p[1]
		nw, nd, nm, nat := p[2], p[3], p[4], p[5]
		ow, od, om, oat := p[6], p[7], p[8], p[9]
		return []mustItem{{
			en: tell,
			de: fmt.Sprintf("„%s“ wird jetzt am %s %s gekocht, nicht mehr am %s %s.", strings.Trim(dish, `"`), deDay(nw, nd, nm), deMealAt(nat), deDay(ow, od, om), deMealAt(oat)),
			said: func(text string, german bool) bool {
				return lineWith(text, func(l string) bool {
					return dayIn(l, nw, nd, nm, german) && dayIn(l, ow, od, om, german)
				})
			},
		}}
	}
	return nil
}

var noCookWord = regexp.MustCompile(`(?i)(nothing|no longer|not cooked|no cooking|leftover|left over|nichts|nicht mehr|kein|reste)`)

func deMealAt(m string) string {
	if s, ok := deMeal[m]; ok {
		return "(" + s + ")"
	}
	return ""
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

func dayItem(p []string) mustItem {
	wd, dd, mon, kcal, side, lo, hi := p[1], p[2], p[3], p[4], p[5], p[6], p[7]
	de := "unter"
	if side == "above" {
		de = "über"
	}
	return mustItem{
		en: fmt.Sprintf("%s %s %s: %s kcal, %s a good amount for the day (%s–%s kcal).", wd, dd, mon, kcal, side, lo, hi),
		de: fmt.Sprintf("%s: %s kcal, %s dem passenden Bereich für den Tag (%s–%s kcal).", deDay(wd, dd, mon), kcal, de, lo, hi),
		said: func(text string, german bool) bool {
			return lineWith(text, func(l string) bool { return dayIn(l, wd, dd, mon, german) && numberIn(l, kcal) })
		},
	}
}

// deDay renders "Fri 27 Nov" as "Freitag, 27. Nov."; an unknown name stays.
func deDay(wd, dd, mon string) string {
	w, ok1 := deWeekday[wd]
	m, ok2 := deMonth[mon]
	if !ok1 || !ok2 {
		return wd + " " + dd + " " + mon
	}
	return fmt.Sprintf("%s, %s. %s", w[0], dd, m.abb)
}

func lineWith(text string, f func(line string) bool) bool {
	for _, l := range strings.Split(text, "\n") {
		if f(l) {
			return true
		}
	}
	return false
}

var enWeekday = map[string]string{
	"Mon": "monday", "Tue": "tuesday", "Wed": "wednesday", "Thu": "thursday",
	"Fri": "friday", "Sat": "saturday", "Sun": "sunday",
}

// dayIn reports a day named in line, by weekday (name or short form) or date.
func dayIn(line, wd, dd, mon string, german bool) bool {
	l := strings.ToLower(line)
	if german {
		m := deMonth[mon]
		date := regexp.MustCompile(`(^|\D)` + dd + `\.\s?(` + regexp.QuoteMeta(strings.TrimSuffix(m.abb, ".")) + `|0?` + fmt.Sprint(m.num) + `\.)`)
		return strings.Contains(l, strings.ToLower(deWeekday[wd][0])) || date.MatchString(line)
	}
	full := enWeekday[wd]
	if full == "" {
		return strings.Contains(line, wd+" "+dd)
	}
	return strings.Contains(l, full) || strings.Contains(l, strings.ToLower(wd)+" ") ||
		regexp.MustCompile(`(?i)\b`+dd+`(st|nd|rd|th)?\.? `+mon+`\b|\b`+mon+`\w* `+dd+`\b`).MatchString(line)
}
