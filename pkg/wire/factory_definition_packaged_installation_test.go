package wire

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
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

func TestProvidePackagedFactoryDefinitions_LoadsDetachedGeneratedCatalog(t *testing.T) {
	definitions, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("providePackagedFactoryDefinitions() error = %v", err)
	}

	wantNames := []string{
		"@you/deep-research",
		"@you/fusion",
		"@you/goal",
		"@you/quorum",
		"@you/review",
		"@you/subagent",
		"@you/tts",
	}
	if len(definitions) != len(wantNames) {
		t.Fatalf("definition count = %d, want %d", len(definitions), len(wantNames))
	}
	for index, wantName := range wantNames {
		if definitions[index].Name != wantName {
			t.Fatalf("definitions[%d].Name = %q, want %q", index, definitions[index].Name, wantName)
		}
	}

	publishedGoal, err := fs.ReadFile(
		packagedfactories.Published(),
		"generated/factories/goal/factory.json",
	)
	if err != nil {
		t.Fatalf("read published Goal definition: %v", err)
	}
	if !bytes.Equal(definitions[2].JSON, publishedGoal) {
		t.Fatal("injected Goal definition differs from the generated publication artifact")
	}

	definitions[2].JSON[0] ^= 0xff
	reloaded, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("second providePackagedFactoryDefinitions() error = %v", err)
	}
	if !bytes.Equal(reloaded[2].JSON, publishedGoal) {
		t.Fatal("mutating one injected catalog changed a later injected catalog")
	}
}
