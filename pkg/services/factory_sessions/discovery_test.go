package factorysessions_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
)

type discoveryDirectoryInspection struct {
	entries []fs.DirEntry
	err     error
}

func (d discoveryDirectoryInspection) Stat(string) (fs.FileInfo, error) {
	return discoveryFileInfo{}, nil
}

func (d discoveryDirectoryInspection) ReadDir(string) ([]fs.DirEntry, error) {
	return d.entries, d.err
}

type discoveryDirEntry struct {
	name  string
	isDir bool
}

type discoveryFileInfo struct{}

func (discoveryFileInfo) Name() string       { return "session-root" }
func (discoveryFileInfo) Size() int64        { return 0 }
func (discoveryFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (discoveryFileInfo) ModTime() time.Time { return time.Time{} }
func (discoveryFileInfo) IsDir() bool        { return true }
func (discoveryFileInfo) Sys() any           { return nil }

func (e discoveryDirEntry) Name() string               { return e.name }
func (e discoveryDirEntry) IsDir() bool                { return e.isDir }
func (e discoveryDirEntry) Type() fs.FileMode          { return 0 }
func (e discoveryDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func alwaysRunnableProbe(folderPath, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure) {
	return logicaltarget.Build(folderPath, factoryDir, ref, filepath.Base(factoryDir)), true, nil
}

func TestDiscoverTargets_ReturnsDefaultAndNamedTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	targets, err := logicaltarget.Discover(root, alwaysRunnableProbe, platformfilesystem.Local{}, os.UserHomeDir)
	if err != nil {
		t.Fatalf("DiscoverTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("len(targets) = %d, want 2", len(targets))
	}
	if targets[0].Ref.Kind != TargetKindDefault || targets[1].Ref != (TargetRef{Kind: TargetKindNamed, Name: "beta"}) {
		t.Fatalf("targets = %#v, want default then named beta", targets)
	}
}

func TestDiscoverTargets_RejectsFolderWithoutRunnableTargets(t *testing.T) {
	root := t.TempDir()
	_, err := logicaltarget.Discover(root, func(string, string, TargetRef) (Target, bool, *DiscoveryFailure) {
		return Target{}, false, nil
	}, platformfilesystem.Local{}, os.UserHomeDir)
	if err == nil {
		t.Fatal("DiscoverTargets(empty) error = nil, want not runnable")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonNotRunnable || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want not_runnable folderPath", reason, field, ok)
	}
}

func TestDiscoverTargets_PreservesConfigLoadFailuresWhenNoRunnableTargetsRemain(t *testing.T) {
	root := t.TempDir()

	_, err := logicaltarget.Discover(root, func(_ string, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure) {
		return Target{}, false, &DiscoveryFailure{
			FactoryDir: factoryDir,
			Ref:        ref,
			Summary:    "unexpected end of JSON input",
		}
	}, platformfilesystem.Local{}, os.UserHomeDir)
	if err == nil {
		t.Fatal("DiscoverTargets(config load failed) error = nil, want structured failure")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonConfigLoadFailed || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want config_load_failed folderPath", reason, field, ok)
	}
	var targetedErr interface {
		ErrorTargets() []factorydefinitions.ValidationTarget
	}
	if !errors.As(err, &targetedErr) {
		t.Fatalf("config load error %v did not expose structured targets", err)
	}
	targets := targetedErr.ErrorTargets()
	if len(targets) != 1 {
		t.Fatalf("config load error targets = %#v, want one target", targets)
	}
	if targets[0].Code != "factory.session.target.config_load_failed" {
		t.Fatalf("config load target code = %q, want factory.session.target.config_load_failed", targets[0].Code)
	}
	if targets[0].Subject.ID != "default" {
		t.Fatalf("config load target subject id = %q, want default", targets[0].Subject.ID)
	}
}

func TestDiscoverTargets_UsesInjectedDirectoryOrderingAndFiltering(t *testing.T) {
	root := t.TempDir()
	directories := discoveryDirectoryInspection{entries: []fs.DirEntry{
		discoveryDirEntry{name: "beta", isDir: true},
		discoveryDirEntry{name: "notes.txt"},
		discoveryDirEntry{name: "bad/name", isDir: true},
		discoveryDirEntry{name: "alpha", isDir: true},
	}}
	targets, err := logicaltarget.Discover(root, alwaysRunnableProbe, directories, os.UserHomeDir)
	if err != nil {
		t.Fatalf("DiscoverTargets: %v", err)
	}
	want := []TargetRef{
		{Kind: TargetKindDefault},
		{Kind: TargetKindNamed, Name: "alpha"},
		{Kind: TargetKindNamed, Name: "beta"},
	}
	if len(targets) != len(want) {
		t.Fatalf("targets = %#v, want refs %#v", targets, want)
	}
	for index := range want {
		if targets[index].Ref != want[index] {
			t.Fatalf("target[%d] ref = %#v, want %#v", index, targets[index].Ref, want[index])
		}
	}
}

func TestDiscoverTargets_FailsClosedAndClassifiesDirectoryErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, nil, os.UserHomeDir); err == nil {
		t.Fatal("DiscoverTargets(nil inspection) error = nil")
	}
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, discoveryDirectoryInspection{err: fs.ErrPermission}, os.UserHomeDir); err == nil {
		t.Fatal("DiscoverTargets(permission) error = nil")
	} else if reason, field, ok := sessionvalidation.ReasonFromError(err); !ok || reason != ValidationReasonUnreadable || field != "folderPath" {
		t.Fatalf("permission validation = (%q, %q, %v)", reason, field, ok)
	}
	readErr := errors.New("directory unavailable")
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, discoveryDirectoryInspection{err: readErr}, os.UserHomeDir); !errors.Is(err, readErr) {
		t.Fatalf("DiscoverTargets(generic read error) = %v, want wrapped %v", err, readErr)
	}
}

func TestSelectTarget_AutoSelectsSingleTarget(t *testing.T) {
	targets := []Target{{
		Ref:   TargetRef{Kind: TargetKindDefault},
		Label: "default",
	}}
	selected, err := logicaltarget.Select(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected == nil || selected.Ref.Kind != TargetKindDefault {
		t.Fatalf("selected = %#v, want default target", selected)
	}
}

func TestSelectTarget_ReturnsNilForAmbiguousFolder(t *testing.T) {
	targets := []Target{
		{Ref: TargetRef{Kind: TargetKindDefault}},
		{Ref: TargetRef{Kind: TargetKindNamed, Name: "beta"}},
	}
	selected, err := logicaltarget.Select(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected != nil {
		t.Fatalf("selected = %#v, want nil for multi-target picker", selected)
	}
}

func TestSelectTarget_RejectsMissingNamedTarget(t *testing.T) {
	targets := []Target{{Ref: TargetRef{Kind: TargetKindDefault}}}
	_, err := logicaltarget.Select(targets, &TargetRef{Kind: TargetKindNamed, Name: "missing"})
	if err == nil {
		t.Fatal("SelectTarget(missing) error = nil, want target_not_found")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonTargetNotFound || field != "target.name" {
		t.Fatalf("validation = (%q, %q, %v), want target_not_found target.name", reason, field, ok)
	}
}

func TestCloneTargets_ReturnsDefensiveCopy(t *testing.T) {
	original := []Target{{Ref: TargetRef{Kind: TargetKindDefault}, Label: "default"}}
	cloned := logicaltarget.Clone(original)
	original[0].Label = "mutated"
	if cloned[0].Label != "default" {
		t.Fatalf("cloned label = %q, want unchanged copy", cloned[0].Label)
	}
}
