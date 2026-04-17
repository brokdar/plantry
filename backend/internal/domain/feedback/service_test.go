package feedback_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// --- fakes ---

type fakeFeedbackRepo struct {
	items map[int64]*feedback.PlateFeedback
	fail  error
}

func newFakeFeedbackRepo() *fakeFeedbackRepo {
	return &fakeFeedbackRepo{items: map[int64]*feedback.PlateFeedback{}}
}

func (r *fakeFeedbackRepo) Upsert(_ context.Context, f *feedback.PlateFeedback) error {
	if r.fail != nil {
		return r.fail
	}
	f.RatedAt = time.Unix(0, 0).UTC()
	clone := *f
	r.items[f.PlateID] = &clone
	return nil
}

func (r *fakeFeedbackRepo) Get(_ context.Context, id int64) (*feedback.PlateFeedback, error) {
	f, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", domain.ErrNotFound, id)
	}
	clone := *f
	return &clone, nil
}

func (r *fakeFeedbackRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return fmt.Errorf("%w: %d", domain.ErrNotFound, id)
	}
	delete(r.items, id)
	return nil
}

func (r *fakeFeedbackRepo) ListByWeek(_ context.Context, _ int64) ([]feedback.PlateFeedback, error) {
	return nil, nil
}

type fakeComponentRepo struct {
	items     map[int64]*component.Component
	markFail  error
	markCalls []int64
}

func (r *fakeComponentRepo) Create(_ context.Context, _ *component.Component) error { return nil }
func (r *fakeComponentRepo) Update(_ context.Context, _ *component.Component) error { return nil }
func (r *fakeComponentRepo) Delete(_ context.Context, _ int64) error                { return nil }
func (r *fakeComponentRepo) Get(_ context.Context, id int64) (*component.Component, error) {
	c, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", domain.ErrNotFound, id)
	}
	clone := *c
	clone.Tags = append([]string(nil), c.Tags...)
	return &clone, nil
}

func (r *fakeComponentRepo) List(_ context.Context, _ component.ListQuery) (*component.ListResult, error) {
	return nil, nil
}

func (r *fakeComponentRepo) CreateVariantGroup(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (r *fakeComponentRepo) Siblings(_ context.Context, _, _ int64) ([]component.Component, error) {
	return nil, nil
}

func (r *fakeComponentRepo) MarkCooked(_ context.Context, id int64, at time.Time) error {
	if r.markFail != nil {
		return r.markFail
	}
	r.markCalls = append(r.markCalls, id)
	if c, ok := r.items[id]; ok {
		c.CookCount++
		t := at
		c.LastCookedAt = &t
	}
	return nil
}

type fakeProfileRepo struct {
	p     *profile.Profile
	calls int
}

func (r *fakeProfileRepo) Get(_ context.Context) (*profile.Profile, error) {
	return r.p, nil
}

func (r *fakeProfileRepo) Update(_ context.Context, p *profile.Profile) (*profile.Profile, error) {
	r.calls++
	r.p = p
	return p, nil
}

type fakePlateRepo struct {
	plates map[int64]*plate.Plate
}

func (r *fakePlateRepo) Get(_ context.Context, id int64) (*plate.Plate, error) {
	p, ok := r.plates[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", domain.ErrNotFound, id)
	}
	clone := *p
	return &clone, nil
}

type inlineTxRunner struct {
	fb   feedback.Repository
	comp component.Repository
	prof profile.Repository
	fail error
}

func (t *inlineTxRunner) RunInFeedbackTx(ctx context.Context, fn func(feedback.Repository, component.Repository, profile.Repository) error) error {
	if t.fail != nil {
		return t.fail
	}
	return fn(t.fb, t.comp, t.prof)
}

// --- fixture ---

type fixture struct {
	svc    *feedback.Service
	tx     *inlineTxRunner
	fb     *fakeFeedbackRepo
	comp   *fakeComponentRepo
	prof   *fakeProfileRepo
	plates *fakePlateRepo
	plate  *plate.Plate
}

func newFixture(components []*component.Component, plateComponentIDs []int64) *fixture {
	fbRepo := newFakeFeedbackRepo()
	compRepo := &fakeComponentRepo{items: make(map[int64]*component.Component)}
	for _, c := range components {
		compRepo.items[c.ID] = c
	}
	prof := &fakeProfileRepo{p: &profile.Profile{Preferences: map[string]any{}}}

	pl := &plate.Plate{ID: 42, WeekID: 1, Day: 0, SlotID: 1}
	for i, cid := range plateComponentIDs {
		pl.Components = append(pl.Components, plate.PlateComponent{
			ID: int64(100 + i), PlateID: 42, ComponentID: cid, Portions: 1, SortOrder: i,
		})
	}
	plates := &fakePlateRepo{plates: map[int64]*plate.Plate{42: pl}}

	tx := &inlineTxRunner{fb: fbRepo, comp: compRepo, prof: prof}
	svc := feedback.NewService(tx, plates, compRepo)
	return &fixture{svc: svc, tx: tx, fb: fbRepo, comp: compRepo, prof: prof, plates: plates, plate: pl}
}

// --- tests ---

func TestRecordFeedback_InvalidStatus(t *testing.T) {
	f := newFixture(nil, nil)
	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.Status("bogus"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidFeedbackStatus))
}

