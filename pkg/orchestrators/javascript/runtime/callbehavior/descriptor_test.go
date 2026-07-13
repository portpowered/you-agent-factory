package callbehavior_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/symbolidentity"
)

func TestProjectInstalledCallBehavior_CoversInstalledSurface(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()

	if inventory.FormatVersion != callbehavior.FormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, callbehavior.FormatVersion)
	}

	wantPaths := symbolidentity.ExpectedInstalledPaths()
	gotPaths := pathsFromInventory(inventory)
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("record paths = %v, want %v", gotPaths, wantPaths)
	}
}

func TestProjectInstalledCallBehavior_RecordsSortedByFullPath(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()
	gotPaths := pathsFromInventory(inventory)
	sorted := append([]string(nil), gotPaths...)
	slices.Sort(sorted)
	if !slices.Equal(gotPaths, sorted) {
		t.Fatalf("records not sorted by path: got %v, want %v", gotPaths, sorted)
	}
}

func TestProjectInstalledCallBehavior_RecordShapeIncludesCallBehaviorSubset(t *testing.T) {
	byPath := recordsByPath(callbehavior.ProjectInstalledCallBehavior())

	t.Run("args value lifecycle", func(t *testing.T) {
		record := byPath["args"]
		if record.Kind != "value" || record.Mutability == "" || record.Lifecycle == "" {
			t.Fatalf("args record = %#v, want value with mutability and lifecycle", record)
		}
		if record.Callable || len(record.Parameters) > 0 {
			t.Fatalf("args record = %#v, want non-callable value", record)
		}
	})

	t.Run("workflow namespace lifecycle", func(t *testing.T) {
		record := byPath["workflow"]
		if record.Kind != "namespace" || record.Lifecycle != "live-namespace" {
			t.Fatalf("workflow record = %#v, want live namespace", record)
		}
	})

	t.Run("workflow.final callable", func(t *testing.T) {
		record := byPath["workflow.final"]
		if !record.Callable || record.Async || record.Return == nil {
			t.Fatalf("workflow.final record = %#v, want sync callable with return behavior", record)
		}
		if record.Return.SyncType != "undefined" {
			t.Fatalf("workflow.final return = %#v, want undefined sync return", record.Return)
		}
		if record.Determinism == "" {
			t.Fatal("workflow.final missing determinism note")
		}
	})

	t.Run("workflow.checkpoint emits checkpoint", func(t *testing.T) {
		record := byPath["workflow.checkpoint"]
		if !slices.Contains(record.EmittedRecords, "checkpoint") {
			t.Fatalf("workflow.checkpoint emittedRecords = %v, want checkpoint", record.EmittedRecords)
		}
		if len(record.Parameters) != 1 || !record.Parameters[0].Required {
			t.Fatalf("workflow.checkpoint parameters = %#v, want one required object", record.Parameters)
		}
	})

	t.Run("workflow.resumeState resume notes", func(t *testing.T) {
		record := byPath["workflow.resumeState"]
		if record.ResumeNotes == "" {
			t.Fatal("workflow.resumeState missing resume notes")
		}
		if len(record.Parameters) != 0 {
			t.Fatalf("workflow.resumeState parameters = %#v, want zero parameters", record.Parameters)
		}
	})

	t.Run("agent.run async promise", func(t *testing.T) {
		record := byPath["agent.run"]
		if !record.Callable || !record.Async || record.Return == nil || !record.Return.Async {
			t.Fatalf("agent.run record = %#v, want async callable with promise return", record)
		}
		if !slices.Contains(record.EmittedRecords, "child_dispatch") {
			t.Fatalf("agent.run emittedRecords = %v, want child_dispatch", record.EmittedRecords)
		}
		if len(record.PolicyChecks) == 0 {
			t.Fatal("agent.run missing policy checks")
		}
	})

	t.Run("parallel callback and promise", func(t *testing.T) {
		record := byPath["parallel"]
		if record.Callback == nil || record.Return == nil || !record.Return.Async {
			t.Fatalf("parallel record = %#v, want callback shape and promise return", record)
		}
	})

	t.Run("pipeline callback stages", func(t *testing.T) {
		record := byPath["pipeline"]
		if record.Callback == nil || len(record.Parameters) < 2 {
			t.Fatalf("pipeline record = %#v, want callback shape and ordered parameters", record)
		}
	})
}

