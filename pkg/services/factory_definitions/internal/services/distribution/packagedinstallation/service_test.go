package packagedinstallation

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func packagedInstallationTestPersistence() factorydefinitions.Persistence {
	validator := factoryvalidation.New(nil)
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	writer := factoryauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		factoryauthoredlayout.NewAgentsFileWriter(fileSystem),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		inboxgitkeep.NewLocal(fileSystem),
	)
	persistence, err := factorypersistence.New(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, factorydefinitioncomposition.LoadCanonicalJSON)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return authoringlayoutprepare.FactoryLayout(
				ctx,
				segment,
				payload,
				validator,
				mapper.Expand,
				authoredmapping.AuthoredFactoryConfigForExpandedLayout,
				mapper.Flatten,
			)
		},
		func(
			targetDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				portableconfig.NewMaterializer(platformfilesystem.Local{}),
				factorydefinitioncomposition.PruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
			return err
		},
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		factorydefinitioncomposition.NamedPaths().RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func TestEnsurePackagedFactories_InvalidPayloadDoesNotCommitTarget(t *testing.T) {
	root := t.TempDir()
	definition := factorydefinitions.PackagedDefinition{
		Name: "@test/invalid",
		JSON: []byte(`{"id":"invalid","workers":[`),
	}
	_, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(t.Context(), root, []factorydefinitions.PackagedDefinition{definition})
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
	root := t.TempDir()
	marker := root + string(os.PathSeparator) + "customer-owned.txt"
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := factorydefinitions.PackagedDefinition{Name: "@test/invalid", JSON: []byte(`{`)}
	if _, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(t.Context(), root, []factorydefinitions.PackagedDefinition{definition}); err == nil {
		t.Fatal("EnsurePackagedFactories() error = nil")
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "keep" {
		t.Fatalf("customer marker = %q, %v", content, err)
	}
}

func TestEnsurePackagedFactories_FailsClosedWithoutFileSystem(t *testing.T) {
	_, err := New(packagedInstallationTestPersistence(), nil).EnsurePackagedFactories(
		t.Context(),
		t.TempDir(),
		[]factorydefinitions.PackagedDefinition{{Name: "@test/missing-filesystem", JSON: []byte(`{}`)}},
	)
	if err == nil || !strings.Contains(err.Error(), "installation filesystem is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
}

func TestInstallPackagedFactory_MaterializesPortableEditableFormats(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/deep-research")
	if !ok {
		t.Fatal("published catalog is missing @you/deep-research")
	}
	tests := []struct {
		format   factorydefinitions.PackagedFactoryFormat
		rootFile string
	}{
		{format: factorydefinitions.PackagedFactoryFormatJSON, rootFile: "factory.json"},
		{format: factorydefinitions.PackagedFactoryFormatYAML, rootFile: "factory.yaml"},
		{format: factorydefinitions.PackagedFactoryFormatYML, rootFile: "factory.yml"},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.format), func(t *testing.T) {
			root := t.TempDir()
			result, installErr := New(
				packagedInstallationTestPersistence(),
				platformfilesystem.Local{},
			).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition:         definition,
				Format:             test.format,
			})
			if installErr != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", installErr)
			}
			if result.Outcome != factorydefinitions.PackagedFactoryInstallCreated ||
				result.Format != test.format {
				t.Fatalf("InstallPackagedFactory() = %#v", result)
			}
			assertSingleAuthoredRoot(t, result.FactoryDir, test.rootFile)
			assertDeepResearchScript(t, definition, result.FactoryDir)
			assertPortableMaterializedContent(t, result.FactoryDir)
			assertCustomerEditIsLoaded(t, result.FactoryDir, test.rootFile)
		})
	}
}

func TestInstallPackagedFactory_DefaultsToJSONAndRejectsUnsupportedFormat(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{})
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

