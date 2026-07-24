package support

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProviderSessionCase_MissingExpectedGoldenFailsLoudly(t *testing.T) {
	caseDir := writeProviderSessionLoadableCase(t, "harness-missing-expected")
	missingPath := filepath.Join(caseDir, "expected-provider-session.json")
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("remove expected-provider-session.json: %v", err)
	}

	_, err := LoadProviderSessionCase(caseDir)
	if err == nil {
		t.Fatal("LoadProviderSessionCase: expected missing fixture failure, got nil")
	}
	assertProviderSessionMissingFixtureError(
		t,
		err,
		"harness-missing-expected",
		"expected-provider-session",
		missingPath,
	)
}

func TestLoadProviderSessionCase_MissingRequestFixtureFailsLoudly(t *testing.T) {
	caseDir := writeProviderSessionLoadableCase(t, "harness-missing-request")
	missingPath := filepath.Join(caseDir, "request.json")
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("remove request.json: %v", err)
	}

	_, err := LoadProviderSessionCase(caseDir)
	if err == nil {
		t.Fatal("LoadProviderSessionCase: expected missing fixture failure, got nil")
	}
	assertProviderSessionMissingFixtureError(
		t,
		err,
		"harness-missing-request",
		"request",
		missingPath,
	)
}

func TestCompareOrUpdateProviderSessionGoldens_DriftWithoutUpdateEnvFailsAndDoesNotRewrite(t *testing.T) {
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "")

	caseDir := writeProviderSessionLoadableCase(t, "harness-drift-no-update")
	loaded, err := LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}

	beforeSession, err := os.ReadFile(loaded.Paths.ExpectedProviderSession)
	if err != nil {
		t.Fatalf("read expected session before: %v", err)
	}
	beforeEvents, err := os.ReadFile(loaded.Paths.ExpectedResponseEvents)
	if err != nil {
		t.Fatalf("read expected events before: %v", err)
	}
	beforeResult, err := os.ReadFile(loaded.Paths.ExpectedInvocationResult)
	if err != nil {
		t.Fatalf("read expected result before: %v", err)
	}

	observed := ProviderSessionObservedGoldens{
		ProviderSession: json.RawMessage(`{
			"provider":"harness",
			"status":"failed",
			"eventId":"evt_drift",
			"finishReason":"error"
		}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage(`{"type":"message.completed","eventId":"evt_1","itemId":"item_drift","finishReason":"error"}`),
		},
		InvocationResult: json.RawMessage(`{"ok":false,"content":"drifted"}`),
	}

	err = CompareOrUpdateProviderSessionGoldens(loaded, observed)
	if err == nil {
		t.Fatal("CompareOrUpdateProviderSessionGoldens: expected drift failure")
	}
	var compareErr *ProviderSessionCompareError
	if !errors.As(err, &compareErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionCompareError", err, err)
	}
	var updatedErr *ProviderSessionGoldensUpdatedError
	if errors.As(err, &updatedErr) {
		t.Fatalf("unexpected update error without UPDATE_FUNCTIONAL_GOLDENS=1: %v", err)
	}

	afterSession, err := os.ReadFile(loaded.Paths.ExpectedProviderSession)
	if err != nil {
		t.Fatalf("read expected session after: %v", err)
	}
	afterEvents, err := os.ReadFile(loaded.Paths.ExpectedResponseEvents)
	if err != nil {
		t.Fatalf("read expected events after: %v", err)
	}
	afterResult, err := os.ReadFile(loaded.Paths.ExpectedInvocationResult)
	if err != nil {
		t.Fatalf("read expected result after: %v", err)
	}
	if string(afterSession) != string(beforeSession) {
		t.Fatalf("expected-provider-session.json was rewritten without UPDATE_FUNCTIONAL_GOLDENS=1")
	}
	if string(afterEvents) != string(beforeEvents) {
		t.Fatalf("expected-response-events.ndjson was rewritten without UPDATE_FUNCTIONAL_GOLDENS=1")
	}
	if string(afterResult) != string(beforeResult) {
		t.Fatalf("expected-invocation-result.json was rewritten without UPDATE_FUNCTIONAL_GOLDENS=1")
	}
}

func TestCompareOrUpdateProviderSessionGoldens_UpdateEnvRewritesExpectedGoldens(t *testing.T) {
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "1")

	caseDir := writeProviderSessionLoadableCase(t, "harness-drift-update")
	loaded, err := LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}

	observed := ProviderSessionObservedGoldens{
		ProviderSession: json.RawMessage(`{
			"provider":"harness",
			"status":"completed",
			"eventId":"evt_updated",
			"finishReason":"stop"
		}`),
		ResponseEvents: []json.RawMessage{
			json.RawMessage(`{"type":"message.delta","eventId":"evt_1","itemId":"item_1","text":"Updated"}`),
			json.RawMessage(`{"type":"message.completed","eventId":"evt_2","itemId":"item_1","finishReason":"stop"}`),
		},
		InvocationResult: json.RawMessage(`{"ok":true,"content":"Updated"}`),
	}

	err = CompareOrUpdateProviderSessionGoldens(loaded, observed)
	if err == nil {
		t.Fatal("CompareOrUpdateProviderSessionGoldens: expected ProviderSessionGoldensUpdatedError after rewrite")
	}
	var updatedErr *ProviderSessionGoldensUpdatedError
	if !errors.As(err, &updatedErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionGoldensUpdatedError", err, err)
	}
	if updatedErr.CaseID != "harness-drift-update" {
		t.Fatalf("CaseID = %q, want harness-drift-update", updatedErr.CaseID)
	}
	if len(updatedErr.Paths) != 3 {
		t.Fatalf("rewritten paths = %#v, want 3 expected golden paths", updatedErr.Paths)
	}
	if !strings.Contains(err.Error(), ProviderSessionUpdateFunctionalGoldensEnv) {
		t.Fatalf("error = %q, want UPDATE_FUNCTIONAL_GOLDENS guidance", err)
	}

	reloaded, err := LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase after rewrite: %v", err)
	}
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "")
	if err := CompareOrUpdateProviderSessionGoldens(reloaded, observed); err != nil {
		t.Fatalf("compare after rewrite should pass without update env: %v", err)
	}
}

func TestProviderSessionFunctionalGoldensUpdateEnabled(t *testing.T) {
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "")
	if ProviderSessionFunctionalGoldensUpdateEnabled() {
		t.Fatal("empty env must disable golden updates")
	}
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "0")
	if ProviderSessionFunctionalGoldensUpdateEnabled() {
		t.Fatal("UPDATE_FUNCTIONAL_GOLDENS=0 must disable golden updates")
	}
	t.Setenv(ProviderSessionUpdateFunctionalGoldensEnv, "1")
	if !ProviderSessionFunctionalGoldensUpdateEnabled() {
		t.Fatal("UPDATE_FUNCTIONAL_GOLDENS=1 must enable golden updates")
	}
}

func writeProviderSessionLoadableCase(t *testing.T, caseID string) string {
	t.Helper()

	manifest := ProviderSessionGoldenManifest{
		SchemaVersion:                ProviderSessionGoldenManifestSchemaVersion,
		ID:                           caseID,
		Provider:                     "harness",
		ProviderVersion:              "fixture-1",
		FidelityClass:                ProviderSessionFidelityFinalOnly,
		Case:                         "update-gate",
		StdoutFile:                   "stdout.jsonl",
		StderrFile:                   "stderr.txt",
		RequestFile:                  "request.json",
		ProcessFile:                  "process.json",
		ExpectedProviderSessionFile:  "expected-provider-session.json",
		ExpectedResponseEventsFile:   "expected-response-events.ndjson",
		ExpectedInvocationResultFile: "expected-invocation-result.json",
		NormalizedFields:             []string{"eventId", "recordedAt"},
		SanitizerVersion:             ProviderSessionGoldenSanitizerVersion,
		Source:                       "sanitized-provider-exec",
	}

	caseDir := t.TempDir()
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, ProviderSessionGoldenManifestFile), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	files := map[string]string{
		"request.json": `{
  "prompt": "hello",
  "sessionId": "sess_fixture_001"
}
`,
		"process.json": `{
  "argv": ["provider", "run"],
  "provider": "harness",
  "model": "fixture-model",
  "exitCode": 0,
  "stdoutStream": true,
  "stderrStream": false,
  "workingDirectoryRole": "case-workspace",
  "timeoutCancelClass": "none",
  "terminalErrorClass": "none"
}
`,
		"stdout.jsonl": `{"type":"message.completed","itemId":"item_1","text":"Hello"}
`,
		"stderr.txt": "",
		"expected-provider-session.json": `{
  "provider": "harness",
  "status": "completed",
  "eventId": "evt_expected",
  "finishReason": "stop"
}
`,
		"expected-response-events.ndjson": `{"type":"message.completed","eventId":"evt_1","itemId":"item_1","finishReason":"stop"}
`,
		"expected-invocation-result.json": `{
  "ok": true,
  "content": "Hello"
}
`,
	}
	for name, contents := range files {
		path := filepath.Join(caseDir, name)
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return caseDir
}

func assertProviderSessionMissingFixtureError(t *testing.T, err error, wantCaseID, wantRole, wantPath string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected missing fixture error, got nil")
	}
	var loadErr *ProviderSessionLoadError
	if !errors.As(err, &loadErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionLoadError", err, err)
	}
	if loadErr.CaseID != wantCaseID {
		t.Fatalf("CaseID = %q, want %q", loadErr.CaseID, wantCaseID)
	}
	if loadErr.Role != wantRole {
		t.Fatalf("Role = %q, want %q", loadErr.Role, wantRole)
	}
	if loadErr.Path != wantPath {
		t.Fatalf("Path = %q, want %q", loadErr.Path, wantPath)
	}
	message := err.Error()
	if !strings.Contains(message, wantCaseID) {
		t.Fatalf("error %q does not name case id %q", message, wantCaseID)
	}
	if !strings.Contains(message, wantRole) {
		t.Fatalf("error %q does not name role %q", message, wantRole)
	}
	if !strings.Contains(message, "missing") {
		t.Fatalf("error %q does not say missing", message)
	}
	if !strings.Contains(message, wantPath) {
		t.Fatalf("error %q does not name path %q", message, wantPath)
	}
}
