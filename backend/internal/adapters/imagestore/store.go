package imagestore

import (
	"bytes"
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

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

const (
	maxDimension = 1200
	jpegQuality  = 85
	maxBodyBytes = 10 << 20 // 10 MB, uploads

	maxDownloadBytes = 5 << 20 // 5 MiB, SaveFromURL; matches httpfetch
)

// Store handles filesystem-based image storage with resize.
type Store struct {
	basePath   string
	httpClient *http.Client
	// guarded serves downloads whose ctx carries a URL guard.
	guarded *http.Client
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

// SetGuardedClient sets the client used for downloads whose ctx carries a
// URL guard (importer.WithURLGuard), typically httpfetch.GuardedClient: it
// re-checks every redirect hop and the dialled address.
func (s *Store) SetGuardedClient(hc *http.Client) { s.guarded = hc }

// SaveFromURL downloads an image from url, resizes it, and saves as JPEG.
// Returns the relative path (e.g., "ingredients/42.jpg"). Bodies larger than
// maxDownloadBytes are rejected.
//
// A URL the user typed has no SSRF guard (private/loopback addresses are
// allowed: Plantry is a single-user, LAN-only app). A URL from an untrusted
// party (a discovered page's image) comes with a guard in ctx: the URL is
// vetted and the download goes through the guarded client.
func (s *Store) SaveFromURL(ctx context.Context, url, category string, id int64) (string, error) {
	hc := s.httpClient
	if guard := importer.URLGuardFrom(ctx); guard != nil {
		if err := guard(url); err != nil {
			return "", fmt.Errorf("download image: %w: %w", importer.ErrURLBlocked, err)
		}
		if s.guarded == nil {
			return "", fmt.Errorf("download image: %w: no guarded client", importer.ErrURLBlocked)
		}
		hc = s.guarded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	// Read one byte past the cap so an oversized body is rejected rather than
	// silently truncated.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	if len(data) > maxDownloadBytes {
		return "", fmt.Errorf("download image: body exceeds %d bytes", maxDownloadBytes)
	}
	return s.processAndSave(bytes.NewReader(data), category, id)
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

// Copy duplicates the stored image of (fromCategory, fromID) to
// (toCategory, toID) without downloading or re-encoding it, and returns the
// new relative path. A missing source is an error wrapping os.ErrNotExist.
func (s *Store) Copy(fromCategory string, fromID int64, toCategory string, toID int64) (string, error) {
	src, err := os.Open(filepath.Join(s.basePath, fromCategory, fmt.Sprintf("%d.jpg", fromID)))
	if err != nil {
		return "", fmt.Errorf("copy image: %w", err)
	}
	defer func() { _ = src.Close() }()

	dir := filepath.Join(s.basePath, toCategory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create category dir: %w", err)
	}
	filename := fmt.Sprintf("%d.jpg", toID)
	dst, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		return "", fmt.Errorf("copy image: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(dst.Name())
		return "", fmt.Errorf("copy image: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("copy image: %w", err)
	}
	return filepath.Join(toCategory, filename), nil
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
