package systeminitializationtests

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation"
	factorypackages "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

type directoryEntrySnapshot struct {
	Contents []byte
	Mode     fs.FileMode
	IsDir    bool
}

func packagedScriptsTestPersistence() interfaces.Persistence {
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
		func(payload []byte) (interfaces.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, factorydefinitioncomposition.LoadCanonicalJSON)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator interfaces.Validator,
		) (*interfaces.PreparedFactoryLayoutPayload, error) {
			return factoryauthoredlayout.Prepare(
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
			prepared *interfaces.PreparedFactoryLayoutPayload,
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
	if after := snapshotDirectoryContents(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("directory changed: before=%#v after=%#v", before, after)
	}
}

func TestEnsurePackagedFactories_InstallsAssembledScriptsThinExecutableAndPreservesEdits(t *testing.T) {
	t.Parallel()

	definition := assembledScriptPackageDefinition(t)
	homeDir := t.TempDir()
	created, err := packagedinstallation.New(packagedScriptsTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(
			t.Context(),
			interfaces.NamedFactoriesRoot(homeDir),
			[]factorypackages.Definition{definition},
		)
	if err != nil {
		t.Fatalf("Initialize(create): %v", err)
	}
	if len(created) != 1 || created[0].Outcome != interfaces.PackagedFactoryInstallCreated {
		t.Fatalf("create results = %#v, want one created package", created)
	}

	factoryDir := created[0].FactoryDir
	assertInstalledPackagedScript(t, factoryDir, "scripts/setup.sh", "#!/bin/sh\nprintf 'packaged setup\\n'\n")
	assertInstalledPackagedScript(t, factoryDir, "scripts/nested/check.py", "print('nested packaged script')\n")
	assertThinPackagedScriptManifest(t, factoryDir)
	assertInstalledPackagedScriptRuntimeConfig(t, factoryDir)

	editedPath := filepath.Join(factoryDir, "scripts", "nested", "check.py")
	if err := os.WriteFile(editedPath, []byte("print('operator edit')\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(operator-edited script): %v", err)
	}
	if err := os.Chmod(editedPath, 0o600); err != nil {
		t.Fatalf("Chmod(operator-edited script): %v", err)
	}
	beforeRerun := snapshotDirectoryContents(t, factoryDir)

	skipped, err := packagedinstallation.New(packagedScriptsTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(
			t.Context(),
			interfaces.NamedFactoriesRoot(homeDir),
			[]factorypackages.Definition{definition},
		)
	if err != nil {
		t.Fatalf("Initialize(rerun): %v", err)
	}
	if len(skipped) != 1 || skipped[0].Outcome != interfaces.PackagedFactoryInstallSkipped {
		t.Fatalf("rerun results = %#v, want one skipped package", skipped)
	}
	assertDirectorySnapshotUnchanged(t, factoryDir, beforeRerun)
}

func assembledScriptPackageDefinition(t *testing.T) factorypackages.Definition {
	t.Helper()

	payload, err := packageassets.Assemble(packageassets.Definition{
		Package: "@test/scripts",
		FactoryJSON: []byte(`{
  "name":"packaged-script-fixture",
  "workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers":[{"name":"runner","type":"SCRIPT_WORKER","command":"scripts/setup.sh"}],
  "workstations":[{"name":"run-script","type":"MODEL_WORKSTATION","body":"Run the packaged script.","worker":"runner","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]
}`),
		Assets: fstest.MapFS{
			"scripts/setup.sh":        {Data: []byte("#!/bin/sh\nprintf 'packaged setup\\n'\n"), Mode: 0o600},
			"scripts/nested/check.py": {Data: []byte("print('nested packaged script')\n"), Mode: 0o400},
		},
	})
	if err != nil {
		t.Fatalf("packageassets.Assemble: %v", err)
	}
	return factorypackages.Definition{Name: "@test/scripts", Project: "packaged-script-fixture", JSON: payload}
}

func assertInstalledPackagedScript(t *testing.T, factoryDir, relativePath, wantContent string) {
	t.Helper()

	path := filepath.Join(factoryDir, filepath.FromSlash(relativePath))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", relativePath, err)
	}
	if string(content) != wantContent {
		t.Fatalf("%s content = %q, want %q", relativePath, content, wantContent)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", relativePath, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("%s mode = %04o, want 0755", relativePath, info.Mode().Perm())
	}
}

func assertThinPackagedScriptManifest(t *testing.T, factoryDir string) {
	t.Helper()

	payload, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(persisted factory.json): %v", err)
	}
	var authored map[string]any
	if err := json.Unmarshal(payload, &authored); err != nil {
		t.Fatalf("Unmarshal(persisted factory.json): %v", err)
	}
	supportingFiles, ok := authored["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("persisted supportingFiles = %#v, want object", authored["supportingFiles"])
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 2 {
		t.Fatalf("persisted bundledFiles = %#v, want two entries", supportingFiles["bundledFiles"])
	}
	for _, value := range bundledFiles {
		assertThinPackagedScriptEntry(t, value)
	}
}

func assertThinPackagedScriptEntry(t *testing.T, value any) {
	t.Helper()

	entry, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("persisted bundled file = %#v, want object", value)
	}
	target, _ := entry["targetPath"].(string)
	if entry["type"] != interfaces.BundledFileTypeScript || !strings.HasPrefix(target, "factory/scripts/") {
		t.Fatalf("persisted bundled file = %#v, want canonical SCRIPT target", entry)
	}
	content, ok := entry["content"].(map[string]any)
	if !ok || content["encoding"] != interfaces.BundledFileEncodingUTF8 {
		t.Fatalf("persisted bundled content = %#v, want UTF-8 metadata", entry["content"])
	}
	if _, exists := content["inline"]; exists {
		t.Fatalf("persisted bundled content = %#v, want no inline body", content)
	}
}

func assertInstalledPackagedScriptRuntimeConfig(t *testing.T, factoryDir string) {
	t.Helper()

	loaded, err := factorydefinitioncomposition.LoadedFactoryLoader(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(installed package): %v", err)
	}
	worker, ok := loaded.Worker("runner")
	if !ok {
		t.Fatal("installed package runtime config missing script worker")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "scripts/setup.sh" {
		t.Fatalf("installed script worker = %#v, want relative SCRIPT_WORKER command", worker)
	}
	manifest := loaded.FactoryConfig().ResourceManifest
	if manifest == nil || len(manifest.BundledFiles) != 2 {
		t.Fatalf("loaded thin bundled manifest = %#v, want two entries", manifest)
	}
}
