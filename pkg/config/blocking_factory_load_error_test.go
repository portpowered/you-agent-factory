package config_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestBlockingFactoryLoadError_PreservesCanonicalTargetsOnCanonicalJSONLoad(t *testing.T) {
	_, err := load.LoadFromCanonicalJSON([]byte(factoryvalidation.CrossPathInvalidFactoryJSON), load.LoadOptions{})
	if err == nil {
		t.Fatal("expected cross-path invalid factory to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, did not want os.ErrNotExist", err)
	}

	loadErr, ok := load.AsBlockingFactoryLoadError(err)
	if !ok {
		t.Fatalf("error = %v, want BlockingFactoryLoadError", err)
	}
	if len(loadErr.Targets) == 0 {
		t.Fatal("expected non-empty blocking validation targets")
	}
	for _, target := range loadErr.Targets {
		if strings.TrimSpace(target.Message) == "" {
			t.Fatalf("target = %#v, want non-empty message", target)
		}
		if strings.TrimSpace(target.Code) == "" {
			t.Fatalf("target = %#v, want non-empty code", target)
		}
	}

	findings := load.BlockingFactoryLoadFindings(err)
	if len(findings) != len(loadErr.Targets) {
		t.Fatalf("findings = %d, targets = %d, want equivalent coverage", len(findings), len(loadErr.Targets))
	}
}

func TestBlockingFactoryLoadError_PreservesTargetsThroughNamedFactoryMaterialization(t *testing.T) {
	rootDir := t.TempDir()

	_, err := factoryconfig.PersistNamedFactory(rootDir, "@you/goal", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))
	if err == nil {
		t.Fatal("expected invalid named factory materialization to fail")
	}
	if !errors.Is(err, factoryconfig.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, did not want os.ErrNotExist", err)
	}

	loadErr, ok := factoryconfig.AsBlockingFactoryLoadError(err)
	if !ok {
		t.Fatalf("error = %v, want BlockingFactoryLoadError through materialization", err)
	}
	if len(loadErr.Targets) == 0 {
		t.Fatal("expected non-empty blocking validation targets")
	}
}

func TestBlockingFactoryLoadError_DistinguishesIOErrorsFromValidationFailures(t *testing.T) {
	missingDir := t.TempDir()
	_ = os.Remove(missingDir)

	_, err := load.LoadFromFactoryDir(missingDir, nil)
	if err == nil {
		t.Fatal("expected missing factory directory to fail")
	}
	if load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, did not want ErrInvalidNamedFactory", err)
	}
	if _, ok := load.AsBlockingFactoryLoadError(err); ok {
		t.Fatalf("error = %v, did not want BlockingFactoryLoadError", err)
	}
}
