package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// moveCookBlocked is the plate service's refusal of a move_cook: "op 1 of 1
// (move_cook): invalid input: plate 93 (2026-12-01 slot 2) comes before the
// cook; ...".
var (
	moveCookBlocked = regexp.MustCompile(`^op 1 of 1 \(move_cook\): invalid input: (.+)$`)
	// moveCookCell parses one blocking meal; its reasons are plate's constants.
	moveCookCell = regexp.MustCompile(`\((\d{4}-\d{2}-\d{2}) slot (\d+)\) (` + regexp.QuoteMeta(plate.BlockComesBefore) + `|` + regexp.QuoteMeta(plate.BlockPastKeeping) + `)`)
)

// moveCookRefusal turns a single move_cook the plate service refused (a meal
// of the batch would come before the new cooking time, or past its keeping
// time) into a refused result: nothing saved, and the "Left out: ..." line
// with the meals that block it, rendered in code (V81, tune-v2c
// g2-cook_elsewhere-07 and h2-cookref-00: the refusal came back as a tool
// error, so the reply had no code line and the model's prose missed "nothing
// changed" or the reason). ok false for any other error.
func moveCookRefusal(ctx context.Context, svc Services, w window.Window, op changeOp, dryRun bool, err error) (map[string]any, bool) {
	m := moveCookBlocked.FindStringSubmatch(err.Error())
	if m == nil || op.Op != "move_cook" || op.CookAt == nil {
		return nil, false
	}
	cells := moveCookCell.FindAllStringSubmatch(m[1], -1)
	if len(cells) == 0 {
		return nil, false
	}
	dish, slotName := "", map[string]string{}
	if svc.State != nil {
		if st, serr := svc.State(ctx, w, todayFrom(ctx)); serr == nil {
			for _, s := range st.Sessions {
				if s.ID == op.SessionID {
					dish = foodName(st, s.FoodID)
				}
			}
			for id, n := range st.SlotNames {
				slotName[fmt.Sprint(id)] = strings.TrimPrefix(n, "slot.")
			}
		}
	}
	at := func(date, slot string) (string, string) {
		day := dayLabelStr(date)
		if n := slotName[slot]; n != "" {
			return day + " " + n, deDay(day) + " " + deSlot(n)
		}
		return day, deDay(day)
	}
	toEN, toDE := at(op.CookAt.Date, fmt.Sprint(op.CookAt.SlotID))
	var before, beforeDE, late, lateDE []string
	seen := map[string]bool{} // a meal blocks once, whatever its links say
	for _, c := range cells {
		if k := c[1] + "|" + c[2]; seen[k] {
			continue
		} else {
			seen[k] = true
		}
		en, de := at(c[1], c[2])
		if c[3] == plate.BlockComesBefore {
			before, beforeDE = append(before, en), append(beforeDE, de)
		} else {
			late, lateDE = append(late, en), append(lateDE, de)
		}
	}
	whatEN, whatDE := "cooking it on "+toEN, "es am "+toDE+" kochen"
	if dish != "" {
		whatEN = fmt.Sprintf("cooking %s on %s", food.Quote(dish), toEN)
		whatDE = fmt.Sprintf("„%s“ am %s kochen", dish, toDE)
	}
	var why, whyDE []string
	if len(before) > 0 {
		why = append(why, joinAnd(before, "and")+" eat it before then")
		whyDE = append(whyDE, joinAnd(beforeDE, "und")+" essen schon vorher davon")
	}
	if len(late) > 0 {
		why = append(why, joinAnd(late, "and")+" would eat it past its keeping time")
		whyDE = append(whyDE, joinAnd(lateDE, "und")+" würden es nach seiner Haltbarkeit essen")
	}
	whyEN, whyDEs := strings.Join(why, "; "), strings.Join(whyDE, "; ")
	entry := map[string]any{"op": 1, "what": whatEN, "why": whyEN, "why_de": whyDEs}
	for k, v := range refusedFact(whatEN, whatDE, whyEN, whyDEs) {
		entry[k] = v
	}
	res := map[string]any{
		"dry_run": dryRun, "saved": false, "applied": []any{}, "refused": whyEN,
		"skipped": []map[string]any{entry},
		"next":    "nothing was saved (left out: " + whatEN + " (" + whyEN + ")); the program tells what was left out and why: apply nothing more unasked",
	}
	res["saved"], res["status"] = saveStatus(res)
	return res, true
}

// joinAnd lists items as "a, b and c".
func joinAnd(items []string, and string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + and + " " + items[len(items)-1]
}
