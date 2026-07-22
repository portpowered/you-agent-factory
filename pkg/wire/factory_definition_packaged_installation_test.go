package wire

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

type packagedInstallationFileSystemStub struct{}

func (packagedInstallationFileSystemStub) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func TestProvideNamedFactoryCandidatePathsResolverForwardsOwnerOperation(t *testing.T) {
	paths, err := factorynamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("build named path resolver: %v", err)
	}
	resolve := provideNamedFactoryCandidatePathsResolver(paths)
	got, err := resolve("project", "global", "@you/goal")
	if err != nil {
		t.Fatalf("resolve candidate paths: %v", err)
	}
	if got.Project != filepath.Join("project", "@you", "goal") {
		t.Fatalf("project candidate = %q", got.Project)
	}
	if got.Global != filepath.Join("global", "@you", "goal") {
		t.Fatalf("global candidate = %q", got.Global)
	}
}

func (packagedInstallationFileSystemStub) ReadFile(string) ([]byte, error) {
	return nil, fs.ErrNotExist
}

type factoryDefinitionClockStub struct{}

func (factoryDefinitionClockStub) Now() time.Time { return time.Unix(1, 0) }

func TestProvideFactoryDefinitionPackagedInstallationFileSystemSelectsOverrideOrLocalDefault(t *testing.T) {
	override := packagedInstallationFileSystemStub{}
	if got := provideFactoryDefinitionPackagedInstallationFileSystem(serviceedges.Edges{
		FactoryDefinitionPackagedInstallationFileSystem: override,
	}); got != override {
		t.Fatalf("override = %T, want exact override", got)
	}
	if got := provideFactoryDefinitionPackagedInstallationFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("default = %T, want platform filesystem Local", got)
	}
}

func TestProvideFactoryDefinitionRemainingEffectsSelectOverridesOrPolicyFreeDefaults(t *testing.T) {
	files := packagedInstallationFileSystemStub{}
	clock := factoryDefinitionClockStub{}
	edges := serviceedges.Edges{
		FactoryDefinitionClock:                         clock,
		FactoryDefinitionVersionFileSystem:             files,
		FactoryDefinitionPackagedGoalPromptFileSystem:  files,
		FactoryDefinitionPortableBundledFileInspection: files,
	}
	if got := provideFactoryDefinitionClock(edges); got != clock {
		t.Fatalf("clock override = %T, want exact override", got)
	}
	if got := provideFactoryDefinitionVersionFileSystem(edges); got != files {
		t.Fatalf("version filesystem override = %T, want exact override", got)
	}
	if got := provideFactoryDefinitionPackagedGoalPromptFileSystem(edges); got != files {
		t.Fatalf("packaged Goal filesystem override = %T, want exact override", got)
	}
	if got := provideFactoryDefinitionPortableBundledFileInspection(edges); got != files {
		t.Fatalf("portable inspection override = %T, want exact override", got)
	}

	if got := provideFactoryDefinitionClock(serviceedges.Edges{}); got != (platformclock.Real{}) {
		t.Fatalf("clock default = %T, want platform clock Real", got)
	}
	if got := provideFactoryDefinitionVersionFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("version filesystem default = %T, want platform filesystem Local", got)
	}
	if got := provideFactoryDefinitionPackagedGoalPromptFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("packaged Goal filesystem default = %T, want platform filesystem Local", got)
	}
	if got := provideFactoryDefinitionPortableBundledFileInspection(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("portable inspection default = %T, want platform filesystem Local", got)
	}
}
