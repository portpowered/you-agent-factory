package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedSubagentMockWorkerAcceptedSummary = "mock worker accepted"

func TestNamedSubagentResponseStream_RealCLICompletesWithPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent response-stream smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	stdout, stderr, err := runNamedSubagentResponseStreamInvocationCLI(
		t,
		mockWorkersPath,
		false,
		requestText,
	)
	if err != nil {
		t.Fatalf("response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != packagedSubagentMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want primary result %q when no live progress arrived", got, packagedSubagentMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout, requestText) {
		t.Fatalf("stdout echoed submitted request text %q", requestText)
	}
}

func TestNamedSubagentResponseStream_JSONModeEmitsInvocationResultRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent JSON response-stream smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-json-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	stdout, stderr, err := runNamedSubagentResponseStreamInvocationCLI(
		t,
		mockWorkersPath,
		true,
		requestText,
	)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	finalRecord, err := namedGoalResponseStreamFinalJSONRecord(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	if finalRecord.RecordType != namedGoalResponseStreamJSONRecordInvocation {
		t.Fatalf("final record type = %q, want %q", finalRecord.RecordType, namedGoalResponseStreamJSONRecordInvocation)
	}
	if finalRecord.Invocation.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("final record status = %q, want COMPLETED", finalRecord.Invocation.Status)
	}
	if got := invocationPrimaryResultText(t, finalRecord.Invocation); got != packagedSubagentMockWorkerAcceptedSummary {
		t.Fatalf("final record primaryResult = %q, want %q", got, packagedSubagentMockWorkerAcceptedSummary)
	}
}

func TestNamedSubagentResponseStream_PrimaryOnlyAndResponseStreamAgreeOnTerminalOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent primary-only vs response-stream parity smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-parity-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	primaryStdout, primaryStderr, err := runNamedSubagentPrimaryOnlyInvocationCLI(t, mockWorkersPath, requestText)
	if err != nil {
		t.Fatalf("primary-only invocation: %v\nstdout:\n%s\nstderr:\n%s", err, primaryStdout, primaryStderr)
	}
	var primaryResponse factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(primaryStdout)), &primaryResponse); err != nil {
		t.Fatalf("decode primary-only JSON: %v\nstdout:\n%s", err, primaryStdout)
	}

	streamStdout, streamStderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	streamTerminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", err, streamStdout)
	}

	assertNamedGoalInvocationTerminalOutcomeParity(t, primaryResponse, streamTerminal)
}

func TestNamedSubagentResponseStream_JSONModeEmitsExactlyOneInvocationResultRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent JSON response-stream invocation_result count smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-json-count-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	stdout, stderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	invocationCount := 0
	for _, record := range records {
		if record.RecordType == namedGoalResponseStreamJSONRecordInvocation {
			invocationCount++
		}
	}
	if invocationCount != 1 {
		t.Fatalf("invocation_result record count = %d, want exactly 1", invocationCount)
	}
	if records[len(records)-1].RecordType != namedGoalResponseStreamJSONRecordInvocation {
		t.Fatalf("final record type = %q, want %q", records[len(records)-1].RecordType, namedGoalResponseStreamJSONRecordInvocation)
	}
}

func TestNamedSubagentResponseStream_JSONModeUsesCanonicalCLIStreamRecordVocabulary(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent JSON response-stream vocabulary smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-json-vocab-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	stdout, stderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	assertNamedGoalResponseStreamJSONRecordsUseCanonicalVocabulary(t, records)
}

func TestNamedSubagentResponseStream_HumanModeUsesCanonicalHumanFormatNotLegacyDialect(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/subagent human response-stream vocabulary smoke")
	}

	requestText := fmt.Sprintf("functional-smoke-subagent-response-stream-human-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedSubagentMockWorkers(t)

	stdout, stderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, false, requestText)
	if err != nil {
		t.Fatalf("human response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	assertNamedGoalHumanResponseStreamAvoidsLegacyDialect(t, stdout)
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "--- primary result ---" {
			continue
		}
		if strings.HasPrefix(trimmed, "progress:") ||
			strings.HasPrefix(trimmed, "reasoning:") ||
			strings.HasPrefix(trimmed, "tool:") ||
			strings.HasPrefix(trimmed, "stream gap:") {
			continue
		}
		if trimmed == packagedSubagentMockWorkerAcceptedSummary {
			continue
		}
		t.Fatalf("unexpected human response-stream line %q outside canonical response-event contract", trimmed)
	}
}

func writePackagedSubagentMockWorkers(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      factorydefinitions.PackagedSubagentWorkerName,
				WorkstationName: factorydefinitions.PackagedSubagentRunWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-subagent.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

func runNamedSubagentPrimaryOnlyInvocationCLI(
	t *testing.T,
	mockWorkersPath string,
	requestText string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedSubagentFactoryName)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		requestText,
	)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func runNamedSubagentResponseStreamInvocationCLI(
	t *testing.T,
	mockWorkersPath string,
	jsonOutput bool,
	requestText string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedSubagentFactoryName)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{
		"run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		"--output", "response-stream",
		mockWorkersPath,
		requestText,
	}
	if jsonOutput {
		args = append([]string{"--json"}, args...)
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), runErr
}
