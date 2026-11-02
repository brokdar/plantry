package discovery_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 20))))
	return buf.Bytes()
}

func newStore(t *testing.T) (*discovery.Store, *sqlite.RecipeSuggestionRepo, string) {
	t.Helper()
	repo := sqlite.NewRecipeSuggestionRepo(testhelper.NewTestDB(t))
	dir := t.TempDir()
	images, err := imagestore.New(dir, nil)
	require.NoError(t, err)
	return &discovery.Store{Repo: repo, Images: images}, repo, dir
}

func imageServer(t *testing.T) *httptest.Server {
	t.Helper()
	data := pngBytes(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestStore_SavesRowAndImage(t *testing.T) {
	store, repo, dir := newStore(t)
	srv := imageServer(t)

	sug, err := store.Save(context.Background(), discovery.NewSuggestion{
		Mode:  discovery.ModeDaily,
		Draft: &importer.Draft{Name: "Fesenjan", SourceURL: "https://x.test/f", ImageURL: srv.URL + "/ok.png"},
	})
	require.NoError(t, err)

	want := fmt.Sprintf("suggestions/%d.jpg", sug.ID)
	require.NotNil(t, sug.ImagePath)
	assert.Equal(t, want, *sug.ImagePath)
	assert.FileExists(t, filepath.Join(dir, want))

	got, err := repo.Get(context.Background(), sug.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://x.test/f", got.SourceURL)
	assert.Equal(t, discovery.StatusPending, got.Status)
	require.NotNil(t, got.ImagePath)
	assert.Equal(t, want, *got.ImagePath)
}

func TestStore_ImageFallsBackToOGImage(t *testing.T) {
	store, _, dir := newStore(t)
	srv := imageServer(t)
	page := `<html><head><meta property="og:image" content="` + srv.URL + `/ok.png">` +
		`<script type="application/ld+json">{"@type":"Recipe","name":"X","recipeIngredient":["1 Ei"]}</script></head></html>`
	draft, err := importer.NewService(nil, jsonld.Extractor{}, nil, nil, nil).
		Extract(context.Background(), importer.ExtractInput{HTML: page})
	require.NoError(t, err)
	require.Equal(t, srv.URL+"/ok.png", draft.ImageURL)
	draft.SourceURL = "https://x.test/og"

	sug, err := store.Save(context.Background(), discovery.NewSuggestion{Mode: discovery.ModeKnown, Draft: draft})
	require.NoError(t, err)
	require.NotNil(t, sug.ImagePath)
	assert.FileExists(t, filepath.Join(dir, *sug.ImagePath))
}

func TestStore_FailedImageLeavesRowStored(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	tests := map[string]string{
		"download fails": srv.URL + "/missing.png",
		"no image":       "",
		"unreachable":    "http://127.0.0.1:1/x",
	}
	for name, imageURL := range tests {
		t.Run(name, func(t *testing.T) {
			store, repo, dir := newStore(t)
			sug, err := store.Save(context.Background(), discovery.NewSuggestion{
				Mode: discovery.ModeExplore, Draft: &importer.Draft{SourceURL: "https://x.test/" + name, ImageURL: imageURL},
			})
			require.NoError(t, err)
			assert.Nil(t, sug.ImagePath)

			got, err := repo.Get(context.Background(), sug.ID)
			require.NoError(t, err)
			assert.Nil(t, got.ImagePath, "image_path stays NULL")
			assert.NoFileExists(t, filepath.Join(dir, "suggestions", fmt.Sprintf("%d.jpg", sug.ID)))
		})
	}
}

func TestStore_ImageGuardSkipsImage(t *testing.T) {
	store, repo, _ := newStore(t)
	srv := imageServer(t)
	store.ImageGuard = func(string) error { return errors.New("blocked") }

	sug, err := store.Save(context.Background(), discovery.NewSuggestion{
		Mode: discovery.ModeDaily, Draft: &importer.Draft{SourceURL: "https://x.test/g", ImageURL: srv.URL + "/ok.png"},
	})
	require.NoError(t, err)
	got, err := repo.Get(context.Background(), sug.ID)
	require.NoError(t, err)
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
	srv := imageServer(t)
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
			Draft: &importer.Draft{SourceURL: url, ImageURL: srv.URL + "/ok.png"},
		})
		require.NoError(t, err)
		require.NotNil(t, s.ImagePath)
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
