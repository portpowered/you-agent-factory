package splitreplacetests

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func TestNamedFactoryPersistenceJSONAndYAMLParity(t *testing.T) {
	t.Parallel()

	jsonRoot := t.TempDir()
	yamlRoot := t.TempDir()
	jsonSource := writePersistenceSource(
		t,
		filepath.Join(t.TempDir(), "authored.json"),
		persistenceParityJSON("created"),
	)
	yamlSource := writePersistenceSource(
		t,
		filepath.Join(t.TempDir(), "authored.yml"),
		persistenceParityYAML("created"),
	)

	persistAuthoredFactory(t, jsonRoot, "alpha", jsonSource, factorydefinitions.NamedFactoryPersistenceModeCreate)
	persistAuthoredFactory(t, yamlRoot, "alpha", yamlSource, factorydefinitions.NamedFactoryPersistenceModeCreate)
	requireEquivalentFactoryLayouts(t, filepath.Join(jsonRoot, "alpha"), filepath.Join(yamlRoot, "alpha"))

	jsonUpdate := writePersistenceSource(
		t,
		filepath.Join(t.TempDir(), "replacement.json"),
		persistenceParityJSON("updated"),
	)
	yamlUpdate := writePersistenceSource(
		t,
		filepath.Join(t.TempDir(), "replacement.yaml"),
		persistenceParityYAML("updated"),
	)
	persistAuthoredFactory(t, jsonRoot, "alpha", jsonUpdate, factorydefinitions.NamedFactoryPersistenceModeReplace)
	persistAuthoredFactory(t, yamlRoot, "alpha", yamlUpdate, factorydefinitions.NamedFactoryPersistenceModeReplace)
	requireEquivalentFactoryLayouts(t, filepath.Join(jsonRoot, "alpha"), filepath.Join(yamlRoot, "alpha"))

	for _, root := range []string{jsonRoot, yamlRoot} {
		factoryDir := filepath.Join(root, "alpha")
		if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
			t.Fatalf("canonical factory.json was not persisted in %s: %v", factoryDir, err)
		}
		if _, err := os.Stat(filepath.Join(factoryDir, "factory.yaml")); !os.IsNotExist(err) {
			t.Fatalf("unexpected YAML persistence model in %s: %v", factoryDir, err)
		}
		for relativePath, want := range map[string]string{
			"scripts/helper.sh":                "echo portable\n",
			"inputs/task/request/payload.json": `{"title":"starter"}`,
		} {
			data, err := os.ReadFile(filepath.Join(factoryDir, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatalf("read materialized %s: %v", relativePath, err)
			}
			if string(data) != want {
				t.Fatalf("%s = %q, want %q", relativePath, data, want)
			}
		}
		loaded, err := factorydefinitioncomposition.LoadedFactoryLoader(factoryDir, nil)
		if err != nil {
			t.Fatalf("load persisted factory %s: %v", factoryDir, err)
		}
		if loaded.FactoryConfig().Project != "updated" {
			t.Fatalf("persisted project = %q, want updated", loaded.FactoryConfig().Project)
		}
	}
}

func TestRejectedYAMLDoesNotCreateOrReplaceNamedFactory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	validSource := writePersistenceSource(
		t,
		filepath.Join(t.TempDir(), "factory.json"),
		persistenceParityJSON("original"),
	)
	persistAuthoredFactory(t, rootDir, "alpha", validSource, factorydefinitions.NamedFactoryPersistenceModeCreate)
	before := readFactoryLayout(t, filepath.Join(rootDir, "alpha"))

	load := factoryauthoredlayout.NewFactorySourceLoader(platformfilesystem.Local{})
	for _, test := range []struct {
		name string
		mode factorydefinitions.NamedFactoryPersistenceMode
		body string
	}{
		{
			name: "create",
			mode: factorydefinitions.NamedFactoryPersistenceModeCreate,
			body: "name: broken\nname: duplicate\n",
		},
		{
			name: "replace",
			mode: factorydefinitions.NamedFactoryPersistenceModeReplace,
			body: "name: [\n",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			sourcePath := writePersistenceSource(
				t,
				filepath.Join(t.TempDir(), "factory.yaml"),
				test.body,
			)
			if _, err := load(sourcePath); err == nil {
				t.Fatal("strict YAML load error = nil")
			}
			if test.mode == factorydefinitions.NamedFactoryPersistenceModeCreate {
				if _, err := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(err) {
					t.Fatalf("rejected YAML created a target: %v", err)
				}
			}
			after := readFactoryLayout(t, filepath.Join(rootDir, "alpha"))
			if !reflect.DeepEqual(after, before) {
				t.Fatal("rejected YAML changed the existing named Factory")
			}
		})
	}
}

