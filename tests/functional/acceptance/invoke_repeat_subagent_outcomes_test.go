package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/packages/subagent"
	"github.com/portpowered/infinite-you/pkg/factory/packages/tts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const packagedSubagentMockWorkerAcceptedSummary = "mock worker accepted"

func TestLocalModelInvoke_MissingReadiness_FailsWithDocumentedBootstrapGuidance(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI local model invoke readiness acceptance")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	initResult, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "local-model-config-init", initResult, err)

	if writeErr := writeProjectFactoryJSON(session.WorkDir, tts.BuiltInFactoryJSON); writeErr != nil {
		t.Fatalf("write project factory.json: %v", writeErr)
	}

	args := append([]string{"--json"}, session.ServerFlags()...)
	args = append(args,
		"models", "invoke", tts.DefaultModelName,
		"--operation", "TTS",
		"--text", "acceptance-local-model-missing-readiness",
	)

	result, err := session.Run(ctx, args...)
	if err == nil {
		t.Fatalf("expected missing-readiness local model invoke failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for missing managed-runtime readiness")
	}

	combined := result.Stdout + result.Stderr
	for _, want := range []string{
		"pull or install",
		tts.DefaultModelName,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("output = %q, want documented missing-readiness guidance %q", combined, want)
		}
	}
	if strings.Contains(combined, "models endpoint not reachable") {
		t.Fatalf("output = %q, want bootstrap readiness failure instead of HTTP transport failure", combined)
	}
}

func TestGoalRepeat_RepeatedNamedRunsAssignDistinctInvocationIdentityAndReuseInstalledCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI goal repeat acceptance")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	initResult, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "goal-repeat-config-init", initResult, err)

	configPath := defaultpaths.OperatorConfigPath(session.HomeDir)
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if writeErr := os.WriteFile(configPath, configBody, 0o600); writeErr != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, writeErr)
	}

	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf("acceptance-goal-repeat-%d", time.Now().UnixNano())

	firstResult, firstErr := session.Run(ctx, namedGoalJSONRunArgs(session, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "goal-repeat-first-run", firstResult, firstErr)
	firstResponse := decodeInvocationResponse(t, firstResult.Stdout)

	installedDir := materializedNamedFactoryDir(t, session.HomeDir, goal.PackagedFactoryName)
	if _, statErr := os.Stat(installedDir); statErr != nil {
		t.Fatalf("installed goal dir %q: %v", installedDir, statErr)
	}

	secondResult, secondErr := session.Run(ctx, namedGoalJSONRunArgs(session, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "goal-repeat-second-run", secondResult, secondErr)
	secondResponse := decodeInvocationResponse(t, secondResult.Stdout)

	if firstResponse.RequestId == "" || secondResponse.RequestId == "" {
		t.Fatalf("requestId missing: first=%q second=%q", firstResponse.RequestId, secondResponse.RequestId)
	}
	if firstResponse.TraceId == "" || secondResponse.TraceId == "" {
		t.Fatalf("traceId missing: first=%q second=%q", firstResponse.TraceId, secondResponse.TraceId)
	}
	if firstResponse.RequestId == secondResponse.RequestId {
		t.Fatalf("requestId = %q for both runs, want distinct invocation identity per repeat", firstResponse.RequestId)
	}
	if firstResponse.TraceId == secondResponse.TraceId {
		t.Fatalf("traceId = %q for both runs, want distinct invocation identity per repeat", firstResponse.TraceId)
	}
	if firstResponse.Status != factoryapi.InvocationTerminalStatusCompleted ||
		secondResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status mismatch: first=%q second=%q, want COMPLETED", firstResponse.Status, secondResponse.Status)
	}
	if got := invocationPrimaryResultText(t, firstResponse); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("first primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if got := invocationPrimaryResultText(t, secondResponse); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("second primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}

	otherSession := harness.NewSession(t).WithNoExternalServer(t)
	if otherSession.HomeDir == session.HomeDir || otherSession.LogDir == session.LogDir {
		t.Fatalf("sessions share home/log dirs: home=%q/%q log=%q/%q",
			session.HomeDir, otherSession.HomeDir, session.LogDir, otherSession.LogDir)
	}
}

