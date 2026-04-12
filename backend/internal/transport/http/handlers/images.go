package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/imagestore"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

const maxImageSize = 10 << 20 // 10 MB

// ImageHandler holds HTTP handlers for ingredient image management.
type ImageHandler struct {
	svc   *ingredient.Service
	store *imagestore.Store
}

// NewImageHandler creates a new ImageHandler.
func NewImageHandler(svc *ingredient.Service, store *imagestore.Store) *ImageHandler {
	return &ImageHandler{svc: svc, store: store}
}

// Store returns the underlying image store (used for static file serving setup).
func (h *ImageHandler) Store() *imagestore.Store {
	return h.store
}

// Upload handles POST /api/ingredients/{id}/image.
func (h *ImageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	i, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	if err := r.ParseMultipartForm(maxImageSize); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	defer func() { _ = file.Close() }()

	imgPath, err := h.store.SaveUpload(r.Context(), file, "ingredients", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	i.ImagePath = &imgPath
	if err := h.svc.Update(r.Context(), i); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"image_path": imgPath})
}

// DeleteImage handles DELETE /api/ingredients/{id}/image.
func (h *ImageHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}

	i, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	if err := h.store.Delete("ingredients", id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}

	i.ImagePath = nil
	if err := h.svc.Update(r.Context(), i); err != nil {
		status, key := toHTTP(err)
		writeError(w, status, key)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
