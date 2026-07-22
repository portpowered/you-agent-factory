package factory_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	runtimeWithoutManifestsHelper = "YOU_TEST_RUNTIME_WITHOUT_MANIFESTS"
	runtimeBehaviorSnapshotPath   = "YOU_TEST_RUNTIME_BEHAVIOR_SNAPSHOT"
)

type runtimeBehaviorSnapshot struct {
	InvocationValue   string                            `json:"invocationValue"`
	InvocationRecords []factory.JavaScriptRuntimeRecord `json:"invocationRecords"`
	ResumeValue       string                            `json:"resumeValue"`
	DeniedFailure     factory.JavaScriptRuntimeFailure  `json:"deniedFailure"`
	DeniedRecords     []factory.JavaScriptRuntimeRecord `json:"deniedRecords"`
}

func TestJavaScriptRuntimeBehaviorDoesNotLoadContractManifests(t *testing.T) {
	t.Parallel()
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

var manifestIndependenceWorkflows = testJavaScriptWorkflows()

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

	invocation := runJavaScript(t, factory.JavaScriptRuntimeRequest{
		Source: `
workflow.checkpoint({ label: "manifest-independent", state: { step: 1 } });
workflow.final({ status: "complete" });
`,
		SourceRef: "manifest-independent-invocation.js",
		SessionID: "manifest-independent-invocation",
		Policy:    factory.DefaultJavaScriptPolicy(),
	})
	if !invocation.OK || string(invocation.Value.JSON) != `{"status":"complete"}` {
		t.Fatalf("invocation outcome = %#v, want successful manifest-independent final", invocation)
	}
	if len(invocation.Records) != 1 || invocation.Records[0].Kind != factory.JavaScriptRecordKindCheckpoint {
		t.Fatalf("invocation records = %#v, want one checkpoint record", invocation.Records)
	}

	resume := runJavaScript(t, factory.JavaScriptRuntimeRequest{
		Source:    `return { resumedStep: workflow.resumeState().step };`,
		SourceRef: "manifest-independent-resume.js",
		SessionID: "manifest-independent-resume",
		Policy:    factory.DefaultJavaScriptPolicy(),
		Resume: &factory.JavaScriptResumeContext{CheckpointState: map[string]any{
			"step": float64(2),
		}},
	})
	if !resume.OK || string(resume.Value.JSON) != `{"resumedStep":2}` {
		t.Fatalf("resume outcome = %#v, want restored checkpoint state", resume)
	}

	maxArtifactBytes := int64(1)
	deniedPolicy := factory.DefaultJavaScriptPolicy()
	deniedPolicy.MaxArtifactBytes = &maxArtifactBytes
	denied := runJavaScript(t, factory.JavaScriptRuntimeRequest{
		Source:    `workflow.artifact({ kind: "report", label: "oversized", content: { body: "too large" } }); return { ok: true };`,
		SourceRef: "manifest-independent-policy.js",
		SessionID: "manifest-independent-policy",
		Policy:    deniedPolicy,
	})
	if denied.OK || denied.Failure.Code != factory.JavaScriptRuntimeCodeScriptError || !strings.Contains(denied.Failure.Message, "policy denied") {
		t.Fatalf("policy outcome = %#v, want stable policy denial", denied)
	}
	for _, record := range denied.Records {
		if record.Kind == factory.JavaScriptRecordKindArtifact {
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

func runJavaScript(t *testing.T, request factory.JavaScriptRuntimeRequest) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	outcome, err := manifestIndependenceWorkflows.Run(t.Context(), request, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("run JavaScript workflow: %v", err)
	}
	return outcome
}
