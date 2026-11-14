/*
Package storage implements local on-disk upload storage.

Uploaded photos are written under <dataDir>/uploads with random UUID names, so
the original client filename never reaches the filesystem and cannot be used for
path traversal. Only JPEG, PNG and WebP images are accepted, and each file is
bounded by the configured maximum size.
*/
package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

// uploadDirName is the only directory the adapter reads or writes.
const uploadDirName = "uploads"

// allowedTypes maps the sniffed MIME type to the extension used on disk.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// Local stores uploads on the local filesystem.
type Local struct {
	root     string
	maxBytes int64
}

// NewLocal returns a store rooted at <dataDir>/uploads. maxUploadMB is the
// per-file limit; non-positive values disable the limit check (not recommended)
// and are clamped to a sane default.
func NewLocal(dataDir string, maxUploadMB int) *Local {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	if maxUploadMB <= 0 {
		maxUploadMB = 8
	}
	return &Local{
		root:     filepath.Join(dataDir, uploadDirName),
		maxBytes: int64(maxUploadMB) << 20,
	}
}

// Save reads one upload, validates its MIME type and size, writes it with a
// random UUID filename and returns the relative path ("uploads/<uuid>.ext").
func (l *Local) Save(r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("save upload: reader is required: %w", errs.ErrInvalidInput)
	}

	data, err := io.ReadAll(io.LimitReader(r, l.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("save upload: reading body: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("save upload: empty file: %w", errs.ErrInvalidInput)
	}
	if int64(len(data)) > l.maxBytes {
		return "", fmt.Errorf("save upload: file exceeds %d bytes: %w", l.maxBytes, errs.ErrInvalidInput)
	}

	mediaType := http.DetectContentType(data)
	extension, ok := allowedTypes[mediaType]
	if !ok {
		return "", fmt.Errorf("save upload: unsupported content type %q: %w", mediaType, errs.ErrInvalidInput)
	}

	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("save upload: generating filename: %w", err)
	}

	if err := os.MkdirAll(l.root, 0o750); err != nil {
		return "", fmt.Errorf("save upload: creating directory: %w", err)
	}

	name := id.String() + extension
	fullPath := filepath.Join(l.root, name)
	if err := os.WriteFile(fullPath, data, 0o640); err != nil {
		return "", fmt.Errorf("save upload: writing file: %w", err)
	}

	return uploadDirName + "/" + name, nil
}

// Read returns the bytes of a stored upload. The name may be a bare filename
// or a relative path with the "uploads/" prefix; anything that escapes the
// upload directory is rejected with ErrInvalidInput.
func (l *Local) Read(path string) ([]byte, error) {
	name, err := safeName(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(l.root, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read upload %q: %w", path, errs.ErrNotFound)
		}
		return nil, fmt.Errorf("read upload %q: %w", path, err)
	}
	return data, nil
}

// safeName validates a stored path and returns the flat filename inside the
// uploads directory.
func safeName(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("upload name is required: %w", errs.ErrInvalidInput)
	}

	normalized := filepath.ToSlash(trimmed)
	normalized = strings.TrimPrefix(normalized, uploadDirName+"/")
	if normalized == "" || normalized == "." || normalized == ".." ||
		strings.Contains(normalized, "/") || strings.HasPrefix(normalized, ".") {
		return "", fmt.Errorf("invalid upload name %q: %w", path, errs.ErrInvalidInput)
	}
	return normalized, nil
}
