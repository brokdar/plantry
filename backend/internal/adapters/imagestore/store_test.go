package imagestore_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
)

func createTestImage(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	require.NoError(t, err)
	return buf.Bytes()
}

func serveImage(t *testing.T, data []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSaveFromURL(t *testing.T) {
	data := createTestImage(t, 100, 100)
	srv := serveImage(t, data)

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)

	relPath, err := store.SaveFromURL(context.Background(), srv.URL+"/image.png", "ingredients", 42)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("ingredients", "42.jpg"), relPath)

	absPath := filepath.Join(store.BasePath(), relPath)
	_, err = os.Stat(absPath)
	require.NoError(t, err)

	// Verify it's a valid JPEG.
	f, err := os.Open(absPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	cfg, format, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, "jpeg", format)
	assert.Equal(t, 100, cfg.Width)
	assert.Equal(t, 100, cfg.Height)
}

func TestSaveFromURL_LargeImage(t *testing.T) {
	data := createTestImage(t, 2000, 1500)
	srv := serveImage(t, data)

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)

	relPath, err := store.SaveFromURL(context.Background(), srv.URL+"/big.png", "recipes", 1)
	require.NoError(t, err)

	f, err := os.Open(filepath.Join(store.BasePath(), relPath))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	require.NoError(t, err)

	// Long edge should be capped at 1200, aspect ratio preserved.
	assert.LessOrEqual(t, cfg.Width, 1200)
	assert.LessOrEqual(t, cfg.Height, 1200)
	assert.Equal(t, 1200, cfg.Width) // 2000 is the long edge → scaled to 1200

	// Aspect ratio: 2000:1500 = 4:3, so height = 1200 * 1500/2000 = 900.
	assert.Equal(t, 900, cfg.Height)
}

func TestSaveFromURL_SmallImage(t *testing.T) {
	data := createTestImage(t, 200, 150)
	srv := serveImage(t, data)

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)

	relPath, err := store.SaveFromURL(context.Background(), srv.URL+"/small.png", "ingredients", 5)
	require.NoError(t, err)

	f, err := os.Open(filepath.Join(store.BasePath(), relPath))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, 200, cfg.Width)
	assert.Equal(t, 150, cfg.Height)
}

func TestSaveUpload(t *testing.T) {
	data := createTestImage(t, 300, 200)

	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	relPath, err := store.SaveUpload(context.Background(), bytes.NewReader(data), "recipes", 10)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("recipes", "10.jpg"), relPath)

	f, err := os.Open(filepath.Join(store.BasePath(), relPath))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	_, format, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, "jpeg", format)
}

func TestDelete(t *testing.T) {
	data := createTestImage(t, 50, 50)

	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	relPath, err := store.SaveUpload(context.Background(), bytes.NewReader(data), "ingredients", 7)
	require.NoError(t, err)

	absPath := filepath.Join(store.BasePath(), relPath)
	_, err = os.Stat(absPath)
	require.NoError(t, err)

	err = store.Delete("ingredients", 7)
	require.NoError(t, err)

	_, err = os.Stat(absPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDelete_NonExistent(t *testing.T) {
	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	err = store.Delete("ingredients", 9999)
	assert.NoError(t, err)
}

func TestDeterministicFilename(t *testing.T) {
	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	data1 := createTestImage(t, 80, 80)
	relPath1, err := store.SaveUpload(context.Background(), bytes.NewReader(data1), "ingredients", 3)
	require.NoError(t, err)

	data2 := createTestImage(t, 120, 120)
	relPath2, err := store.SaveUpload(context.Background(), bytes.NewReader(data2), "ingredients", 3)
	require.NoError(t, err)

	assert.Equal(t, relPath1, relPath2)

	// Verify the file has the dimensions of the second save.
	f, err := os.Open(filepath.Join(store.BasePath(), relPath2))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, 120, cfg.Width)
	assert.Equal(t, 120, cfg.Height)
}

func TestSaveFromURL_InvalidImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not an image"))
	}))
	t.Cleanup(srv.Close)

	store, err := imagestore.New(t.TempDir(), srv.Client())
	require.NoError(t, err)

	_, err = store.SaveFromURL(context.Background(), srv.URL+"/bad", "ingredients", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode image")
}

func TestCategorySubdirectory(t *testing.T) {
	base := t.TempDir()
	store, err := imagestore.New(base, nil)
	require.NoError(t, err)

	data := createTestImage(t, 50, 50)
	_, err = store.SaveUpload(context.Background(), bytes.NewReader(data), "meals", 1)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(base, "meals"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
