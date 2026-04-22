package template_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- fakes ---------------------------------------------------------------

type fakeTemplateRepo struct {
	templates map[int64]*template.Template
	next      int64
}

func newFakeTemplateRepo() *fakeTemplateRepo {
	return &fakeTemplateRepo{templates: map[int64]*template.Template{}, next: 1}
}

func (r *fakeTemplateRepo) Create(_ context.Context, t *template.Template) error {
	t.ID = r.next
	r.next++
	for i := range t.Components {
		t.Components[i].ID = int64(i) + 1
		t.Components[i].TemplateID = t.ID
	}
	cp := *t
	cp.Components = append([]template.TemplateComponent(nil), t.Components...)
	r.templates[t.ID] = &cp
	return nil
}

func (r *fakeTemplateRepo) Get(_ context.Context, id int64) (*template.Template, error) {
	t, ok := r.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	cp.Components = append([]template.TemplateComponent(nil), t.Components...)
	return &cp, nil
}

func (r *fakeTemplateRepo) UpdateName(_ context.Context, id int64, name string) (*template.Template, error) {
	t, ok := r.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	t.Name = name
	cp := *t
	cp.Components = append([]template.TemplateComponent(nil), t.Components...)
	return &cp, nil
}

func (r *fakeTemplateRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.templates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.templates, id)
	return nil
}

func (r *fakeTemplateRepo) List(_ context.Context) ([]template.Template, error) {
	out := make([]template.Template, 0, len(r.templates))
	for _, t := range r.templates {
		cp := *t
		cp.Components = append([]template.TemplateComponent(nil), t.Components...)
		out = append(out, cp)
	}
	return out, nil
}

func (r *fakeTemplateRepo) ReplaceComponents(_ context.Context, templateID int64, comps []template.TemplateComponent) error {
	t, ok := r.templates[templateID]
	if !ok {
		return domain.ErrNotFound
	}
	t.Components = append([]template.TemplateComponent(nil), comps...)
	for i := range t.Components {
		t.Components[i].TemplateID = templateID
		t.Components[i].ID = int64(i) + 1
	}
	return nil
}