func TestInstallPackagedFactory_MaterializesEveryPublishedFactory(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	for _, definition := range catalog.All() {
		definition := definition
		t.Run(definition.Name, func(t *testing.T) {
			result, installErr := New(
				packagedInstallationTestPersistence(),
				platformfilesystem.Local{},
			).InstallPackagedFactory(
				t.Context(),
				factorydefinitions.PackagedFactoryInstallParams{
					NamedFactoriesRoot: t.TempDir(),
					Definition:         definition,
					Format:             factorydefinitions.PackagedFactoryFormatJSON,
				},
			)
			if installErr != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", installErr)
			}
			if result.Name != definition.Name ||
				result.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
				t.Fatalf("InstallPackagedFactory() = %#v", result)
			}
			if _, loadErr := factorydefinitioncomposition.LoadDirectory(
				result.FactoryDir,
				nil,
			); loadErr != nil {
				t.Fatalf("load materialized Factory: %v", loadErr)
			}
		})
	}
}

func TestInstallPackagedFactory_RepeatSkipsWithoutContentDrift(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{})
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
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{})
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
	if _, loadErr := factorydefinitioncomposition.LoadDirectory(replaced.FactoryDir, nil); loadErr != nil {
		t.Fatalf("load replaced Factory: %v", loadErr)
	}
}

func TestInstallPackagedFactory_RefusesAlternateFormatWithoutReplace(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{})
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
	_, err = installer.InstallPackagedFactory(
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
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := t.TempDir()
	_, err = New(packagedInstallationTestPersistence(), platformfilesystem.Local{}).
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
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{})
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
	invalid := definition
	invalid.JSON = []byte(`{"name":"broken","workers":[`)

	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         invalid,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
			Replace:            true,
		},
	)
	if err == nil {
		t.Fatal("replace with invalid payload error = nil")
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
	if _, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil); err != nil {
		t.Fatalf("load materialized Factory: %v", err)
	}
}

func assertDeepResearchScript(
	t *testing.T,
	definition factorydefinitions.PackagedDefinition,
	factoryDir string,
) {
	t.Helper()
	var published struct {
		SupportingFiles struct {
			BundledFiles []struct {
				TargetPath string `json:"targetPath"`
				Content    struct {
					Inline string `json:"inline"`
				} `json:"content"`
			} `json:"bundledFiles"`
		} `json:"supportingFiles"`
	}
	if err := json.Unmarshal(definition.JSON, &published); err != nil {
		t.Fatalf("decode published definition: %v", err)
	}
	if len(published.SupportingFiles.BundledFiles) == 0 {
		t.Fatal("published definition has no bundled assets")
	}
	for _, bundled := range published.SupportingFiles.BundledFiles {
		relativePath := strings.TrimPrefix(bundled.TargetPath, "factory/")
		content, err := os.ReadFile(filepath.Join(factoryDir, relativePath))
		if err != nil {
			t.Fatalf("read materialized asset %s: %v", relativePath, err)
		}
		if string(content) != bundled.Content.Inline {
			t.Fatalf("materialized asset %s differs from published content", relativePath)
		}
	}
}

func assertPortableMaterializedContent(t *testing.T, factoryDir string) {
	t.Helper()
	err := filepath.WalkDir(factoryDir, func(
		path string,
		entry os.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, forbidden := range []string{
			"packages/packaged-factories",
			"generated/factories",
			"node_modules",
			"npm ",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("%s contains non-portable reference %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk materialized Factory: %v", err)
	}
}

func assertCustomerEditIsLoaded(t *testing.T, factoryDir, rootFile string) {
	t.Helper()
	path := filepath.Join(factoryDir, rootFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored root: %v", err)
	}
	var edited []byte
	if rootFile == "factory.json" {
		edited = []byte(strings.Replace(
			string(content),
			`"name": "deep-research"`,
			`"name": "customer-edited"`,
			1,
		))
	} else {
		edited = []byte(strings.Replace(
			string(content),
			"\nname: deep-research\n",
			"\nname: customer-edited\n",
			1,
		))
	}
	if string(edited) == string(content) {
		t.Fatalf("could not locate editable name in %s", rootFile)
	}
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("write customer edit: %v", err)
	}
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("load customer-edited Factory: %v", err)
	}
	if loaded.FactoryConfig().Name != "customer-edited" {
		t.Fatalf("loaded name = %q, want customer-edited", loaded.FactoryConfig().Name)
	}
}
