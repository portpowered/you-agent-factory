package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestBuildProcessFactoryListProjectsEffectiveCatalogWithoutRuntimeOrWrites(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	workingDirectory := t.TempDir()
	projectRoot := filepath.Join(workingDirectory, "factory")
	globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
	prepareRootListCatalog(t, projectRoot, globalRoot)
	before := rootListTreeSnapshot(t, home, workingDirectory)

	integration := &rootRecordingIntegration{identity: "catalog.test.provider"}
	var initializationCalls atomic.Int32
	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			initializationCalls.Add(1)
			return nil, errors.New("factory list must not initialize")
		},
		ProviderRegistrations: []inference.Registration{{
			Manifest:    rootExternalManifest(t, "catalog.test.provider", "catalog-test"),
			Integration: integration,
		}},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	entries, diagnostics := executeRootFactoryList(t, process, home, workingDirectory)
	assertRootEffectiveList(t, entries, diagnostics, projectRoot, globalRoot)
	after := rootListTreeSnapshot(t, home, workingDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("filesystem changed during listing\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertRootListEffects(t, initializationCalls.Load(), apiStarts, integration)
	assertRootCanceledList(t, process, home, workingDirectory)
}

type rootListEntry struct {
	Name              string `json:"name"`
	FactoryDirectory  string `json:"factoryDirectory"`
	Current           bool   `json:"current"`
	Description       string `json:"description"`
	InvocationExample string `json:"invocationExample"`
}

func executeRootFactoryList(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) ([]rootListEntry, string) {
	t.Helper()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "--json", "factory", "list"},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          context.Background(),
		WorkingDirectory: workingDirectory,
	}); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v", err)
	}
	var entries []rootListEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, output.String())
	}
	return entries, diagnostics.String()
}

func assertRootEffectiveList(
	t *testing.T,
	entries []rootListEntry,
	diagnostics string,
	projectRoot string,
	globalRoot string,
) {
	t.Helper()
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name
	}
	if !slices.IsSorted(names) {
		t.Fatalf("factory names = %v, want ascending order", names)
	}
	assertRootListEntry(t, entries, "aaa-local", filepath.Join(projectRoot, "aaa-local"), false, "Local description", true)
	assertRootListEntry(t, entries, "mmm-global", filepath.Join(globalRoot, "mmm-global"), false, "Global description", false)
	assertRootListEntry(t, entries, "zzz-shared", filepath.Join(projectRoot, "zzz-shared"), true, "Project override", true)
	if strings.Count(strings.Join(names, "\n"), "zzz-shared") != 1 {
		t.Fatalf("effective names = %v, want one shared entry", names)
	}
	packagedOnly := false
	for _, entry := range entries {
		if entry.FactoryDirectory == "-" {
			packagedOnly = true
			break
		}
	}
	if !packagedOnly {
		t.Fatalf("entries = %#v, want an unmaterialized packaged Factory", entries)
	}
	if !strings.Contains(diagnostics, "global broken (malformed)") ||
		strings.Contains(diagnostics, "do-not-leak") {
		t.Fatalf("diagnostics = %q, want safe malformed-entry context", diagnostics)
	}
}

func assertRootListEffects(
	t *testing.T,
	initializationCalls int32,
	apiStarts int,
	integration *rootRecordingIntegration,
) {
	t.Helper()
	if initializationCalls != 0 || apiStarts != 0 ||
		integration.discoverCalls != 0 || integration.capabilityCalls != 0 || integration.invokeCalls != 0 {
		t.Fatalf(
			"forbidden effects = initialization:%d api:%d provider:(%d,%d,%d)",
			initializationCalls,
			apiStarts,
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}
}

func assertRootCanceledList(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) {
	t.Helper()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := process.Execute(Input{
		Args:             []string{"you", "factory", "list"},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          canceled,
		WorkingDirectory: workingDirectory,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled factory list error = %v, want context canceled", err)
	}
	if output.Len() != 0 || diagnostics.String() != "Error: context canceled\n" {
		t.Fatalf(
			"canceled factory list output = (%q, %q), want no listing and standard cancellation diagnostic",
			output.String(),
			diagnostics.String(),
		)
	}
}

func prepareRootListCatalog(t *testing.T, projectRoot string, globalRoot string) {
	t.Helper()
	writeRootListFactory(t, projectRoot, "aaa-local", "Local description", true)
	writeRootListFactory(t, projectRoot, "zzz-shared", "Project override", true)
	writeRootListFactory(t, globalRoot, "mmm-global", "Global description", false)
	writeRootListFactory(t, globalRoot, "zzz-shared", "Shadowed global", false)
	writeRootListPayload(t, globalRoot, "broken", []byte(`{"secret":"do-not-leak"`))
	if err := os.WriteFile(filepath.Join(globalRoot, ".current-factory"), []byte("zzz-shared\n"), 0o600); err != nil {
		t.Fatalf("write current pointer: %v", err)
	}
}

func writeRootListFactory(
	t *testing.T,
	root string,
	name string,
	description string,
	signature bool,
) {
	t.Helper()
	definition := map[string]any{
		"name": name,
		"id":   name,
		"description": map[string]any{
			"type":  "LOCALIZABLE_ASSET",
			"value": description,
		},
		"workTypes":    []any{},
		"resources":    []any{},
		"workers":      []any{},
		"workstations": []any{},
	}
	if signature {
		definition["invocationSignature"] = map[string]any{
			"parameters": []map[string]any{{
				"name": "prompt", "externalName": "request", "required": true,
				"bindings": []map[string]any{{"kind": "NAMED"}},
			}},
		}
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	writeRootListPayload(t, root, name, payload)
}

func writeRootListPayload(t *testing.T, root string, name string, payload []byte) {
	t.Helper()
	factoryDirectory := filepath.Join(root, name)
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", factoryDirectory, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertRootListEntry(
	t *testing.T,
	entries []rootListEntry,
	name string,
	location string,
	current bool,
	description string,
	hasSignature bool,
) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if entry.FactoryDirectory != location || entry.Current != current ||
			entry.Description != description {
			t.Fatalf("entry %s = %#v", name, entry)
		}
		if hasSignature != (entry.InvocationExample != "") {
			t.Fatalf("entry %s invocation example = %q, signature=%t", name, entry.InvocationExample, hasSignature)
		}
		return
	}
	t.Fatalf("entry %s not found", name)
}

func rootListTreeSnapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := root + ":" + relative
			if entry.IsDir() {
				snapshot[key] = "directory"
				return nil
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot[key] = string(payload)
			return nil
		})
		if err != nil {
			t.Fatalf("snapshot %s: %v", root, err)
		}
	}
	return snapshot
}