func (r *fakeTemplateRepo) ListComponentsByTemplate(_ context.Context, templateID int64) ([]template.TemplateComponent, error) {
	t, ok := r.templates[templateID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return append([]template.TemplateComponent(nil), t.Components...), nil
}

func (r *fakeTemplateRepo) CountUsingComponent(_ context.Context, componentID int64) (int64, error) {
	var n int64
	for _, t := range r.templates {
		for _, c := range t.Components {
			if c.ComponentID == componentID {
				n++
			}
		}
	}
	return n, nil
}

type fakeComponentChecker struct {
	existing map[int64]bool
}

func (c *fakeComponentChecker) Exists(_ context.Context, id int64) (bool, error) {
	return c.existing[id], nil
}

type fakePlateSource struct {
	plates map[int64][]plate.PlateComponent
}

func (p *fakePlateSource) ListComponentsByPlate(_ context.Context, plateID int64) ([]plate.PlateComponent, error) {
	pcs, ok := p.plates[plateID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return append([]plate.PlateComponent(nil), pcs...), nil
}

// fakePlateRepo is a tiny plate.Repository used inside the transactional Apply
// flow. Only the methods Apply() touches are real; the rest panic.
type fakePlateRepo struct {
	plates map[int64]*plate.Plate
	pcs    map[int64]*plate.PlateComponent
	nextPC int64
}

func (r *fakePlateRepo) Get(_ context.Context, id int64) (*plate.Plate, error) {
	p, ok := r.plates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	cp.Components = nil
	for _, pc := range r.pcs {
		if pc.PlateID == id {
			cp.Components = append(cp.Components, *pc)
		}
	}
	return &cp, nil
}

func (r *fakePlateRepo) CreateComponent(_ context.Context, pc *plate.PlateComponent) error {
	r.nextPC++
	pc.ID = r.nextPC
	cp := *pc
	r.pcs[pc.ID] = &cp
	return nil
}

func (r *fakePlateRepo) DeleteComponent(_ context.Context, id int64) error {
	if _, ok := r.pcs[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.pcs, id)
	return nil
}

// --- plate.Repository methods not needed by Apply: panic if touched ---

func (r *fakePlateRepo) Create(context.Context, *plate.Plate) error { panic("not used") }
func (r *fakePlateRepo) Update(context.Context, *plate.Plate) error { panic("not used") }
func (r *fakePlateRepo) Delete(context.Context, int64) error        { panic("not used") }
func (r *fakePlateRepo) ListByWeek(context.Context, int64) ([]plate.Plate, error) {
	panic("not used")
}

func (r *fakePlateRepo) SetSkipped(context.Context, int64, bool, *string) (*plate.Plate, error) {
	panic("not used")
}

func (r *fakePlateRepo) DeleteByWeek(context.Context, int64) (int64, error) {
	panic("not used")
}

func (r *fakePlateRepo) GetComponent(context.Context, int64) (*plate.PlateComponent, error) {
	panic("not used")
}

func (r *fakePlateRepo) UpdateComponent(context.Context, *plate.PlateComponent) error {
	panic("not used")
}

func (r *fakePlateRepo) ListComponentsByPlate(_ context.Context, plateID int64) ([]plate.PlateComponent, error) {
	var out []plate.PlateComponent
	for _, pc := range r.pcs {
		if pc.PlateID == plateID {
			out = append(out, *pc)
		}
	}
	return out, nil
}

func (r *fakePlateRepo) CountUsingComponent(context.Context, int64) (int64, error) {
	panic("not used")
}

func (r *fakePlateRepo) CountUsingTimeSlot(context.Context, int64) (int64, error) {
	panic("not used")
}

type fakeTxRunner struct {
	tr template.Repository
	pr plate.Repository
}

func (t *fakeTxRunner) RunInTemplateTx(ctx context.Context, fn func(template.Repository, plate.Repository) error) error {
	return fn(t.tr, t.pr)
}

// ---- setup helper --------------------------------------------------------

type harness struct {
	svc    *template.Service
	tRepo  *fakeTemplateRepo
	comps  *fakeComponentChecker
	src    *fakePlateSource
	plates *fakePlateRepo
}

func newHarness() *harness {
	tRepo := newFakeTemplateRepo()
	comps := &fakeComponentChecker{existing: map[int64]bool{10: true, 20: true, 30: true}}
	src := &fakePlateSource{plates: map[int64][]plate.PlateComponent{}}
	plates := &fakePlateRepo{
		plates: map[int64]*plate.Plate{},
		pcs:    map[int64]*plate.PlateComponent{},
	}
	tx := &fakeTxRunner{tr: tRepo, pr: plates}
	return &harness{
		svc:    template.NewService(tRepo, comps, src, tx),
		tRepo:  tRepo,
		comps:  comps,
		src:    src,
		plates: plates,
	}
}

// ---- tests ---------------------------------------------------------------

func TestCreate_EmptyTemplate(t *testing.T) {
	h := newHarness()

	tpl, err := h.svc.Create(context.Background(), "Empty", nil, nil)

	require.NoError(t, err)
	assert.Equal(t, int64(1), tpl.ID)
	assert.Equal(t, "Empty", tpl.Name)
	assert.Empty(t, tpl.Components)
}

func TestCreate_TrimsAndRejectsEmptyName(t *testing.T) {
	h := newHarness()

	_, err := h.svc.Create(context.Background(), "   ", nil, nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestCreate_RejectsBothSources(t *testing.T) {
	h := newHarness()
	id := int64(99)

	_, err := h.svc.Create(context.Background(), "X", &id,
		[]template.TemplateComponent{{ComponentID: 10, Portions: 1}})

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestCreate_FromInlineComponents(t *testing.T) {
	h := newHarness()

	tpl, err := h.svc.Create(context.Background(), "Curry Night", nil, []template.TemplateComponent{
		{ComponentID: 10, Portions: 2},
		{ComponentID: 20, Portions: 1},
	})

	require.NoError(t, err)
	require.Len(t, tpl.Components, 2)
	assert.Equal(t, int64(10), tpl.Components[0].ComponentID)
	assert.Equal(t, 2.0, tpl.Components[0].Portions)
	assert.Equal(t, 0, tpl.Components[0].SortOrder)
	assert.Equal(t, 1, tpl.Components[1].SortOrder)
}

func TestCreate_InlineDefaultPortionsToOne(t *testing.T) {
	h := newHarness()

	tpl, err := h.svc.Create(context.Background(), "X", nil, []template.TemplateComponent{
		{ComponentID: 10, Portions: 0},
	})

	require.NoError(t, err)
	assert.Equal(t, 1.0, tpl.Components[0].Portions)
}

func TestCreate_InlineRejectsUnknownComponent(t *testing.T) {
	h := newHarness()

	_, err := h.svc.Create(context.Background(), "X", nil, []template.TemplateComponent{
		{ComponentID: 999, Portions: 1},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestCreate_FromPlate(t *testing.T) {
	h := newHarness()
	h.src.plates[42] = []plate.PlateComponent{
		{ID: 1, PlateID: 42, ComponentID: 10, Portions: 2, SortOrder: 0},
		{ID: 2, PlateID: 42, ComponentID: 20, Portions: 1, SortOrder: 1},
	}
	id := int64(42)

	tpl, err := h.svc.Create(context.Background(), "Curry From Plate", &id, nil)

	require.NoError(t, err)
	require.Len(t, tpl.Components, 2)
	assert.Equal(t, int64(10), tpl.Components[0].ComponentID)
	assert.Equal(t, 2.0, tpl.Components[0].Portions)
	assert.Equal(t, int64(20), tpl.Components[1].ComponentID)
}

func TestCreate_FromPlateMissing(t *testing.T) {
	h := newHarness()
	id := int64(404)

	_, err := h.svc.Create(context.Background(), "X", &id, nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestUpdateName(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "Old", nil, nil)
	require.NoError(t, err)

	updated, err := h.svc.UpdateName(context.Background(), tpl.ID, "  New  ")

	require.NoError(t, err)
	assert.Equal(t, "New", updated.Name)
}

func TestUpdateName_EmptyRejected(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "Old", nil, nil)
	require.NoError(t, err)

	_, err = h.svc.UpdateName(context.Background(), tpl.ID, "   ")

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestDelete(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "X", nil, nil)
	require.NoError(t, err)

	require.NoError(t, h.svc.Delete(context.Background(), tpl.ID))

	_, err = h.svc.Get(context.Background(), tpl.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestApply_Replace(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "Set", nil, []template.TemplateComponent{
		{ComponentID: 10, Portions: 2},
		{ComponentID: 20, Portions: 1},
	})
	require.NoError(t, err)

	h.plates.plates[7] = &plate.Plate{ID: 7}
	h.plates.pcs[101] = &plate.PlateComponent{ID: 101, PlateID: 7, ComponentID: 30, Portions: 3, SortOrder: 0}

	err = h.svc.Apply(context.Background(), tpl.ID, 7, false)
	require.NoError(t, err)

	p, err := h.plates.Get(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, p.Components, 2)

	// Assert the original component 30 is gone; template's 10 and 20 present.
	var ids []int64
	for _, pc := range p.Components {
		ids = append(ids, pc.ComponentID)
	}
	assert.ElementsMatch(t, []int64{10, 20}, ids)
}

func TestApply_Merge_PreservesOrder(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "Set", nil, []template.TemplateComponent{
		{ComponentID: 10, Portions: 1},
		{ComponentID: 20, Portions: 1},
	})
	require.NoError(t, err)

	h.plates.plates[7] = &plate.Plate{ID: 7}
	h.plates.pcs[201] = &plate.PlateComponent{ID: 201, PlateID: 7, ComponentID: 30, Portions: 1, SortOrder: 0}
	h.plates.nextPC = 201

	err = h.svc.Apply(context.Background(), tpl.ID, 7, true)
	require.NoError(t, err)

	p, err := h.plates.Get(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, p.Components, 3)

	// existing 30 stays at 0; template components appended at 1, 2.
	byComp := map[int64]int{}
	for _, pc := range p.Components {
		byComp[pc.ComponentID] = pc.SortOrder
	}
	assert.Equal(t, 0, byComp[30])
	assert.Equal(t, 1, byComp[10])
	assert.Equal(t, 2, byComp[20])
}

func TestApply_MissingTemplate(t *testing.T) {
	h := newHarness()
	h.plates.plates[7] = &plate.Plate{ID: 7}

	err := h.svc.Apply(context.Background(), 999, 7, false)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestApply_MissingPlate(t *testing.T) {
	h := newHarness()
	tpl, err := h.svc.Create(context.Background(), "X", nil, nil)
	require.NoError(t, err)

	err = h.svc.Apply(context.Background(), tpl.ID, 404, false)

	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}
