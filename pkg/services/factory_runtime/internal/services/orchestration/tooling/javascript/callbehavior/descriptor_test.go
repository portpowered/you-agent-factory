package callbehavior_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
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
		assertArgsValueLifecycleRecord(t, byPath["args"])
	})
	t.Run("workflow namespace lifecycle", func(t *testing.T) {
		assertWorkflowNamespaceLifecycleRecord(t, byPath["workflow"])
	})
	t.Run("workflow.final callable", func(t *testing.T) {
		assertWorkflowFinalCallableRecord(t, byPath["workflow.final"])
	})
	t.Run("workflow.checkpoint emits checkpoint", func(t *testing.T) {
		assertWorkflowCheckpointCallableRecord(t, byPath["workflow.checkpoint"])
	})
	t.Run("workflow.resumeState resume notes", func(t *testing.T) {
		assertWorkflowResumeStateCallableRecord(t, byPath["workflow.resumeState"])
	})
	t.Run("agent.run async promise", func(t *testing.T) {
		assertAgentRunAsyncPromiseRecord(t, byPath["agent.run"])
	})
	t.Run("parallel callback and promise", func(t *testing.T) {
		assertParallelCallbackPromiseRecord(t, byPath["parallel"])
	})
	t.Run("pipeline callback stages", func(t *testing.T) {
		assertPipelineCallbackStagesRecord(t, byPath["pipeline"])
	})
}

func assertArgsValueLifecycleRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.Kind != "value" || record.Mutability == "" || record.Lifecycle == "" {
		t.Fatalf("args record = %#v, want value with mutability and lifecycle", record)
	}
	if record.Callable || len(record.Parameters) > 0 {
		t.Fatalf("args record = %#v, want non-callable value", record)
	}
}

func assertWorkflowNamespaceLifecycleRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.Kind != "namespace" || record.Lifecycle != "live-namespace" {
		t.Fatalf("workflow record = %#v, want live namespace", record)
	}
}

func assertWorkflowFinalCallableRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if !record.Callable || record.Async || record.Return == nil {
		t.Fatalf("workflow.final record = %#v, want sync callable with return behavior", record)
	}
	if record.Return.SyncType != "undefined" {
		t.Fatalf("workflow.final return = %#v, want undefined sync return", record.Return)
	}
	if record.Determinism == "" {
		t.Fatal("workflow.final missing determinism note")
	}
}

func assertWorkflowCheckpointCallableRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if !slices.Contains(record.EmittedRecords, "checkpoint") {
		t.Fatalf("workflow.checkpoint emittedRecords = %v, want checkpoint", record.EmittedRecords)
	}
	if len(record.Parameters) != 1 || !record.Parameters[0].Required {
		t.Fatalf("workflow.checkpoint parameters = %#v, want one required object", record.Parameters)
	}
}

func assertWorkflowResumeStateCallableRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.ResumeNotes == "" {
		t.Fatal("workflow.resumeState missing resume notes")
	}
	if len(record.Parameters) != 0 {
		t.Fatalf("workflow.resumeState parameters = %#v, want zero parameters", record.Parameters)
	}
}

func assertAgentRunAsyncPromiseRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if !record.Callable || !record.Async || record.Return == nil || !record.Return.Async {
		t.Fatalf("agent.run record = %#v, want async callable with promise return", record)
	}
	if !slices.Contains(record.EmittedRecords, "child_dispatch") {
		t.Fatalf("agent.run emittedRecords = %v, want child_dispatch", record.EmittedRecords)
	}
	if len(record.PolicyChecks) == 0 {
		t.Fatal("agent.run missing policy checks")
	}
}

func assertParallelCallbackPromiseRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.Callback == nil || record.Return == nil || !record.Return.Async {
		t.Fatalf("parallel record = %#v, want callback shape and promise return", record)
	}
}

func assertPipelineCallbackStagesRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.Callback == nil || len(record.Parameters) < 2 {
		t.Fatalf("pipeline record = %#v, want callback shape and ordered parameters", record)
	}
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
