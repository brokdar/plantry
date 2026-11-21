package agent_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

var updateFacts = flag.Bool("update-facts", false, "rewrite testdata/facts_golden.json")

// raw runs a tool and returns its result as the loop gets it.
func (e *planToolEnv) raw(name, input string) json.RawMessage {
	e.t.Helper()
	out, _, _, err := e.ts.Execute(e.ctx, name, json.RawMessage(input))
	require.NoError(e.t, err)
	return out
}

// goldenCase is one core request type: what the user asked in each language, the
// model's own framing (the same in the language), and the tool result the reply
// is made from.
type goldenCase struct {
	name         string
	userEN       string
	userDE       string
	modelEN      string
	modelDE      string
	write        bool
	run          func(e *planToolEnv) json.RawMessage
	prepare      func(t *testing.T) *planToolEnv
	staticOutput string
}

// The golden file holds, per core request type, the whole reply a household reads
// in English and in German: the model's framing, the facts block and the one
// offer. A change of a sentence shows here first. Rewrite with
// `go test ./internal/domain/agent -run TestFactsGolden -update-facts`.
func TestFactsGolden_CoreRequestTypes(t *testing.T) {
	cases := []goldenCase{
		{
			name: "move_swap", userEN: "swap the two dinners", userDE: "tausche die beiden Abendessen", modelEN: "Done, I swapped them.", modelDE: "Erledigt, ich habe sie getauscht.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, q := e.twoDinners()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"move","plate_id":%d,"to":%s,"occupant":"swap"}]}`, p.ID, cellJSON(q.DateString(), q.SlotID)))
			},
		},
		{
			name: "away", userEN: "we are out on the third day", userDE: "wir sind am dritten Tag weg", modelEN: "Noted, you are out that day.", modelDE: "Notiert, ihr seid an dem Tag weg.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, _ := e.twoDinners()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"away","date":%q}]}`, p.DateString()))
			},
		},
		{
			name: "guests", userEN: "a friend joins the dinner", userDE: "ein Freund isst mit", modelEN: "Sure, the dinner is for two now.", modelDE: "Klar, das Abendessen ist jetzt für zwei.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, _ := e.twoDinners()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, p.ID))
			},
		},
		{
			name: "add_food", userEN: "add a side to the dinner", userDE: "füge dem Abendessen eine Beilage hinzu", modelEN: "Added.", modelDE: "Hinzugefügt.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, _ := e.twoDinners()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"add","plate_id":%d,"food_id":%d,"portions":1}]}`, p.ID, e.sides[0]))
			},
		},
		{
			name: "replace", userEN: "another main for the dinner", userDE: "ein anderes Hauptgericht fürs Abendessen", modelEN: "Here is a different main.", modelDE: "Hier ist ein anderes Hauptgericht.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, _ := e.twoDinners()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"replace","plate_id":%d,"position":"main","food_id":%d}]}`, p.ID, e.unusedFood("main")))
			},
		},
		{
			name: "cook_elsewhere", userEN: "cook it at breakfast time", userDE: "koche es zum Frühstück", modelEN: "The cooking moves earlier.", modelDE: "Das Kochen rückt nach vorn.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				v, p := e.laterMealSession()
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"move_cook","session_id":%d,"cook_at":%s}]}`, v.ID, cellJSON(p.DateString(), e.slotID("breakfast"))))
			},
		},
		{
			name: "lighter", userEN: "make the dinner lighter", userDE: "mach das Abendessen leichter", modelEN: "I made it lighter.", modelDE: "Ich habe es leichter gemacht.", write: true,
			prepare: func(t *testing.T) *planToolEnv { return fixEnv(t, 2600) },
			run: func(e *planToolEnv) json.RawMessage {
				id := e.dinner3()
				for _, p := range e.draftMeals() {
					if p.ID == id {
						return e.raw("apply_changes", fmt.Sprintf(`{"goal":{"kind":"lighter","plate_id":%d},"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.75}]}`, id, mainComponentID(e.t, p)))
					}
				}
				e.t.Fatal("no plate")
				return nil
			},
		},
		{
			name: "under_minimum", userEN: "halve the dinner", userDE: "halbiere das Abendessen", modelEN: "I did that.", modelDE: "Das habe ich gemacht.", write: true,
			prepare: func(t *testing.T) *planToolEnv { return fixEnv(t, 3600) },
			run: func(e *planToolEnv) json.RawMessage {
				id := e.dinner3()
				for _, p := range e.draftMeals() {
					if p.ID == id {
						return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.5}]}`, mainComponentID(e.t, p)))
					}
				}
				e.t.Fatal("no plate")
				return nil
			},
		},
		{
			name: "impossible", userEN: "halve the dinner", userDE: "halbiere das Abendessen", modelEN: "That does not work.", modelDE: "Das geht nicht.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				var a plate.Plate
				for _, p := range e.draftMeals() {
					if p.SlotID == e.slotID("dinner") {
						a = p
						break
					}
				}
				gp := e.mustExec("get_plan", fmt.Sprintf(`{"from":%q,"to":%q}`, a.DateString(), a.DateString()))
				var kcal float64
				for _, d := range gp["evaluation"].(map[string]any)["days"].([]any) {
					if m := d.(map[string]any); m["date"] == a.DateString() {
						kcal = m["kcal"].(float64)
					}
				}
				e.sql(`UPDATE user_profile SET kcal_target = ? WHERE id = 1`, (kcal-5)/0.75)
				return e.raw("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.5}]}`, mainComponentID(e.t, a)))
			},
		},
		{
			name: "undo", userEN: "undo that", userDE: "mach das rückgängig", modelEN: "It is back as it was.", modelDE: "Es ist wieder wie vorher.", write: true,
			run: func(e *planToolEnv) json.RawMessage {
				p, q := e.twoDinners()
				ctx, conv := e.chat()
				e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[{"op":"move","plate_id":%d,"to":%s,"occupant":"swap"}]}`, p.ID, cellJSON(q.DateString(), q.SlotID)))
				out, _, _, err := e.ts.Execute(agent.WithTurn(ctx, conv), "undo_last_change", json.RawMessage(`{}`))
				require.NoError(e.t, err)
				return out
			},
		},
		{
			name: "read_only", userEN: "what is on the plan?", userDE: "was steht auf dem Plan?", modelEN: "The week holds 9123 kcal.", modelDE: "Die Woche hat 9123 kcal.",
			run: func(e *planToolEnv) json.RawMessage { return e.raw("get_plan", `{}`) },
		},
		{
			name: "ambiguous", userEN: "make it lighter", userDE: "mach es leichter", modelEN: "Do you mean the Friday dinner?", modelDE: "Meinst du das Abendessen am Freitag?",
			staticOutput: `null`,
		},
	}
	got := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var out json.RawMessage
			if c.staticOutput == "" {
				e := relEnv(t)
				if c.prepare != nil {
					e = c.prepare(t)
				}
				out = c.run(e)
			}
			var reply [2]string
			for i, lg := range [][3]string{{c.userEN, c.modelEN}, {c.userDE, c.modelDE}} {
				var output json.RawMessage
				if out != nil {
					output = out
				}
				reply[i] = agent.ComposeReply(lg[1], lg[0], c.write, output)
				assert.NotEmpty(t, reply[i])
			}
			assert.NotEqual(t, reply[0], reply[1], "the two languages are not the same text")
			got[c.name] = reply[0] + "\n--- de ---\n" + reply[1]
		})
	}
	t.Cleanup(func() {
		path := "testdata/facts_golden.json"
		if *updateFacts {
			b, _ := json.MarshalIndent(got, "", "  ")
			require.NoError(t, os.WriteFile(path, append(b, '\n'), 0o644))
			return
		}
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var want map[string]string
		require.NoError(t, json.Unmarshal(raw, &want))
		for name, w := range want {
			assert.Equal(t, w, got[name], name)
		}
		assert.Len(t, got, len(want))
	})
}