func persistAuthoredFactory(
	t *testing.T,
	rootDir string,
	name string,
	sourcePath string,
	mode factorydefinitions.NamedFactoryPersistenceMode,
) {
	t.Helper()
	load := factoryauthoredlayout.NewFactorySourceLoader(platformfilesystem.Local{})
	source, err := load(sourcePath)
	if err != nil {
		t.Fatalf("load authored source %s: %v", sourcePath, err)
	}
	persistence := factorydefinitioncomposition.FactoryDefinitionPersistenceWithValidator(
		factoryvalidation.New(nil),
	)
	result, err := persistence.PersistNamedFactory(
		context.Background(),
		factorydefinitions.NamedFactoryPersistenceRequest{
			Mode:    mode,
			RootDir: rootDir,
			Name:    name,
			Payload: source.Data,
		},
	)
	if err != nil {
		t.Fatalf("persist %s from %s: %v", mode, sourcePath, err)
	}
	if result.FactoryDir != filepath.Join(rootDir, name) {
		t.Fatalf("factory dir = %q, want %q", result.FactoryDir, filepath.Join(rootDir, name))
	}
}

func requireEquivalentFactoryLayouts(t *testing.T, left, right string) {
	t.Helper()
	leftFiles := readFactoryLayout(t, left)
	rightFiles := readFactoryLayout(t, right)
	if !reflect.DeepEqual(leftFiles, rightFiles) {
		t.Fatalf("persisted layouts differ:\nJSON: %#v\nYAML: %#v", leftFiles, rightFiles)
	}
}

func readFactoryLayout(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read Factory layout %s: %v", root, err)
	}
	return files
}

func writePersistenceSource(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write source %s: %v", path, err)
	}
	return path
}

func persistenceParityJSON(project string) string {
	return `{
  "name": "parity",
  "id": "` + project + `",
  "description": {
    "type": "LOCALIZABLE_ASSET",
    "value": "Factory",
    "locales": ["fr-FR"],
    "values": {"fr-FR": "Fabrique"}
  },
  "examples": [{
    "name": "multiline",
    "description": {"type": "LOCALIZABLE_ASSET", "value": "Example"},
    "args": {"prompt": "first line\nsecond line\n"}
  }],
  "supportingFiles": {
    "bundledFiles": [
      {
        "type": "SCRIPT",
        "targetPath": "factory/scripts/helper.sh",
        "content": {"encoding": "utf-8", "inline": "echo portable\n"}
      },
      {
        "type": "INPUT",
        "targetPath": "factory/inputs/task/request/payload.json",
        "content": {"encoding": "utf-8", "inline": "{\"title\":\"starter\"}"}
      }
    ]
  },
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "executor",
    "type": "MODEL_WORKER",
    "body": "You are the executor."
  }],
  "workstations": [{
    "name": "execute",
    "worker": "executor",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION",
    "body": "Implement {{ .WorkID }}."
  }]
}`
}

func persistenceParityYAML(project string) string {
	return strings.TrimSpace(`
name: parity
id: `+project+`
description:
  type: LOCALIZABLE_ASSET
  value: Factory
  locales: [fr-FR]
  values:
    fr-FR: Fabrique
examples:
  - name: multiline
    description:
      type: LOCALIZABLE_ASSET
      value: Example
    args:
      prompt: |
        first line
        second line
supportingFiles:
  bundledFiles:
    - type: SCRIPT
      targetPath: factory/scripts/helper.sh
      content:
        encoding: utf-8
        inline: |
          echo portable
    - type: INPUT
      targetPath: factory/inputs/task/request/payload.json
      content:
        encoding: utf-8
        inline: '{"title":"starter"}'
workTypes:
  - name: task
    states:
      - {name: init, type: INITIAL}
      - {name: complete, type: TERMINAL}
      - {name: failed, type: FAILED}
workers:
  - name: executor
    type: MODEL_WORKER
    body: You are the executor.
workstations:
  - name: execute
    worker: executor
    inputs:
      - {workType: task, state: init}
    outputs:
      - {workType: task, state: complete}
    onFailure:
      - {workType: task, state: failed}
    type: MODEL_WORKSTATION
    body: Implement {{ .WorkID }}.
`) + "\n"
}
