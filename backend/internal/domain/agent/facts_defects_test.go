package agent

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// G4/CC19: a fact is told whole. A cooking line with long dish names keeps its
// minutes in both languages: nothing cuts a line.
func TestFacts_CookingLineWithLongNamesKeepsItsMinutes(t *testing.T) {
	t.Parallel()
	long := func(n string) string { return strings.Repeat(n, 30) }
	d := func(day int) time.Time { return time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC) }
	one := 1.0
	st := func(cook int) planning.State {
		return planning.State{
			SlotNames: map[int64]string{3: "slot.dinner"}, SlotOrder: plate.SlotOrder{3: 1},
			Window: window.Window{Start: d(21), End: d(27)},
			Foods: map[int64]planning.FoodFacts{
				1: {Name: long("Lachs im Blätterteig "), Minutes: 80}, 2: {Name: long("Hasselback potatoes "), Minutes: 75}, 3: {Name: long("Kartoffelgratin klassisch "), Minutes: 50},
			},
			Sessions: []planning.Session{
				{CookSession: plate.CookSession{ID: 1, FoodID: 1, CookDate: d(cook), CookSlotID: 3}},
				{CookSession: plate.CookSession{ID: 2, FoodID: 2, CookDate: d(cook), CookSlotID: 3}},
				{CookSession: plate.CookSession{ID: 3, FoodID: 3, CookDate: d(cook), CookSlotID: 3}},
			},
			Plates: []planning.Plate{{ID: 7, Date: d(26), SlotID: 3, Eaters: 1, Components: []planning.Component{
				{FoodID: 1, Portions: &one, SessionID: 1}, {FoodID: 2, Portions: &one, SessionID: 2}, {FoodID: 3, Portions: &one, SessionID: 3},
			}}},
		}
	}
	in := reportInput{beforeSt: st(26), afterSt: st(25), facts: &factSink{}}
	buildFacts(in, changeReport{})
	block := selectBlock(in.facts.items)
	require.Len(t, block, 1)
	assert.Greater(t, len([]rune(block[0].EN)), 280, "a long line is not cut")
	assert.Contains(t, block[0].EN, "the longest recipe takes about 1 h 20 min.")
	assert.Contains(t, block[0].DE, "das längste Rezept braucht etwa 1 Std. 20 Min.")
}

// RP8: German numbers have a comma and German words; a single person is "1 person".
func TestFacts_GermanNumbersAndSingularPeople(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "2,5 Punkte", badnessWordsL(planning.CodeWeekFatRange, 2.5, langDE))
	assert.Equal(t, "2.5 points", badnessWordsL(planning.CodeWeekFatRange, 2.5, langEN))
	en, de := peopleWords(1)
	assert.Equal(t, "1 person", en)
	assert.Equal(t, "1 Person", de)
	en, de = peopleWords(2)
	assert.Equal(t, "2 people", en)
	assert.Equal(t, "2 Personen", de)
	en, de = peopleWords(1.5)
	assert.Equal(t, "1.5 people", en)
	assert.Equal(t, "1,5 Personen", de)
}

// G4/RP5: one fact per figure. A day told as worse and the warning about the same
// day are one line, in the earlier class.
func TestSelectBlock_OneFactPerFigure(t *testing.T) {
	t.Parallel()
	all := []fact{
		{Class: classWarn, Kind: kindWarn, Status: statusUnder, EN: "New warning: Sat 21 Nov holds 1600 kcal.", DE: "Neue Warnung: x", Key: "day:Sat 21 Nov"},
		{Class: classWorse, Kind: kindDay, Status: statusUnder, EN: "Sat 21 Nov holds 1600 kcal (was 2000).", DE: "y", Key: "day:Sat 21 Nov"},
		{Class: classWorse, Kind: kindDay, Status: statusUnder, EN: "Sun 22 Nov holds 1500 kcal.", DE: "z", Key: "day:Sun 22 Nov"},
	}
	got := selectBlock(all)
	require.Len(t, got, 2)
	assert.Equal(t, "Sat 21 Nov holds 1600 kcal (was 2000).", got[0].EN)
	assert.Equal(t, classWorse, got[0].Class)
}