func TestRecordFeedback_CookedIncrementsEachComponentOnce(t *testing.T) {
	comps := []*component.Component{
		{ID: 1, Name: "Curry"},
		{ID: 2, Name: "Rice"},
	}
	f := newFixture(comps, []int64{1, 2})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2}, f.comp.markCalls)

	// Re-marking cooked is a no-op — cook_count does NOT double.
	_, err = f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2}, f.comp.markCalls, "re-marking should not increment again")
}

func TestRecordFeedback_CookedToLovedNoDoubleIncrement(t *testing.T) {
	comps := []*component.Component{{ID: 1, Name: "Curry"}}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.NoError(t, err)
	assert.Len(t, f.comp.markCalls, 1)

	_, err = f.svc.RecordFeedback(context.Background(), 42, feedback.StatusLoved, nil)
	require.NoError(t, err)
	assert.Len(t, f.comp.markCalls, 1, "cooked→loved should not re-increment")
}

func TestRecordFeedback_SkippedDoesNotTouchComponents(t *testing.T) {
	comps := []*component.Component{{ID: 1, Name: "Curry"}}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusSkipped, nil)
	require.NoError(t, err)
	assert.Empty(t, f.comp.markCalls)
}

func TestRecordFeedback_LovedAppendsTagsToLikes(t *testing.T) {
	comps := []*component.Component{
		{ID: 1, Name: "Curry", Tags: []string{"spicy", "thai"}},
		{ID: 2, Name: "Rice", Tags: []string{"thai", "quick"}},
	}
	f := newFixture(comps, []int64{1, 2})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusLoved, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, f.prof.calls)

	likes := f.prof.p.Preferences["likes"].([]string)
	assert.ElementsMatch(t, []string{"spicy", "thai", "quick"}, likes)
}

func TestRecordFeedback_DislikedAppendsTagsToDislikes(t *testing.T) {
	comps := []*component.Component{
		{ID: 1, Name: "Mushrooms", Tags: []string{"fungi"}},
	}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusDisliked, nil)
	require.NoError(t, err)

	dislikes := f.prof.p.Preferences["dislikes"].([]string)
	assert.Equal(t, []string{"fungi"}, dislikes)
}

func TestRecordFeedback_LovedWithNoTagsSkipsProfileWrite(t *testing.T) {
	comps := []*component.Component{{ID: 1, Name: "Plain rice"}}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusLoved, nil)
	require.NoError(t, err)
	assert.Zero(t, f.prof.calls)
}

func TestRecordFeedback_CookedDoesNotTouchPreferences(t *testing.T) {
	comps := []*component.Component{
		{ID: 1, Name: "Curry", Tags: []string{"spicy"}},
	}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.NoError(t, err)
	assert.Zero(t, f.prof.calls)
}

func TestRecordFeedback_NotePersisted(t *testing.T) {
	comps := []*component.Component{{ID: 1, Name: "Curry"}}
	f := newFixture(comps, []int64{1})

	note := "extra cilantro"
	fb, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, &note)
	require.NoError(t, err)
	require.NotNil(t, fb.Note)
	assert.Equal(t, "extra cilantro", *fb.Note)
}

func TestRecordFeedback_UnknownPlateReturnsError(t *testing.T) {
	f := newFixture(nil, nil)
	// Remove the seeded plate to force a miss.
	delete(f.plates.plates, 42)
	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestRecordFeedback_MarkCookedFailureLeavesNoFeedback(t *testing.T) {
	// inlineTxRunner runs without true tx isolation; simulate by having the
	// MarkCooked failure surface through the closure and assert the resulting
	// error propagates. For true rollback semantics the sqlite test covers it.
	comps := []*component.Component{{ID: 1, Name: "Curry"}}
	f := newFixture(comps, []int64{1})
	f.comp.markFail = errors.New("boom")

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestDeleteFeedback_RemovesRow(t *testing.T) {
	comps := []*component.Component{{ID: 1, Name: "Curry"}}
	f := newFixture(comps, []int64{1})

	_, err := f.svc.RecordFeedback(context.Background(), 42, feedback.StatusCooked, nil)
	require.NoError(t, err)

	require.NoError(t, f.svc.DeleteFeedback(context.Background(), 42))
	_, err = f.fb.Get(context.Background(), 42)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestDeleteFeedback_UnknownIsError(t *testing.T) {
	f := newFixture(nil, nil)
	err := f.svc.DeleteFeedback(context.Background(), 42)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}
