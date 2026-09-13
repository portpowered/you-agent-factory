package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
)

func TestApplyNamedReplacement_RestoresPointerWhenRuntimeReplacementFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		seedAlpha   bool
		wantPointer string
	}{
		{name: "existing pointer", seedAlpha: true, wantPointer: "alpha"},
		{name: "absent pointer", seedAlpha: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			for _, name := range []string{"alpha", "beta"} {
				if err := os.MkdirAll(filepath.Join(rootDir, name), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", name, err)
				}
				if err := os.WriteFile(
					filepath.Join(rootDir, name, factorydefinitions.FactoryConfigFile),
					[]byte(`{"name":"`+name+`"}`),
					0o644,
				); err != nil {
					t.Fatalf("write %s factory: %v", name, err)
				}
			}
			paths := testCurrentPointerStore{}
			if test.seedAlpha {
				if err := paths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
					t.Fatalf("write alpha pointer: %v", err)
				}
			}

			replacementErr := errors.New("controlled runtime replacement failure")
			err := ApplyNamedReplacement(
				context.Background(),
				"session",
				&livesession.LiveSession{ID: "session"},
				true,
				rootDir,
				"beta",
				emptyRuntimeRecord{},
				func(context.Context, string) error { return nil },
				func(context.Context) error { return nil },
				func(context.Context, *livesession.LiveSession, string, runtimeports.RuntimeInstance) error {
					return replacementErr
				},
				func(string, string, runtimeports.RuntimeInstance) error { return nil },
				paths.ReadCurrentPointer,
				paths.WriteCurrentPointer,
				paths,
			)
			if !errors.Is(err, replacementErr) {
				t.Fatalf("ApplyNamedReplacement error = %v, want replacement failure", err)
			}
			pointer, pointerErr := paths.ReadCurrentPointer(rootDir)
			if test.seedAlpha {
				if pointerErr != nil || pointer != test.wantPointer {
					t.Fatalf("pointer after failed replacement = %q, error=%v, want %q", pointer, pointerErr, test.wantPointer)
				}
				return
			}
			if !errors.Is(pointerErr, os.ErrNotExist) {
				t.Fatalf("pointer after failed replacement = %q, error=%v, want absent", pointer, pointerErr)
			}
		})
	}
}

type emptyRuntimeRecord struct {
	factory.RuntimeRecord
}

type testCurrentPointerStore struct{}

func (testCurrentPointerStore) ReadCurrentPointer(rootDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, ".current-factory"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (testCurrentPointerStore) WriteCurrentPointer(rootDir, name string) error {
	return os.WriteFile(filepath.Join(rootDir, ".current-factory"), []byte(name), 0o644)
}

func (testCurrentPointerStore) RemoveCurrentPointer(rootDir string) error {
	return os.Remove(filepath.Join(rootDir, ".current-factory"))
}
