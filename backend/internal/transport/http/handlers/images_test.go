package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func testImagePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func setupImageRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	svc := ingredient.NewService(repo)

	store, err := imagestore.New(t.TempDir(), nil)
	require.NoError(t, err)

	ih := handlers.NewImageHandler(svc, store)
	ingH := handlers.NewIngredientHandler(svc)

	r := chi.NewRouter()
	r.Route("/api/ingredients", func(r chi.Router) {
		r.Post("/", ingH.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", ingH.Get)
			r.Post("/image", ih.Upload)
			r.Delete("/image", ih.DeleteImage)
		})
	})
	return r
}

func createTestIngredient(t *testing.T, r http.Handler) float64 {
	t.Helper()
	body := `{"name":"Test Ingredient"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients", bytes.NewBufferString(body))
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got["id"].(float64)
}

func TestUploadImage(t *testing.T) {
	r := setupImageRouter(t)
	id := createTestIngredient(t, r)

	// Build multipart request.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testImagePNG(t))
	require.NoError(t, w.Close())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/ingredients/%d/image", int(id)), &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Contains(t, got["image_path"], "ingredients/")
}

func TestUploadImage_NotFound(t *testing.T) {
	r := setupImageRouter(t)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testImagePNG(t))
	require.NoError(t, w.Close())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingredients/999/image", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestDeleteImage(t *testing.T) {
	r := setupImageRouter(t)
	id := createTestIngredient(t, r)

	// Upload first.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testImagePNG(t))
	require.NoError(t, w.Close())

	uploadResp := httptest.NewRecorder()
	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/ingredients/%d/image", int(id)), &buf)
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	r.ServeHTTP(uploadResp, uploadReq)
	require.Equal(t, http.StatusOK, uploadResp.Code)

	// Delete.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/ingredients/%d/image", int(id)), nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestDeleteImage_NotFound(t *testing.T) {
	r := setupImageRouter(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/ingredients/999/image", nil)
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}
