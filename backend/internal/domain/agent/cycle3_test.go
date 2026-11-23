package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
)

// replyOf runs one write turn over a stored tool result and returns the stored reply.
func replyOf(t *testing.T, tool, file, user, model string) string {
	t.Helper()
	result, err := os.ReadFile("testdata/m3tune/" + file)
	require.NoError(t, err)
	ts := minimalToolSet(t, tool, result, ToolEffectNone, nil)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", tool), scriptEndTurn(model)}})
	req := makeReq(1)
	req.UserText = user
	req.Tools = ts.Describe()
	repo := &memRepo{}
	require.NoError(t, runLoopRepo(t, req, client, ts, repo))
	msgs, _ := repo.ListMessages(context.Background(), 1)
	var blocks []struct{ Type, Text string }
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
	require.NotEmpty(t, blocks)
	return blocks[len(blocks)-1].Text
}

// v1.4 P-7 (G4/RP4): a number of the model's text that equals one of the facts
// block or of the turn's tool results stays. tune3 g-add-05 and g-lighter-05
// lost their whole text to the old "no digit" rule.
func TestReply_ModelNumbersTheBlockBacksStay(t *testing.T) {
	t.Parallel()
	got := replyOf(t, "apply_changes", "add05_apply_changes.json", "Bitte füge am Freitag zum Abendessen Hummus hinzu.",
		"Hummus ist zum Freitagabend hinzugefügt (100 g). Der Abend liegt damit bei 843 kcal, der Tag bei 1500 kcal und im Rahmen.")
	assert.True(t, strings.HasPrefix(got, "Hummus ist zum Freitagabend hinzugefügt (100 g). Der Abend liegt damit bei 843 kcal, der Tag bei 1500 kcal und im Rahmen."), got)

	got = replyOf(t, "apply_changes", "lighter05_apply_changes.json", "make Saturday dinner lighter", // the reply's language is the request's (V81)
		"Saturday dinner is lighter now: the pasta and rice both went down to 0.75 portions. The meal is 638 kcal, and it can't go further without pushing the day under its minimum.")
	assert.True(t, strings.HasPrefix(got, "Saturday dinner is lighter now: the pasta and rice both went down to 0.75 portions."), got)
}

// P-7, the other side: a number neither the block nor the tool results hold,
// and a backed number in a sentence that breaks another rule, are dropped.
func TestReply_UnbackedNumbersAndOtherBreaksAreStillDropped(t *testing.T) {
	t.Parallel()
	got := replyOf(t, "apply_changes", "add05_apply_changes.json", "Bitte füge am Freitag zum Abendessen Hummus hinzu.",
		"Hummus ist zum Freitagabend hinzugefügt (175 g). Der Abend wird frisch gekocht und liegt bei 843 kcal.")
	assert.NotContains(t, got, "175 g")
	assert.NotContains(t, got, "frisch gekocht")
	assert.True(t, strings.HasPrefix(got, "- "), "the block goes out alone: %q", got)
}

// RP7: the dot of a German date ends no sentence (tune3 g-away-13 dropped
// "Dienstag, 8." as its own third sentence).
func TestSentences_GermanDatesAreNoSentenceEnd(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{
		"Dienstag, 8. Dezember, Abendessen: kein Gericht mehr.",
		"Montag, 7. Dez. Abendessen: Lachs wird für 2 statt 3 Portionen gekocht.",
		"Am 25. Nov. bleibt alles.",
	}, sentences("Dienstag, 8. Dezember, Abendessen: kein Gericht mehr. Montag, 7. Dez. Abendessen: Lachs wird für 2 statt 3 Portionen gekocht. Am 25. Nov. bleibt alles."))
	assert.Equal(t, []string{"It holds 0.75 portions.", "Done."}, sentences("It holds 0.75 portions. Done."))
	assert.Equal(t, []string{"Es sind 8.", "Fertig."}, sentences("Es sind 8. Fertig."), "a number before a word that is no month still ends")
}

// RP7 (PR #174 review): a sentence that ends on a month's own word ends there.
func TestSentences_SentenceEndingOnAMonthWordStillEnds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Das wird im Mai.", "Der Tag bleibt gleich."}, sentences("Das wird im Mai. Der Tag bleibt gleich."))
	assert.Equal(t, []string{"Das ist im Juni.", "Es bleibt so."}, sentences("Das ist im Juni. Es bleibt so."))
	assert.Equal(t, []string{"Am 3 Dez. Abendessen: Lachs."}, sentences("Am 3 Dez. Abendessen: Lachs."), "a day number before the abbreviation keeps the date whole")
	assert.Equal(t, []string{"Das war im Dez.", "Der Tag bleibt."}, sentences("Das war im Dez. Der Tag bleibt."), "an abbreviation with no day number and an uppercase word after it ends")
	// Documented outcome of the rule: a 1-2 digit number before a month word is a day.
	assert.Equal(t, []string{"Platz 3. Dezember ist es."}, sentences("Platz 3. Dezember ist es."), "Platz 3. Dezember: read as a date, no split")
}

// P-7 (PR #174 review): on a write turn only the facts block and the turn's tool
// results back a number; the plan state of the prompt does not, nor a date's day.
func TestReply_WriteTurnNumbersOnlyInThePlanStateAreDropped(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Fri 27 Nov dinner: A replaced by B.", DE: "x"}}}
	turn := replyTurn{user: "swap it", write: true, block: block, numbers: map[string]bool{"843": true}, state: map[string]bool{"750": true}}
	got := composeReply("The evening holds 843 kcal. Before it held 750 kcal.", turn)
	assert.Contains(t, got, "The evening holds 843 kcal.")
	assert.NotContains(t, got, "750", "a number only the plan state holds")
	// the same state number backs a read-only answer
	read := replyTurn{user: "how much?", numbers: map[string]bool{}, state: map[string]bool{"750": true}}
	assert.Equal(t, "It holds 750 kcal.", composeReply("It holds 750 kcal.", read))
}

func TestToolNumbers_DatesAndBareIdsBackNothing(t *testing.T) {
	t.Parallel()
	set := map[string]bool{}
	toolNumbers([]byte(`{"ids":[17,18],"label":"Wed 25 Nov dinner","note":"moved to 25. Dezember","kcal":843}`), set)
	assert.Equal(t, map[string]bool{"843": true}, set)
	turn := replyTurn{user: "x", write: true, numbers: set, block: &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "A replaced by B.", DE: "x"}}}}
	assert.NotContains(t, composeReply("Add 25 g of it.", turn), "25 g")
}

func TestLint_GermanCommaWithThreeDigitsIsADecimal(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1.25", normModelNumber("1,250", langDE))
	assert.Equal(t, "1250", normModelNumber("1,250", langEN))
	assert.Equal(t, "1250", normModelNumber("1.250", langDE))
	turn := replyTurn{user: "Bitte tausche das", numbers: map[string]bool{"1250": true}, state: map[string]bool{}}
	assert.Equal(t, fallbackDE, composeReply("Das sind 1,250 kg im Ganzen, bitte nicht mit dem Tag für 1250 verwechseln.", turn), "1,250 kg is 1.25 in German, not backed by 1250")
}
