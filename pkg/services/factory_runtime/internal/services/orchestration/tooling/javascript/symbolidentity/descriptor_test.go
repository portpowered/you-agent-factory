package symbolidentity_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

func TestProjectInstalledBindings_CoversInstalledSurface(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()

	if inventory.FormatVersion != symbolidentity.FormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, symbolidentity.FormatVersion)
	}

	wantPaths := []string{
		"agent",
		"agent.run",
		"args",
		"log",
		"meta",
		"parallel",
		"phase",
		"pipeline",
		"workflow",
		"workflow.artifact",
		"workflow.budget",
		"workflow.checkpoint",
		"workflow.final",
		"workflow.log",
		"workflow.resumeState",
	}
	gotPaths := pathsFromInventory(inventory)
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("symbol paths = %v, want %v", gotPaths, wantPaths)
	}
}

func TestProjectInstalledBindings_SymbolsSortedByFullPath(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()
	gotPaths := pathsFromInventory(inventory)
	sorted := append([]string(nil), gotPaths...)
	slices.Sort(sorted)
	if !slices.Equal(gotPaths, sorted) {
		t.Fatalf("symbols not sorted by path: got %v, want %v", gotPaths, sorted)
	}
}

func TestProjectInstalledBindings_RecordShapeIncludesIdentitySubset(t *testing.T) {
	byPath := recordsByPath(symbolidentity.ProjectInstalledBindings())

	t.Run("args value", func(t *testing.T) {
		assertArgsValueRecord(t, byPath["args"])
	})
	t.Run("workflow namespace", func(t *testing.T) {
		assertWorkflowNamespaceRecord(t, byPath["workflow"])
	})
	t.Run("agent.run async callable", func(t *testing.T) {
		assertAgentRunAsyncCallableRecord(t, byPath["agent.run"])
	})
	t.Run("phase sync callable", func(t *testing.T) {
		assertPhaseSyncCallableRecord(t, byPath["phase"])
	})
}

func assertArgsValueRecord(t *testing.T, record symbolidentity.SymbolRecord) {
	t.Helper()
	if record.IDCandidate != "args" || record.Name != "args" || record.Kind != "value" {
		t.Fatalf("args record = %#v, want value identity", record)
	}
	if record.Callable || record.Async || record.Parent != "" || len(record.Members) > 0 {
		t.Fatalf("args record = %#v, want non-callable value without parent or members", record)
	}
}

func assertWorkflowNamespaceRecord(t *testing.T, record symbolidentity.SymbolRecord) {
	t.Helper()
	wantMembers := []string{"artifact", "budget", "checkpoint", "final", "log", "resumeState"}
	if record.Kind != "namespace" || !slices.Equal(record.Members, wantMembers) {
		t.Fatalf("workflow record = %#v, want namespace with members %v", record, wantMembers)
	}
}

func assertAgentRunAsyncCallableRecord(t *testing.T, record symbolidentity.SymbolRecord) {
	t.Helper()
	if record.Kind != "function" || record.Parent != "agent" || !record.Callable || !record.Async {
		t.Fatalf("agent.run record = %#v, want async callable function under agent", record)
	}
}

func assertPhaseSyncCallableRecord(t *testing.T, record symbolidentity.SymbolRecord) {
	t.Helper()
	if record.Kind != "function" || !record.Callable || record.Async {
		t.Fatalf("phase record = %#v, want sync callable function", record)
	}
}

func TestProjectInstalledBindings_DoesNotSerializeImplementationFields(t *testing.T) {
	raw, err := symbolidentity.MarshalInventory(symbolidentity.ProjectInstalledBindings())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}

	for _, forbidden := range []string{
		"parameters", "returns", "errors", "limits", "capabilities", "examples", "signature",
	} {
		if _, ok := decoded[forbidden]; ok {
			t.Fatalf("inventory root includes forbidden field %q", forbidden)
		}
	}

	symbols, ok := decoded["symbols"].([]any)
	if !ok {
		t.Fatalf("symbols = %#v, want array", decoded["symbols"])
	}
	for index, entry := range symbols {
		record, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("symbols[%d] = %#v, want object", index, entry)
		}
		for _, forbidden := range []string{
			"parameters", "returns", "errors", "limits", "capabilities", "examples", "signature",
			"implementation", "handler",
		} {
			if _, ok := record[forbidden]; ok {
				t.Fatalf("symbols[%d] includes forbidden field %q", index, forbidden)
			}
		}
	}
}

func TestProjectInstalledBindings_RepeatRunsAreByteIdentical(t *testing.T) {
	first, err := symbolidentity.MarshalInventory(symbolidentity.ProjectInstalledBindings())
	if err != nil {
		t.Fatalf("first MarshalInventory() error = %v", err)
	}
	second, err := symbolidentity.MarshalInventory(symbolidentity.ProjectInstalledBindings())
	if err != nil {
		t.Fatalf("second MarshalInventory() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeat descriptor runs differ:\nfirst  = %s\nsecond = %s", first, second)
	}
}

func recordsByPath(inventory symbolidentity.Inventory) map[string]symbolidentity.SymbolRecord {
	byPath := make(map[string]symbolidentity.SymbolRecord, len(inventory.Symbols))
	for _, record := range inventory.Symbols {
		byPath[record.Path] = record
	}
	return byPath
}

func pathsFromInventory(inventory symbolidentity.Inventory) []string {
	paths := make([]string, 0, len(inventory.Symbols))
	for _, record := range inventory.Symbols {
		paths = append(paths, record.Path)
	}
	return paths
}
