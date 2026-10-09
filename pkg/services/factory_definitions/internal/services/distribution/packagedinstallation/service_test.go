package packagedinstallation

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestEnsurePackagedFactories_PreparationFailureDoesNotCommitTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	definition := factorydefinitions.PackagedDefinition{
		Name: "@test/invalid",
		JSON: []byte(`{"id":"invalid","workers":[`),
	}
	_, err := newNativeTestInstaller(&installationPersistenceStub{prepareErr: errors.New("controlled preparation failure")}, platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{}).
		EnsurePackagedFactories(t.Context(), root, "", []factorydefinitions.PackagedDefinition{definition})
	if err == nil || !strings.Contains(err.Error(), "install packaged factory") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("committed entries = %v, want none", entries)
	}
}

func TestEnsurePackagedFactories_PreparationFailurePreservesExistingRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	marker := root + string(os.PathSeparator) + "customer-owned.txt"
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := factorydefinitions.PackagedDefinition{Name: "@test/invalid", JSON: []byte(`{`)}
	if _, err := newNativeTestInstaller(&installationPersistenceStub{prepareErr: errors.New("controlled preparation failure")}, platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{}).
		EnsurePackagedFactories(t.Context(), root, "", []factorydefinitions.PackagedDefinition{definition}); err == nil {
		t.Fatal("EnsurePackagedFactories() error = nil")
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "keep" {
		t.Fatalf("customer marker = %q, %v", content, err)
	}
}

func TestEnsurePackagedFactories_FailsClosedWithoutFileSystem(t *testing.T) {
	t.Parallel()

	_, err := newNativeTestInstaller(packagedInstallationTestPersistence(), nil, os.Mkdir, logging.NoopLogger{}).EnsurePackagedFactories(
		t.Context(),
		t.TempDir(),
		"",
		[]factorydefinitions.PackagedDefinition{{Name: "@test/missing-filesystem", JSON: []byte(`{}`)}},
	)
	if err == nil || !strings.Contains(err.Error(), "installation filesystem is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
}

func TestInstallPackagedFactory_DefaultsToJSONAndRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	installer := newNativeTestInstaller(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
	root := t.TempDir()
	result, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             "",
		},
	)
	if err != nil {
		t.Fatalf("default InstallPackagedFactory() error = %v", err)
	}
	if result.Format != factorydefinitions.PackagedFactoryFormatJSON {
		t.Fatalf("default format = %q", result.Format)
	}
	assertSingleAuthoredRoot(t, result.FactoryDir, "factory.json")

	unsupportedRoot := t.TempDir()
	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: unsupportedRoot,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormat("TOML"),
		},
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported packaged Factory format "TOML"`) {
		t.Fatalf("unsupported format error = %v", err)
	}
	entries, readErr := os.ReadDir(unsupportedRoot)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported format created entries: %v", entries)
	}
}

func TestInstallPackagedFactory_RepeatSkipsWithoutContentDrift(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	installer := newNativeTestInstaller(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil || created.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("initial InstallPackagedFactory() = %#v, %v", created, err)
	}
	marker := filepath.Join(created.FactoryDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotDirectoryContents(t, created.FactoryDir)

	skipped, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("repeat InstallPackagedFactory() error = %v", err)
	}
	if skipped.Outcome != factorydefinitions.PackagedFactoryInstallSkipped {
		t.Fatalf("repeat outcome = %q, want skipped", skipped.Outcome)
	}
	assertDirectorySnapshotUnchanged(t, created.FactoryDir, before)
}

func TestInstallPackagedFactory_ExplicitReplaceRestoresPackagedLayout(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	installer := newNativeTestInstaller(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	marker := filepath.Join(created.FactoryDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("replace-me"), 0o600); err != nil {
		t.Fatal(err)
	}

	replaced, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
			Replace:            true,
		},
	)
	if err != nil {
		t.Fatalf("replace InstallPackagedFactory() error = %v", err)
	}
	if replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replace outcome = %q, want replaced", replaced.Outcome)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("customer marker after replace = %v, want absent", statErr)
	}
}

func TestInstallPackagedFactory_RefusesAlternateFormatWithoutReplace(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	installer := newNativeTestInstaller(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
	root := t.TempDir()
	if _, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	); err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	_, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatYAML,
		},
	)
	if err == nil || !errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists) {
		t.Fatalf("alternate format InstallPackagedFactory() error = %v, want %v", err, factorydefinitions.ErrNamedFactoryAlreadyExists)
	}
}

func TestInstallPackagedFactory_CancellationBeforeCommitLeavesTargetAbsent(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := t.TempDir()
	_, err := newNativeTestInstaller(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{}).
		InstallPackagedFactory(
			ctx,
			factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition:         definition,
				Format:             factorydefinitions.PackagedFactoryFormatJSON,
			},
		)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("InstallPackagedFactory() error = %v, want cancellation", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("root entries after cancellation = %v, want none", entries)
	}
}

func TestInstallPackagedFactory_FailedReplacePreservesCommittedLayout(t *testing.T) {
	t.Parallel()

	definition := installationDefinitionFixture()
	persistence := &installationPersistenceStub{}
	installer := newNativeTestInstaller(persistence, platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	before := snapshotDirectoryContents(t, created.FactoryDir)
	persistence.prepareErr = errors.New("controlled replacement preparation failure")

	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
			Replace:            true,
		},
	)
	if err == nil {
		t.Fatal("replace with preparation failure error = nil")
	}
	assertDirectorySnapshotUnchanged(t, created.FactoryDir, before)
}

type directoryEntrySnapshot struct {
	Contents []byte
	Mode     fs.FileMode
	IsDir    bool
}

func snapshotDirectoryContents(t *testing.T, root string) map[string]directoryEntrySnapshot {
	t.Helper()
	snapshot := map[string]directoryEntrySnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := directoryEntrySnapshot{Mode: info.Mode(), IsDir: entry.IsDir()}
		if info.Mode().IsRegular() {
			value.Contents, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		snapshot[filepath.ToSlash(relative)] = value
		return nil
	}); err != nil {
		t.Fatalf("snapshot directory: %v", err)
	}
	return snapshot
}

func assertDirectorySnapshotUnchanged(t *testing.T, root string, before map[string]directoryEntrySnapshot) {
	t.Helper()
	after := snapshotDirectoryContents(t, root)
	if reflect.DeepEqual(before, after) {
		return
	}
	for path, want := range before {
		if got, ok := after[path]; !ok {
			t.Errorf("directory entry %q was removed", path)
		} else if !reflect.DeepEqual(want, got) {
			t.Errorf("directory entry %q changed: before=%#v after=%#v", path, want, got)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("directory entry %q was added", path)
		}
	}
}

func assertSingleAuthoredRoot(t *testing.T, factoryDir, want string) {
	t.Helper()
	for _, rootFile := range []string{"factory.json", "factory.yaml", "factory.yml"} {
		_, err := os.Stat(filepath.Join(factoryDir, rootFile))
		if rootFile == want {
			if err != nil {
				t.Fatalf("stat selected root %s: %v", rootFile, err)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected authored root %s: %v", rootFile, err)
		}
	}
}
