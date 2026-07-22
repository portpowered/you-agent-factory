package packagedfactories_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

func TestSourceReadsAuthoredFactoryAndAsset(t *testing.T) {
	t.Parallel()

	source := packagedfactories.Source()
	for _, path := range []string{
		"factories/deep-research/factory.json",
		"factories/deep-research/scripts/deep-research.workflow.js",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			content, err := fs.ReadFile(source, path)
			if err != nil {
				t.Fatalf("read embedded source %q: %v", path, err)
			}
			if len(content) == 0 {
				t.Fatalf("embedded source %q is empty", path)
			}
		})
	}
}

func TestSourceReadBytesAreDetachedAcrossCallers(t *testing.T) {
	t.Parallel()

	const path = "factories/goal/factory.json"
	first, err := fs.ReadFile(packagedfactories.Source(), path)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	second, err := fs.ReadFile(packagedfactories.Source(), path)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("equivalent reads returned different content")
	}

	first[0] ^= 0xff
	third, err := fs.ReadFile(packagedfactories.Source(), path)
	if err != nil {
		t.Fatalf("read after caller mutation: %v", err)
	}
	if !bytes.Equal(second, third) {
		t.Fatal("mutating one read affected a later read")
	}
}

func TestSourceReturnsOrdinaryReadErrors(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"factories/missing/factory.json",
		"../factory.json",
		"factories/deep-research",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			_, err := fs.ReadFile(packagedfactories.Source(), path)
			if err == nil {
				t.Fatalf("read %q unexpectedly succeeded", path)
			}
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("read %q returned %T, want *fs.PathError", path, err)
			}
		})
	}
}

func TestSourceDoesNotExposeWriteCapabilities(t *testing.T) {
	t.Parallel()

	source := packagedfactories.Source()
	if _, ok := source.(interface {
		WriteFile(string, []byte, fs.FileMode) error
	}); ok {
		t.Fatal("embedded source exposes WriteFile")
	}
	if _, ok := source.(interface{ Remove(string) error }); ok {
		t.Fatal("embedded source exposes Remove")
	}
	if _, ok := source.(interface{ Rename(string, string) error }); ok {
		t.Fatal("embedded source exposes Rename")
	}

	file, err := source.Open("factories/goal/factory.json")
	if err != nil {
		t.Fatalf("open embedded source: %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close embedded source: %v", err)
		}
	})
	if _, ok := file.(io.Writer); ok {
		t.Fatal("embedded source file is writable")
	}
}
