package discovery_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func newStore(t *testing.T) (*discovery.Store, *sqlite.RecipeSuggestionRepo, string) {
	t.Helper()
	repo := sqlite.NewRecipeSuggestionRepo(testhelper.NewTestDB(t))
	dir := t.TempDir()
	images, err := imagestore.New(dir, nil)
	require.NoError(t, err)
	return &discovery.Store{Repo: repo, Images: images}, repo, dir
}

// FIND-14: a site's photo (a draft's image, e.g. from og:image) is never
// downloaded for a suggestion, even when the page names a reachable one.
func TestStore_SavesRowWithoutImage(t *testing.T) {
	store, repo, dir := newStore(t)
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
	t.Cleanup(srv.Close)

	sug, err := store.Save(context.Background(), discovery.NewSuggestion{
		Mode:  discovery.ModeDaily,
		Draft: &importer.Draft{Name: "Fesenjan", SourceURL: "https://x.test/f", ImageURL: srv.URL + "/ok.png"},
	})
	require.NoError(t, err)
	assert.Nil(t, sug.ImagePath)
	assert.False(t, hit, "the page's image is never requested")
	assert.NoDirExists(t, filepath.Join(dir, "suggestions"))

	got, err := repo.Get(context.Background(), sug.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://x.test/f", got.SourceURL)
	assert.Equal(t, discovery.StatusPending, got.Status)
	assert.Nil(t, got.ImagePath)
}

// B4-AC6: the store-level decisions B5 uses move the run's outcome (D11);
// a reject removes the image (D16).
func TestStore_AcceptRejectMoveOutcome(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewRecipeSuggestionRepo(db)
	dir := t.TempDir()
	images, err := imagestore.New(dir, nil)
	require.NoError(t, err)
	store := &discovery.Store{Repo: repo, Images: images}
	ctx := context.Background()

	res, err := db.Exec(`INSERT INTO ai_runs (kind, started_at, status) VALUES ('discovery', datetime('now'), 'ok')`)
	require.NoError(t, err)
	runID, _ := res.LastInsertId()
	outcome := func() string {
		var o *string
		require.NoError(t, db.QueryRow(`SELECT outcome FROM ai_runs WHERE id = ?`, runID).Scan(&o))
		if o == nil {
			return "NULL"
		}
		return *o
	}
	save := func(url string) *discovery.Suggestion {
		s, err := store.Save(ctx, discovery.NewSuggestion{
			Mode: discovery.ModeDaily, RunID: &runID,
			Draft: &importer.Draft{SourceURL: url},
		})
		require.NoError(t, err)
		// A suggestion saved before Discover stopped copying photos.
		path := fmt.Sprintf("suggestions/%d.jpg", s.ID)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "suggestions"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("x"), 0o600))
		require.NoError(t, repo.SetImagePath(ctx, s.ID, &path))
		s.ImagePath = &path
		return s
	}
	a, b := save("https://x.test/a"), save("https://x.test/b")
	assert.Equal(t, "NULL", outcome())

	taste := discovery.ReasonTaste
	got, err := store.Reject(ctx, a.ID, &taste)
	require.NoError(t, err)
	assert.Equal(t, discovery.StatusRejected, got.Status)
	assert.Nil(t, got.ImagePath)
	assert.NoFileExists(t, filepath.Join(dir, *a.ImagePath))
	stored, err := repo.Get(ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.ImagePath, "image_path cleared in the DB")
	assert.Equal(t, "open", outcome())

	res, err = db.Exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Saved', 'composed', 'main', 2)`)
	require.NoError(t, err)
	foodID, _ := res.LastInsertId()
	got, err = store.Accept(ctx, b.ID, foodID)
	require.NoError(t, err)
	assert.Equal(t, discovery.StatusAccepted, got.Status)
	assert.FileExists(t, filepath.Join(dir, *b.ImagePath), "accept keeps the image for the copy")
	assert.Equal(t, "accepted", outcome())

	_, err = store.Reject(ctx, b.ID, nil)
	assert.ErrorIs(t, err, domain.ErrDiscoveryDecided)

	rej, err := store.Rejected(ctx, 20)
	require.NoError(t, err)
	require.Len(t, rej, 1)
	assert.Equal(t, a.ID, rej[0].ID)
}
