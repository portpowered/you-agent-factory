package lifecycle_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type readHost struct {
	*versionHost
	root                                                   string
	pointerError, sessionError, runtimeError, captureError error
}

func (h readHost) PersistRootDir() string                           { return h.root }
func (h readHost) ReadCurrentFactoryPointer(string) (string, error) { return "alpha", h.pointerError }
func (h readHost) RequireSession(string) (*definitions.DefinitionSession, error) {
	return &definitions.DefinitionSession{FolderPath: h.root}, h.sessionError
}
func (h readHost) SessionRuntimeConfig(string) (definitions.LoadedFactorySource, error) {
	return h.source, h.runtimeError
}
func (h readHost) SessionFactoryPersistRoot(*definitions.DefinitionSession) string { return h.root }
func (h readHost) CurrentRuntimeConfig() definitions.LoadedFactorySource           { return h.source }
func (h readHost) PreparePortableFactoryConfig(_ string, config *definitions.FactoryConfig, _ bool) (*definitions.FactoryConfig, error) {
	return config, nil
}
func (h readHost) CaptureFactorySnapshot(string, *definitions.FactoryConfig, definitions.RuntimeDefinitionLookup, string, map[string]string) (*definitions.FactorySnapshot, error) {
	if h.captureError != nil {
		return nil, h.captureError
	}
	return definitions.NewFactorySnapshot(map[string]any{"name": "loaded", "project": "preserved"})
}

type readSource struct {
	versionSource
	dir string
}

func (s readSource) FactoryDir() string { return s.dir }

func newReadHost() readHost {
	return readHost{root: "root", versionHost: &versionHost{source: readSource{
		versionSource: versionSource{config: &definitions.FactoryConfig{Version: &definitions.FactoryVersion{Logical: 7, Physical: time.Unix(100, 0).UTC()}}},
		dir:           filepath.Join("root", "alpha"),
	}}}
}

func TestCurrentSessionReadReturnsNamedSnapshotAndDurableVersion(t *testing.T) {
	t.Parallel()
	host := newReadHost()
	got, err := injectedLifecycle(host, nil).GetCurrentFactoryForSession(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" || got.Version == nil || *got.Version != *host.source.FactoryConfig().Version {
		t.Fatalf("editable factory = %#v", got)
	}
	assertReadSnapshot(t, got.Snapshot, "alpha")
}

func assertReadSnapshot(t *testing.T, snapshot *definitions.FactorySnapshot, name string) {
	t.Helper()
	var result struct{ Name, Project string }
	if snapshot == nil {
		t.Fatal("missing snapshot")
	}
	if err := snapshot.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Name != name || result.Project != "preserved" {
		t.Fatalf("snapshot = %#v", result)
	}
}

func TestCurrentNamedReadPreservesSourceAndFailureCauses(t *testing.T) {
	t.Parallel()
	cause := errors.New("read dependency unavailable")
	for _, stage := range []string{"success", "pointer", "lookup", "load", "capture", "missing pointer"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := newReadHost()
			wantError := cause
			switch stage {
			case "success":
				wantError = nil
			case "pointer":
				host.pointerError = cause
			case "lookup":
				host.resolveError = cause
			case "load":
				host.loadError = cause
			case "capture":
				host.captureError = cause
			case "missing pointer":
				host.pointerError = fs.ErrNotExist
				wantError = definitions.ErrCurrentFactoryNotFound
			}
			got, err := injectedLifecycle(host, nil).GetCurrentNamedFactory(context.Background())
			if wantError != nil {
				if got != nil || !errors.Is(err, wantError) {
					t.Fatalf("read = %#v, %v; want cause %v", got, err, wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertReadSnapshot(t, got, "alpha")
		})
	}
}

func TestCurrentSessionReadRetainsFailuresWithoutFabricatingSnapshot(t *testing.T) {
	t.Parallel()
	cause := errors.New("session read unavailable")
	for _, stage := range []string{"session", "runtime", "capture", "version"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := newReadHost()
			switch stage {
			case "session":
				host.sessionError = cause
			case "runtime":
				host.runtimeError = cause
			case "capture":
				host.captureError = cause
			case "version":
				host.loadError = cause
			}
			got, err := injectedLifecycle(host, nil).GetCurrentFactoryForSession(context.Background(), "session")
			if !errors.Is(err, cause) || got.Snapshot != nil || got.Version != nil {
				t.Fatalf("read = %#v, %v", got, err)
			}
		})
	}
}