func TestSubagentInvocation_SuccessfulNamedRun_ReturnsAuthoritativePrimaryResultJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI subagent primary JSON acceptance")
	}

	session, mockWorkersPath := prepareNamedSubagentAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	requestText := fmt.Sprintf("acceptance-subagent-primary-%d", time.Now().UnixNano())
	result, err := session.Run(ctx, namedSubagentJSONRunArgs(session, "", mockWorkersPath, requestText)...)
	session.RequireSuccess(t, "subagent-primary-json", result, err)

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &response); decodeErr != nil {
		t.Fatalf("decode subagent JSON stdout: %v\nstdout:\n%s", decodeErr, result.Stdout)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedSubagentMockWorkerAcceptedSummary {
		t.Fatalf("primaryResult = %q, want %q", got, packagedSubagentMockWorkerAcceptedSummary)
	}
	if strings.Contains(result.Stdout, requestText) {
		t.Fatalf("stdout echoed submitted request text %q", requestText)
	}
}

func TestSubagentInvocation_PrimaryAndResponseStreamAgreeOnTerminalOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI subagent primary vs response-stream parity acceptance")
	}

	session, mockWorkersPath := prepareNamedSubagentAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	requestText := fmt.Sprintf("acceptance-subagent-stream-parity-%d", time.Now().UnixNano())

	primaryResult, primaryErr := session.Run(ctx, namedSubagentJSONRunArgs(session, "", mockWorkersPath, requestText)...)
	session.RequireSuccess(t, "subagent-parity-primary-json", primaryResult, primaryErr)
	primaryResponse := decodeInvocationResponse(t, primaryResult.Stdout)

	streamResult, streamErr := session.Run(ctx, namedSubagentJSONRunArgs(session, "response-stream", mockWorkersPath, requestText)...)
	session.RequireSuccess(t, "subagent-parity-response-stream-json", streamResult, streamErr)

	records, parseErr := parseResponseStreamNDJSONRecords(streamResult.Stdout)
	if parseErr != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", parseErr, streamResult.Stdout)
	}
	streamTerminal, terminalErr := responseStreamTerminalInvocation(records)
	if terminalErr != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", terminalErr, streamResult.Stdout)
	}

	assertInvocationTerminalOutcomeParity(t, primaryResponse, streamTerminal)
}

func prepareNamedSubagentAcceptanceSession(t *testing.T) (*builtcliacceptance.Session, string) {
	t.Helper()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initResult, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "subagent-config-init", initResult, err)

	configPath := defaultpaths.OperatorConfigPath(session.HomeDir)
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if writeErr := os.WriteFile(configPath, configBody, 0o600); writeErr != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, writeErr)
	}

	return session, writePackagedSubagentMockWorkersConfig(t)
}

func namedGoalJSONRunArgs(session *builtcliacceptance.Session, mockWorkersPath, goalText string) []string {
	args := append([]string{"--json"}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	return args
}

func namedSubagentJSONRunArgs(
	session *builtcliacceptance.Session,
	outputMode string,
	mockWorkersPath string,
	requestText string,
) []string {
	args := append([]string{"--json"}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", subagent.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--quiet",
	)
	if outputMode == "response-stream" {
		args = append(args, "--output", "response-stream")
	}
	args = append(args, mockWorkersPath, requestText)
	return args
}

func writePackagedSubagentMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      subagent.PackagedWorkerName,
				WorkstationName: subagent.PackagedRunWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged subagent mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-subagent-acceptance.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged subagent mock-workers config: %v", err)
	}
	return path
}

func writeProjectFactoryJSON(workDir string, payload []byte) error {
	factoryDir := filepath.Join(workDir, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(factoryDir, "factory.json"), payload, 0o644)
}

func materializedNamedFactoryDir(t *testing.T, homeDir, factoryName string) string {
	t.Helper()

	globalRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	dir, err := factoryconfig.MapNamedFactoryDir(globalRoot, factoryName)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(%q): %v", factoryName, err)
	}
	return dir
}

func decodeInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &response); decodeErr != nil {
		t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, stdout)
	}
	return response
}
