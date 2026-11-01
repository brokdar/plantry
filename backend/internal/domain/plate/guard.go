package plate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

// The draft guard (Phase 4 D5): the agent changes drafts only. Under
// actor.Agent every write that would change or delete a confirmed plate or
// cook session, or a draft linked to a confirmed session, fails with
// domain.ErrRowConfirmed before it writes anything. The error names the
// confirmed rows so the model can tell the user. Confirm and Unlock are not
// guarded (Unlock is the gate, D6).

// byAgent reports whether ctx's actor is the agent.
func byAgent(ctx context.Context) bool { return actor.From(ctx).Kind == actor.Agent }

// guardPlates fails under the agent when a plate, or a session one of its
// components eats from, is confirmed.
func (s *Service) guardPlates(ctx context.Context, plates ...Plate) error {
	if !byAgent(ctx) {
		return nil
	}
	var rows []string
	seen := map[int64]bool{}
	for _, p := range plates {
		if p.State == StateConfirmed {
			rows = append(rows, describePlate(p))
		}
		for _, sid := range linkedSessions(p.Components) {
			if seen[sid] {
				continue
			}
			seen[sid] = true
			sess, err := s.sessions.GetSession(ctx, sid)
			if err != nil {
				return err
			}
			if sess.State == StateConfirmed {
				row, err := s.describeSession(ctx, *sess)
				if err != nil {
					return err
				}
				rows = append(rows, row)
			}
		}
	}
	return rowsConfirmed(rows)
}

// guardSession fails under the agent when sess is confirmed.
func (s *Service) guardSession(ctx context.Context, sess CookSession) error {
	if !byAgent(ctx) || sess.State != StateConfirmed {
		return nil
	}
	row, err := s.describeSession(ctx, sess)
	if err != nil {
		return err
	}
	return rowsConfirmed([]string{row})
}

func rowsConfirmed(rows []string) error {
	if len(rows) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s; ask the user to unlock first", domain.ErrRowConfirmed, strings.Join(rows, "; "))
}

// describePlate is "plate 4 (2026-11-01 slot 2: Chili, Rice)".
func describePlate(p Plate) string {
	names := make([]string, len(p.Components))
	for i, c := range p.Components {
		names[i] = c.FoodName
	}
	return fmt.Sprintf("plate %d (%s slot %d: %s)", p.ID, p.DateString(), p.SlotID, strings.Join(names, ", "))
}

// describeSession is "session 7 (Chili, cooks 2026-11-01 slot 2)".
func (s *Service) describeSession(ctx context.Context, sess CookSession) (string, error) {
	f, err := s.foods.Get(ctx, sess.FoodID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("session %d (%s, cooks %s slot %d)", sess.ID, f.Name, sess.CookDate.Format(dateLayout), sess.CookSlotID), nil
}
