package imagestore

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

const (
	maxDimension = 1200
	jpegQuality  = 85
	maxBodyBytes = 10 << 20 // 10 MB
)

// Store handles filesystem-based image storage with resize.
type Store struct {
	basePath   string
	httpClient *http.Client
}

// New creates a Store. Creates basePath directory if it doesn't exist.
func New(basePath string, httpClient *http.Client) (*Store, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("create image dir: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Store{basePath: basePath, httpClient: httpClient}, nil
}

// SaveFromURL downloads an image from url, resizes it, and saves as JPEG.
// Returns the relative path (e.g., "ingredients/42.jpg").
func (s *Store) SaveFromURL(ctx context.Context, url, category string, id int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxBodyBytes)
	return s.processAndSave(limited, category, id)
}

// SaveUpload processes an uploaded image from reader, resizes, and saves as JPEG.
// Returns the relative path.
func (s *Store) SaveUpload(_ context.Context, r io.Reader, category string, id int64) (string, error) {
	limited := io.LimitReader(r, maxBodyBytes)
	return s.processAndSave(limited, category, id)
}

// Delete removes the image file for the given category and id.
// Returns nil if the file does not exist.
func (s *Store) Delete(category string, id int64) error {
	path := filepath.Join(s.basePath, category, fmt.Sprintf("%d.jpg", id))
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// BasePath returns the base directory for serving static images.
func (s *Store) BasePath() string {
	return s.basePath
}

func (s *Store) processAndSave(r io.Reader, category string, id int64) (string, error) {
	img, err := imaging.Decode(r, imaging.AutoOrientation(true))
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	img = resizeIfNeeded(img)

	dir := filepath.Join(s.basePath, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create category dir: %w", err)
	}

	filename := fmt.Sprintf("%d.jpg", id)
	absPath := filepath.Join(dir, filename)

	if err := imaging.Save(img, absPath, imaging.JPEGQuality(jpegQuality)); err != nil {
		return "", fmt.Errorf("save image: %w", err)
	}

	relPath := filepath.Join(category, filename)
	return relPath, nil
}

func resizeIfNeeded(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= maxDimension && h <= maxDimension {
		return img
	}

	return imaging.Fit(img, maxDimension, maxDimension, imaging.Lanczos)
}
