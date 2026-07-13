package factory

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestList_WritesHumanReadableTable(t *testing.T) {
	rootDir := setupNamedFactoriesForListTest(t, []string{"alpha", "beta"}, "beta")

	var out strings.Builder
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
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
	if err := List(ListConfig{Dir: rootDir, JSON: true, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}

	var entries []factoryconfig.NamedFactoryListEntry
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
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "No factories found.\n" {
		t.Fatalf("output = %q, want empty-state output", got)
	}
}

func TestList_InvalidRootFailsBeforePrinting(t *testing.T) {
	var out strings.Builder
	err := List(ListConfig{Dir: filepath.Join(t.TempDir(), "missing"), Output: &out})
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
	for _, name := range names {
		if _, err := factoryconfig.PersistNamedFactory(rootDir, name, listTestNamedFactoryPayload(t, name)); err != nil {
			t.Fatalf("PersistNamedFactory(%s): %v", name, err)
		}
	}
	if current != "" {
		if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, current); err != nil {
			t.Fatalf("WriteCurrentFactoryPointer: %v", err)
		}
	}
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