// The same figure through the producers: a day under its range is one line.
func TestBuildReport_DayUnderRangeIsToldOnce(t *testing.T) {
	t.Parallel()
	before := reportState(2, 1, 1)
	after := reportState(1.5, 0.5, 1)
	issue := planning.Issue{Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-21", Params: map[string]any{"kcal": 1600.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}}
	r := buildReport(reportInput{
		beforeSt: before, afterSt: after,
		before: planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-21", Kcal: 2000, BandLow: 1800, BandHigh: 2200}}},
		after: planning.Evaluation{
			Plates: []planning.PlateEval{{PlateID: 10, Kcal: 400}},
			Days:   []planning.DayEval{{Date: "2026-11-21", Kcal: 1600, BandLow: 1800, BandHigh: 2200}},
			Issues: []planning.Issue{issue},
		},
	})
	var n int
	for _, f := range r.Facts {
		if strings.Contains(f.EN, "1600 kcal") {
			n++
		}
	}
	assert.Equal(t, 1, n, "%v", r.Tell)
}

// SF2: a refused part says what (dish, day, meal) and why, in both languages.
func TestRefusedPart_NamesWhatAndWhy(t *testing.T) {
	t.Parallel()
	got := refusedFact("removing \"Greek yogurt\" from Wed 25 Nov dinner", "„Greek yogurt“ von Mittwoch, 25. Nov. Abendessen entfernen", "Wed 25 Nov would fall to 1480 kcal, under its minimum of 1500 kcal", "Mittwoch, 25. Nov. würde auf 1480 kcal fallen, unter den Mindestwert von 1500 kcal")
	assert.Equal(t, `Left out: removing "Greek yogurt" from Wed 25 Nov dinner (Wed 25 Nov would fall to 1480 kcal, under its minimum of 1500 kcal).`, got["en"])
	assert.Equal(t, `Weggelassen: „Greek yogurt“ von Mittwoch, 25. Nov. Abendessen entfernen (Mittwoch, 25. Nov. würde auf 1480 kcal fallen, unter den Mindestwert von 1500 kcal).`, got["de"])
}

// The check snapshot: the touched days before and after, with plate ids,
// components and batches, so the facts can be checked against the stored plan.
func TestBuildReport_CheckHoldsTheTouchedDaysBeforeAndAfter(t *testing.T) {
	t.Parallel()
	before := reportState(2, 1, 1)
	after := reportState(1.5, 0.5, 1)
	r := buildReport(reportInput{
		beforeSt: before, afterSt: after,
		after: planning.Evaluation{Plates: []planning.PlateEval{{PlateID: 10, Kcal: 400}}},
	})
	require.NotNil(t, r.Check)
	require.NotEmpty(t, r.Check.BeforeDays)
	require.Equal(t, len(r.Check.BeforeDays), len(r.Check.AfterDays))
	day := r.Check.AfterDays[0]
	assert.Equal(t, "2026-11-21", day.Date)
	assert.Equal(t, []int64{10}, day.PlateIDs)
	require.Len(t, day.Plates, 1)
	require.Len(t, day.Plates[0].Components, 1)
	assert.Equal(t, 0.5, *day.Plates[0].Components[0].Portions)
	assert.Equal(t, 1.0, *r.Check.BeforeDays[0].Plates[0].Components[0].Portions)
	require.NotEmpty(t, day.Sessions)

	// the model reads no part of it
	raw, err := json.Marshal(map[string]any{"report": r})
	require.NoError(t, err)
	view := string(withoutFacts(raw))
	assert.NotContains(t, view, `"check"`)
	assert.NotContains(t, view, `"facts"`)
	assert.Contains(t, view, `"tell"`)
}

var quotedName = regexp.MustCompile(`"([^"]+)"`)

// assertOfferDE: the German offer carries the dishes and the numbers of the English one.
func assertOfferDE(t *testing.T, what, whatDE string) {
	t.Helper()
	require.NotEmpty(t, whatDE, "no German offer for %q", what)
	for _, m := range quotedName.FindAllStringSubmatch(what, -1) {
		assert.Contains(t, whatDE, m[1], "the German offer names the dish %q: %s", m[1], whatDE)
	}
	for _, n := range regexp.MustCompile(`\d+(?:\.\d+)?`).FindAllString(what, -1) {
		assert.Contains(t, whatDE, strings.Replace(n, ".", ",", 1), "the German offer has the number %s: %s", n, whatDE)
	}
}
