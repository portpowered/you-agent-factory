package contracts_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

const (
	runtimeWithoutManifestsHelper = "YOU_TEST_RUNTIME_WITHOUT_MANIFESTS"
	runtimeBehaviorSnapshotPath   = "YOU_TEST_RUNTIME_BEHAVIOR_SNAPSHOT"
)

type runtimeBehaviorSnapshot struct {
	InvocationValue   string                          `json:"invocationValue"`
	InvocationRecords []workflowruntime.RuntimeRecord `json:"invocationRecords"`
	ResumeValue       string                          `json:"resumeValue"`
	DeniedFailure     workflowruntime.Failure         `json:"deniedFailure"`
	DeniedRecords     []workflowruntime.RuntimeRecord `json:"deniedRecords"`
}

func TestJavaScriptRuntimeBehaviorDoesNotLoadContractManifests(t *testing.T) {
	if os.Getenv(runtimeWithoutManifestsHelper) == "1" {
		writeRuntimeBehaviorSnapshot(t, captureRuntimeBehavior(t))
		return
	}

	baseline := marshalRuntimeBehaviorSnapshot(t, captureRuntimeBehavior(t))
	root := t.TempDir()
	for _, relativePath := range []string{
		filepath.Join("contracts", "javascript", "runtime-api.json"),
		filepath.Join("contracts", "javascript", "runtime-manifest.schema.json"),
		filepath.Join("packages", "api", "generated", "javascript", "runtime-api.json"),
	} {
		writeUnusableManifest(t, root, relativePath)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestJavaScriptRuntimeBehaviorDoesNotLoadContractManifests$")
	cmd.Dir = root
	snapshotPath := filepath.Join(root, "runtime-behavior.json")
	cmd.Env = append(os.Environ(),
		runtimeWithoutManifestsHelper+"=1",
		runtimeBehaviorSnapshotPath+"="+snapshotPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execute JavaScript runtime with unusable contract manifests: %v\n%s", err, output)
	}
	withoutManifests, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read runtime behavior snapshot: %v", err)
	}
	if !bytes.Equal(withoutManifests, baseline) {
		t.Fatalf("runtime behavior changed with unusable manifests:\nbaseline: %s\nunusable: %s", baseline, withoutManifests)
	}
}

func writeUnusableManifest(t *testing.T, root, relativePath string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create unusable manifest directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not-valid-json"), 0o600); err != nil {
		t.Fatalf("write unusable manifest %s: %v", filepath.ToSlash(relativePath), err)
	}
}

func captureRuntimeBehavior(t *testing.T) runtimeBehaviorSnapshot {
	t.Helper()

	invocation := runJavaScript(t, workflowruntime.Request{
		Source: `
workflow.checkpoint({ label: "manifest-independent", state: { step: 1 } });
workflow.final({ status: "complete" });
`,
		SourceRef: "manifest-independent-invocation.js",
		SessionID: "manifest-independent-invocation",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	})
	if !invocation.OK || string(invocation.Value.JSON) != `{"status":"complete"}` {
		t.Fatalf("invocation outcome = %#v, want successful manifest-independent final", invocation)
	}
	if len(invocation.Records) != 1 || invocation.Records[0].Kind != workflowruntime.RecordKindCheckpoint {
		t.Fatalf("invocation records = %#v, want one checkpoint record", invocation.Records)
	}

	resume := runJavaScript(t, workflowruntime.Request{
		Source:    `return { resumedStep: workflow.resumeState().step };`,
		SourceRef: "manifest-independent-resume.js",
		SessionID: "manifest-independent-resume",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
		Resume: &workflowruntime.ResumeContext{CheckpointState: map[string]any{
			"step": float64(2),
		}},
	})
	if !resume.OK || string(resume.Value.JSON) != `{"resumedStep":2}` {
		t.Fatalf("resume outcome = %#v, want restored checkpoint state", resume)
	}

	maxArtifactBytes := int64(1)
	deniedPolicy := workflowpolicy.DefaultEffectivePolicy()
	deniedPolicy.MaxArtifactBytes = &maxArtifactBytes
	denied := runJavaScript(t, workflowruntime.Request{
		Source:    `workflow.artifact({ kind: "report", label: "oversized", content: { body: "too large" } }); return { ok: true };`,
		SourceRef: "manifest-independent-policy.js",
		SessionID: "manifest-independent-policy",
		Policy:    deniedPolicy,
	})
	if denied.OK || denied.Failure.Code != workflowruntime.CodeScriptError || !strings.Contains(denied.Failure.Message, "policy denied") {
		t.Fatalf("policy outcome = %#v, want stable policy denial", denied)
	}
	for _, record := range denied.Records {
		if record.Kind == workflowruntime.RecordKindArtifact {
			t.Fatalf("policy denial emitted artifact record: %#v", denied.Records)
		}
	}

	return runtimeBehaviorSnapshot{
		InvocationValue:   string(invocation.Value.JSON),
		InvocationRecords: invocation.Records,
		ResumeValue:       string(resume.Value.JSON),
		DeniedFailure:     denied.Failure,
		DeniedRecords:     denied.Records,
	}
}

func writeRuntimeBehaviorSnapshot(t *testing.T, snapshot runtimeBehaviorSnapshot) {
	t.Helper()
	path := os.Getenv(runtimeBehaviorSnapshotPath)
	if path == "" {
		t.Fatal("runtime behavior snapshot path is required in helper process")
	}
	if err := os.WriteFile(path, marshalRuntimeBehaviorSnapshot(t, snapshot), 0o600); err != nil {
		t.Fatalf("write runtime behavior snapshot: %v", err)
	}
}

func marshalRuntimeBehaviorSnapshot(t *testing.T, snapshot runtimeBehaviorSnapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal runtime behavior snapshot: %v", err)
	}
	return raw
}

func runJavaScript(t *testing.T, request workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), request, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("run JavaScript workflow: %v", err)
	}
	return outcome
}

func TestJavaScriptAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoRootFromCaller(t, 0)
	authoredSchema := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-manifest.schema.json")
	authoredCatalog := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-api.json")

	if _, err := os.Stat(authoredSchema); err != nil {
		t.Fatalf("authored runtime-manifest schema missing: %v", err)
	}
	if _, err := os.Stat(authoredCatalog); err != nil {
		t.Fatalf("authored JavaScript runtime API catalog missing: %v", err)
	}

	allowed := map[string]struct{}{
		"runtime-manifest.schema.json": {},
		"runtime-api.json":             {},
	}
	entries, err := os.ReadDir(testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript"))
	if err != nil {
		t.Fatalf("read contracts/javascript: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("contracts/javascript must contain only authored contract files, found directory %s", entry.Name())
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("unexpected authored javascript contract %s under %s", entry.Name(), repositoryRoot)
		}
	}
}

func TestJavaScriptStagedRuntimeAPIProjectsFromAuthoredCatalog(t *testing.T) {
	t.Parallel()

	const (
		wantSource = "contracts/javascript/runtime-api.json"
		wantTarget = "packages/api/generated/javascript/runtime-api.json"
	)

	found := false
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Target != wantTarget {
			continue
		}
		found = true
		if artifact.Source != wantSource {
			t.Fatalf("staged runtime-api source = %q, want authored catalog %q", artifact.Source, wantSource)
		}
	}
	if !found {
		t.Fatalf("missing staged runtime-api projection in RawArtifacts()")
	}
}
