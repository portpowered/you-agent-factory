package lifecycle_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

func injectedLifecycle(host lifecycle.Host, filesystem definitions.VersionFileSystem) *lifecycle.Service {
	disabled := definitions.UnimplementedService{}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host, lifecycle.StubActivationGateway(), disabled, disabled, disabled, disabled,
		disabled, disabled, filesystem, disabled.ListEffectiveFactories,
		definitions.UnimplementedService{},
	)
}

func TestVersionAdvancesLogicalAndUTCPhysicalTime(t *testing.T) {
	t.Parallel()
	current := definitions.FactoryVersion{Logical: 9, Physical: time.Unix(100, 0)}
	for _, tc := range []struct {
		name          string
		current       *definitions.FactoryVersion
		now, physical time.Time
		logical       int64
	}{
		{"initial", nil, time.Unix(50, 0), time.Unix(50, 0), 1},
		{"later clock", &current, time.Unix(200, 0), time.Unix(200, 0), 10},
		{"equal clock", &current, current.Physical, current.Physical.Add(time.Nanosecond), 10},
		{"earlier clock", &current, time.Unix(50, 0), current.Physical.Add(time.Nanosecond), 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := injectedLifecycle(nil, nil).NextEditableFactoryVersion(tc.current, tc.now.In(time.FixedZone("local", 3600)))
			if got.Logical != tc.logical || !got.Physical.Equal(tc.physical) || got.Physical.Location() != time.UTC {
				t.Fatalf("version = %#v, want logical %d physical %v UTC", got, tc.logical, tc.physical)
			}
		})
	}
}

func TestFreshVersionRequiresBothCoordinatesToAdvance(t *testing.T) {
	t.Parallel()
	current := definitions.FactoryVersion{Logical: 9, Physical: time.Unix(100, 0).UTC()}
	for _, tc := range []struct {
		name      string
		candidate *definitions.FactoryVersion
		stale     bool
	}{
		{"missing", nil, true},
		{"equal", &current, true},
		{"older", &definitions.FactoryVersion{Logical: 8, Physical: time.Unix(90, 0)}, true},
		{"only logical advances", &definitions.FactoryVersion{Logical: 10, Physical: current.Physical}, true},
		{"only physical advances", &definitions.FactoryVersion{Logical: 9, Physical: time.Unix(101, 0)}, true},
		{"both advance", &definitions.FactoryVersion{Logical: 10, Physical: time.Unix(101, 0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := injectedLifecycle(nil, nil).RequireFreshEditableFactoryVersion(tc.candidate, current)
			if errors.Is(err, definitions.ErrFactoryVersionStale) != tc.stale || (!tc.stale && err != nil) {
				t.Fatalf("freshness = %v, want stale %t", err, tc.stale)
			}
		})
	}
}

type versionHost struct {
	lifecycle.Host
	source                  definitions.MutableLoadedFactorySource
	resolveError, loadError error
	loadedDir               string
}

func (h *versionHost) ResolveExistingFactoryDir(root, name string) (string, error) {
	return filepath.Join(root, name), h.resolveError
}

func (h *versionHost) WorkstationLoader() definitions.WorkstationLoader { return nil }

func (h *versionHost) LoadFactory(dir string, _ definitions.WorkstationLoader) (definitions.MutableLoadedFactorySource, error) {
	h.loadedDir = dir
	return h.source, h.loadError
}

type versionSource struct {
	definitions.MutableLoadedFactorySource
	config *definitions.FactoryConfig
}

func (s versionSource) FactoryConfig() *definitions.FactoryConfig { return s.config }

type versionFileSystem struct {
	info fs.FileInfo
	err  error
	path string
}

func (f *versionFileSystem) Stat(path string) (fs.FileInfo, error) {
	f.path = path
	return f.info, f.err
}

type modifiedFile struct {
	fs.FileInfo
	modified time.Time
}

func (f modifiedFile) ModTime() time.Time { return f.modified }

func TestCurrentVersionPrefersPersistedMetadataAndFallsBackToMtime(t *testing.T) {
	t.Parallel()
	persisted := definitions.FactoryVersion{Logical: 17, Physical: time.Unix(100, 0).In(time.FixedZone("local", 3600))}
	for _, tc := range []struct {
		name     string
		version  *definitions.FactoryVersion
		modified time.Time
		logical  int64
	}{
		{"persisted", &persisted, time.Unix(200, 0), 17},
		{"mtime", nil, time.Unix(200, 123), time.Unix(200, 123).UnixNano()},
		{"pre epoch mtime", nil, time.Unix(-1, 0), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := &versionHost{source: versionSource{config: &definitions.FactoryConfig{Version: tc.version}}}
			filesystem := &versionFileSystem{info: modifiedFile{modified: tc.modified}}
			got, err := injectedLifecycle(host, filesystem).CurrentFactoryDefinitionVersionAtRoot("root", "alpha")
			physical := tc.modified
			if tc.version != nil {
				physical = tc.version.Physical
			}
			if err != nil || got.Logical != tc.logical || !got.Physical.Equal(physical) || got.Physical.Location() != time.UTC {
				t.Fatalf("version = %#v, %v", got, err)
			}
			if host.loadedDir != filepath.Join("root", "alpha") {
				t.Fatalf("loaded directory = %q", host.loadedDir)
			}
			if tc.version != nil && filesystem.path != "" {
				t.Fatal("persisted version consulted mtime")
			}
			if tc.version == nil && filesystem.path != filepath.Join("root", "alpha", definitions.FactoryConfigFile) {
				t.Fatalf("stat path = %q", filesystem.path)
			}
		})
	}
}

