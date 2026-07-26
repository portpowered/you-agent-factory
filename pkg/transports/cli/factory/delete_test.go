package factory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDelete_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()

	var out strings.Builder
	if err := testDelete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := out.String(); got != "Deleted factory alpha\n" {
		t.Fatalf("output = %q, want deleted confirmation", got)
	}
}

func TestDelete_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()

	var out bytes.Buffer
	if err := testDelete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var result DeleteResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "alpha" {
		t.Fatalf("result = %#v, want deleted factory alpha", result)
	}
}

func TestDelete_RejectsCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()
	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		delete: func(string, string) error {
			return factorydefinitions.ErrNamedFactoryIsCurrent
		},
	})

	err := testDelete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected delete current factory to fail")
	}
	if !strings.Contains(err.Error(), "cannot delete current factory") {
		t.Fatalf("error = %v, want current-factory refusal", err)
	}
}

func TestDelete_RejectsMissingFactory(t *testing.T) {
	rootDir := t.TempDir()
	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		delete: func(string, string) error {
			return os.ErrNotExist
		},
	})

	err := testDelete(DeleteConfig{
		Name:   "missing",
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected missing factory delete to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestDelete_ListNoLongerIncludesDeletedName(t *testing.T) {
	rootDir := t.TempDir()

	if err := testDelete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		list: func(string) ([]factorydefinitions.NamedFactoryListEntry, error) {
			return []factorydefinitions.NamedFactoryListEntry{{
				Name:       "beta",
				FactoryDir: filepath.Join(rootDir, "beta"),
			}}, nil
		},
	})
	var out strings.Builder
	if err := testList(ListConfig{ProjectRoot: rootDir, GlobalRoot: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if strings.Contains(out.String(), "alpha\t") {
		t.Fatalf("list output still includes alpha: %q", out.String())
	}
	betaDir := filepath.Join(rootDir, "beta")
	wantRow := "beta\t" + betaDir + "\t\n"
	if !strings.Contains(out.String(), wantRow) {
		t.Fatalf("list output = %q, want beta row %q", out.String(), wantRow)
	}
}
