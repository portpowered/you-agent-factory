package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestList_RendersEffectiveCatalogMetadataDiagnosticsAndPackagedLocation(t *testing.T) {
	projectLocation := filepath.Join("project", "factory", "alpha")
	catalog := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{
			Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
				{
					Name:     "alpha",
					Location: &projectLocation,
					Definition: &factorydefinitions.FactoryConfig{
						Description: &factorydefinitions.NameValueConfig{Value: "Project Alpha"},
					},
					InvocationSignature: &factorydefinitions.InvocationSignatureConfig{
						Parameters: []factorydefinitions.InvocationParameterConfig{{
							Name: "prompt", ExternalName: "request", Required: true,
							Bindings: []factorydefinitions.InvocationParameterBindingConfig{{
								Kind: work.InvocationParameterBindingKindNamed,
							}},
						}},
					},
				},
				{
					Name:       "zeta-built-in",
					Definition: &factorydefinitions.FactoryConfig{},
				},
			},
			Diagnostics: []factorydefinitions.EffectiveFactoryCatalogDiagnostic{{
				Source:  factorydefinitions.EffectiveFactoryCatalogSourceGlobal,
				Name:    "broken",
				Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
				Message: "Factory definition is malformed",
			}},
		}, nil
	}
	readCurrent := func(root string) (string, error) {
		if root == "project-root" {
			return "alpha", nil
		}
		return "", fs.ErrNotExist
	}

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := List(catalog, readCurrent, ListConfig{
		Context: context.Background(), ProjectRoot: "project-root", GlobalRoot: "global-root",
		Output: &output, Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := "NAME\tFACTORY DIRECTORY\tCURRENT\n" +
		"alpha\t" + projectLocation + "\tyes\n" +
		"zeta-built-in\t-\t\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if got := diagnostics.String(); got !=
		"Factory catalog global broken (malformed): Factory definition is malformed\n" {
		t.Fatalf("diagnostics = %q", got)
	}

	var jsonOutput bytes.Buffer
	err = List(catalog, readCurrent, ListConfig{
		Context: context.Background(), ProjectRoot: "project-root", GlobalRoot: "global-root",
		JSON: true, Output: &jsonOutput,
	})
	if err != nil {
		t.Fatalf("List(JSON) error = %v", err)
	}
	var entries []ListEntry
	if err := json.Unmarshal(jsonOutput.Bytes(), &entries); err != nil {
		t.Fatalf("decode JSON list: %v", err)
	}
	if entries[0].Description != "Project Alpha" ||
		entries[0].InvocationExample != "you run --named alpha --request <prompt>" {
		t.Fatalf("effective metadata = %#v", entries[0])
	}
	if entries[1].FactoryDirectory != absentFactoryLocation ||
		entries[1].InvocationExample != "" {
		t.Fatalf("packaged metadata = %#v", entries[1])
	}
}

func TestList_GlobalCurrentMarksEffectiveProjectOverride(t *testing.T) {
	location := filepath.Join("project", "factory", "shared")
	catalog := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{
			Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
				Name: "shared", Location: &location, Definition: &factorydefinitions.FactoryConfig{},
			}},
		}, nil
	}
	readCurrent := func(root string) (string, error) {
		if root == "project-root" {
			return "", fs.ErrNotExist
		}
		return "shared", nil
	}

	var output bytes.Buffer
	err := List(catalog, readCurrent, ListConfig{
		Context: context.Background(), ProjectRoot: "project-root", GlobalRoot: "global-root",
		Output: &output,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "shared\t"+location+"\tyes\n") {
		t.Fatalf("output = %q, want effective project row marked current", got)
	}
}

func TestList_CancellationEmitsNoPartialOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	catalog := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		cancel()
		return factorydefinitions.ListEffectiveFactoriesResult{
			Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
				Name: "must-not-render", Definition: &factorydefinitions.FactoryConfig{},
			}},
			Diagnostics: []factorydefinitions.EffectiveFactoryCatalogDiagnostic{{
				Message: "must not render",
			}},
		}, nil
	}
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := List(catalog, func(string) (string, error) {
		return "", fs.ErrNotExist
	}, ListConfig{
		Context: ctx, ProjectRoot: "project-root", GlobalRoot: "global-root",
		Output: &output, Diagnostics: &diagnostics,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context canceled", err)
	}
	if output.Len() != 0 || diagnostics.Len() != 0 {
		t.Fatalf("canceled output = (%q, %q), want atomic empty output", output.String(), diagnostics.String())
	}
}

func TestList_WritesHumanReadableTable(t *testing.T) {
	rootDir := setupNamedFactoriesForListTest(t, []string{"alpha", "beta"}, "beta")

	var out strings.Builder
	if err := testList(ListConfig{ProjectRoot: rootDir, GlobalRoot: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	betaDir := filepath.Join(rootDir, "beta")
	want := "NAME\tFACTORY DIRECTORY\tCURRENT\n" +
		"alpha\t" + alphaDir + "\t\n" +
		"beta\t" + betaDir + "\tyes\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_JSONEmitsArray(t *testing.T) {
	rootDir := setupNamedFactoriesForListTest(t, []string{"alpha"}, "")

	var out bytes.Buffer
	if err := testList(ListConfig{ProjectRoot: rootDir, GlobalRoot: rootDir, JSON: true, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}

	var entries []factorydefinitions.NamedFactoryListEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "alpha" || entries[0].Current {
		t.Fatalf("entries = %#v, want one non-current alpha entry", entries)
	}
}

func TestList_EmptyRootPrintsEmptyState(t *testing.T) {
	rootDir := t.TempDir()

	var out strings.Builder
	if err := testList(ListConfig{ProjectRoot: rootDir, GlobalRoot: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "No factories found.\n" {
		t.Fatalf("output = %q, want empty-state output", got)
	}
}

func TestList_InvalidRootFailsBeforePrinting(t *testing.T) {
	wantErr := errors.New("list named factories")
	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		list: func(string) ([]factorydefinitions.NamedFactoryListEntry, error) {
			return nil, wantErr
		},
	})
	var out strings.Builder
	missing := filepath.Join(t.TempDir(), "missing")
	err := testList(ListConfig{ProjectRoot: missing, GlobalRoot: missing, Output: &out})
	if err == nil {
		t.Fatal("expected invalid factory root to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no output on failure", out.String())
	}
}

func setupNamedFactoriesForListTest(t *testing.T, names []string, current string) string {
	t.Helper()

	rootDir := t.TempDir()
	entries := make([]factorydefinitions.NamedFactoryListEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, factorydefinitions.NamedFactoryListEntry{
			Name:       name,
			FactoryDir: filepath.Join(rootDir, name),
			Current:    name == current,
		})
	}
	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		list: func(string) ([]factorydefinitions.NamedFactoryListEntry, error) {
			return append([]factorydefinitions.NamedFactoryListEntry(nil), entries...), nil
		},
	})
	return rootDir
}

func listTestNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()

	cfg := map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-" + project,
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
				"type":      "MODEL_WORKSTATION",
				"body":      "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(listTestNamedFactoryPayload): %v", err)
	}
	return data
}

func writeFactoryConfigFile(t *testing.T, dir, stem string, payload []byte) string {
	t.Helper()

	path := filepath.Join(dir, stem+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func saveTestNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return listTestNamedFactoryPayload(t, project)
}

func ioDiscard(t *testing.T) *strings.Builder {
	t.Helper()
	return &strings.Builder{}
}

func currentFactorySaveServer(t *testing.T, current factoryapi.Factory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet, http.MethodPut:
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
}