func TestCurrentVersionRetainsLookupLoadAndStatFailures(t *testing.T) {
	t.Parallel()
	cause := errors.New("version dependency unavailable")
	for _, stage := range []string{"lookup", "load", "stat"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := &versionHost{source: versionSource{config: &definitions.FactoryConfig{}}}
			filesystem := &versionFileSystem{}
			switch stage {
			case "lookup":
				host.resolveError = cause
			case "load":
				host.loadError = cause
			case "stat":
				filesystem.err = cause
			}
			got, err := injectedLifecycle(host, filesystem).CurrentFactoryDefinitionVersionAtRoot("root", "alpha")
			if !errors.Is(err, cause) || got != (definitions.FactoryVersion{}) {
				t.Fatalf("version = %#v, %v; want no version and cause", got, err)
			}
		})
	}
}

type staleSaveHost struct {
	*versionHost
	snapshot *definitions.FactorySnapshot
	root     string
}

func (h staleSaveHost) PersistRootDir() string { return h.root }
func (h staleSaveHost) RequireSession(string) (*definitions.DefinitionSession, error) {
	return &definitions.DefinitionSession{FolderPath: h.root}, nil
}
func (h staleSaveHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*definitions.FactorySnapshot, error) {
	return h.snapshot, nil
}
func (h staleSaveHost) ValidateEditableFactorySnapshot(context.Context, *definitions.FactorySnapshot) error {
	return nil
}

func TestStaleSaveRejectsBeforePersistenceOrActivation(t *testing.T) {
	t.Parallel()
	current := definitions.FactoryVersion{Logical: 9, Physical: time.Unix(100, 0).UTC()}
	for _, submitted := range []*definitions.FactoryVersion{nil, &current, {Logical: 8, Physical: time.Unix(90, 0)}} {
		t.Run("submitted version", func(t *testing.T) {
			t.Parallel()
			snapshot, err := definitions.NewFactorySnapshot(map[string]any{"name": "alpha"})
			if err != nil {
				t.Fatal(err)
			}
			host := staleSaveHost{versionHost: &versionHost{source: versionSource{config: &definitions.FactoryConfig{Version: &current}}}, snapshot: snapshot, root: "root"}
			// The injected ports fail the test if a rejected save reaches an effect.
			host.Host = forbiddenPersistence{t: t}
			disabled := definitions.UnimplementedService{}
			gateway := staleActivation{DefinitionActivationGateway: lifecycle.StubActivationGateway(), t: t}
			service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
				host, gateway, disabled, disabled, disabled, disabled, disabled, disabled, nil, disabled.ListEffectiveFactories,
				definitions.UnimplementedService{},
			)
			got, err := service.Save(context.Background(), "session", definitions.SaveModeReplaceCurrent, definitions.EditableFactory{Snapshot: snapshot, Version: submitted})
			if !errors.Is(err, definitions.ErrFactoryVersionStale) || got.Snapshot != nil {
				t.Fatalf("save = %#v, %v", got, err)
			}
		})
	}
}

type forbiddenPersistence struct {
	lifecycle.Host
	t *testing.T
}

func (h forbiddenPersistence) ReplaceFactoryLayoutAtDir(string, *definitions.PreparedFactoryLayoutPayload) (*definitions.FactorySplitLayoutReplaceResult, error) {
	h.t.Fatal("stale save reached persistence")
	return nil, nil
}

type staleActivation struct {
	definitions.DefinitionActivationGateway
	t *testing.T
}

func (g staleActivation) ActivateSessionEditableFactory(context.Context, *definitions.DefinitionSession, string, string, string, string, string) error {
	g.t.Fatal("stale save reached activation")
	return nil
}
