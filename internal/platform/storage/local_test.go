package storage_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
)

func jpegBytes() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 64)...)
}

func pngBytes() []byte {
	return append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 64)...)
}

func TestLocalSaveStoresAllowedImageWithUUIDName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := storage.NewLocal(dir, 1)

	path, err := store.Save(bytes.NewReader(jpegBytes()))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.HasPrefix(path, "uploads/") || !strings.HasSuffix(path, ".jpg") {
		t.Fatalf("path = %q, want uploads/<uuid>.jpg", path)
	}
	if strings.Contains(filepath.Base(path), " ") || len(filepath.Base(path)) != 36+len(".jpg") {
		t.Fatalf("filename = %q, want a UUID plus extension", filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}

	data, err := store.Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(data, jpegBytes()) {
		t.Fatal("Read() returned different bytes")
	}
}

func TestLocalSaveAcceptsPNGAndWebP(t *testing.T) {
	t.Parallel()

	store := storage.NewLocal(t.TempDir(), 1)

	pngPath, err := store.Save(bytes.NewReader(pngBytes()))
	if err != nil {
		t.Fatalf("Save(png) error = %v", err)
	}
	if !strings.HasSuffix(pngPath, ".png") {
		t.Fatalf("png path = %q", pngPath)
	}

	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0x00}, 32)...)
	webpPath, err := store.Save(bytes.NewReader(webp))
	if err != nil {
		t.Fatalf("Save(webp) error = %v", err)
	}
	if !strings.HasSuffix(webpPath, ".webp") {
		t.Fatalf("webp path = %q", webpPath)
	}
}

func TestLocalSaveRejectsDisallowedAndOversizedFiles(t *testing.T) {
	t.Parallel()

	t.Run("not an image", func(t *testing.T) {
		t.Parallel()
		store := storage.NewLocal(t.TempDir(), 1)
		_, err := store.Save(strings.NewReader("hello world"))
		if !errors.Is(err, errs.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		t.Parallel()
		store := storage.NewLocal(t.TempDir(), 1)
		big := append(jpegBytes(), bytes.Repeat([]byte{0x00}, 2*1024*1024)...)
		_, err := store.Save(bytes.NewReader(big))
		if !errors.Is(err, errs.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		store := storage.NewLocal(t.TempDir(), 1)
		_, err := store.Save(bytes.NewReader(nil))
		if !errors.Is(err, errs.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestLocalDeleteRemovesFileAndIsIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := storage.NewLocal(dir, 1)

	path, err := store.Save(bytes.NewReader(jpegBytes()))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Delete(path); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Read(path); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Read() after delete err = %v, want ErrNotFound", err)
	}
	if err := store.Delete(path); err != nil {
		t.Fatalf("Delete() second call error = %v, want nil", err)
	}
}

func TestLocalDeleteRejectsTraversal(t *testing.T) {
	t.Parallel()

	store := storage.NewLocal(t.TempDir(), 1)
	if err := store.Delete("../secret.txt"); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestLocalReadRejectsTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := storage.NewLocal(dir, 1)

	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("top secret"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}

	for _, name := range []string{"../secret.txt", "uploads/../../secret.txt", "/etc/passwd", ".env", ""} {
		if _, err := store.Read(name); !errors.Is(err, errs.ErrInvalidInput) && !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("Read(%q) err = %v, want invalid or not found", name, err)
		}
	}
}