func TestProjectInstalledCallBehavior_IDCandidatesAlignWithSymbolIdentity(t *testing.T) {
	callByPath := recordsByPath(callbehavior.ProjectInstalledCallBehavior())
	symbolByPath := symbolRecordsByPath(symbolidentity.ProjectInstalledBindings())

	for path, callRecord := range callByPath {
		symbolRecord, ok := symbolByPath[path]
		if !ok {
			t.Fatalf("call-behavior path %q missing from symbol identity", path)
		}
		if callRecord.IDCandidate != symbolRecord.IDCandidate {
			t.Fatalf("idCandidate for %q = %q, want %q from symbol identity", path, callRecord.IDCandidate, symbolRecord.IDCandidate)
		}
	}
}

func TestProjectInstalledCallBehavior_DoesNotExposeHostContext(t *testing.T) {
	for _, path := range pathsFromInventory(callbehavior.ProjectInstalledCallBehavior()) {
		for _, forbidden := range callbehavior.ForbiddenRootGlobals {
			if path == forbidden || strings.HasPrefix(path, forbidden+".") {
				t.Fatalf("inventory exposes forbidden global %q via path %q", forbidden, path)
			}
		}
	}
}

func TestProjectInstalledCallBehavior_RepeatRunsAreByteIdentical(t *testing.T) {
	first, err := callbehavior.MarshalInventory(callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("first MarshalInventory() error = %v", err)
	}
	second, err := callbehavior.MarshalInventory(callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("second MarshalInventory() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeat descriptor runs differ:\nfirst  = %s\nsecond = %s", first, second)
	}
}

func TestVerifyProjectedInstalledCallBehavior_PassesForLiveDescriptor(t *testing.T) {
	if err := callbehavior.VerifyProjectedInstalledCallBehavior(); err != nil {
		t.Fatalf("VerifyProjectedInstalledCallBehavior() error = %v", err)
	}
}

func recordsByPath(inventory callbehavior.Inventory) map[string]callbehavior.CallBehaviorRecord {
	byPath := make(map[string]callbehavior.CallBehaviorRecord, len(inventory.Records))
	for _, record := range inventory.Records {
		byPath[record.Path] = record
	}
	return byPath
}

func symbolRecordsByPath(inventory symbolidentity.Inventory) map[string]symbolidentity.SymbolRecord {
	byPath := make(map[string]symbolidentity.SymbolRecord, len(inventory.Symbols))
	for _, record := range inventory.Symbols {
		byPath[record.Path] = record
	}
	return byPath
}

func pathsFromInventory(inventory callbehavior.Inventory) []string {
	paths := make([]string, 0, len(inventory.Records))
	for _, record := range inventory.Records {
		paths = append(paths, record.Path)
	}
	return paths
}

func TestProjectInstalledCallBehavior_CallableRecordsIncludeRequiredFields(t *testing.T) {
	for _, record := range callbehavior.ProjectInstalledCallBehavior().Records {
		if !record.Callable {
			continue
		}
		if record.Return == nil {
			t.Fatalf("callable record %q missing return behavior", record.Path)
		}
		if record.Async && !record.Return.Async {
			t.Fatalf("async callable %q return.async = false, want true", record.Path)
		}
		if !record.Async && record.Return.Async {
			t.Fatalf("sync callable %q return.async = true, want false", record.Path)
		}
	}
}

func TestProjectInstalledCallBehavior_ValueRecordsIncludeLifecycleFields(t *testing.T) {
	for _, record := range callbehavior.ProjectInstalledCallBehavior().Records {
		if record.Kind != "value" && record.Kind != "namespace" {
			continue
		}
		if record.Mutability == "" || record.Nullability == "" || record.Lifecycle == "" {
			t.Fatalf("record %q = %#v, want mutability, nullability, and lifecycle", record.Path, record)
		}
	}
}

func TestMarshalInventory_ProducesValidJSON(t *testing.T) {
	raw, err := callbehavior.MarshalInventory(callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}
	if decoded["formatVersion"] != callbehavior.FormatVersion {
		t.Fatalf("formatVersion = %v, want %q", decoded["formatVersion"], callbehavior.FormatVersion)
	}
}
