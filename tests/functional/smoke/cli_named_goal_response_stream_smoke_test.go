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

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

const namedGoalResponseStreamJSONRecordPrimary = "primary_result"

func TestNamedGoalResponseStream_RealCLICompletesWithPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal response-stream smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	stdout, stderr, err := runNamedGoalResponseStreamInvocationCLI(
		t,
		mockWorkersPath,
		false,
		goalText,
	)
	if err != nil {
		t.Fatalf("response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want primary result %q when no live progress arrived", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
}

func TestNamedGoalResponseStream_JSONModeEmitsPrimaryResultRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal JSON response-stream smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-json-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	stdout, stderr, err := runNamedGoalResponseStreamInvocationCLI(
		t,
		mockWorkersPath,
		true,
		goalText,
	)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	finalRecord, err := namedGoalResponseStreamFinalJSONRecord(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	if finalRecord.RecordType != namedGoalResponseStreamJSONRecordPrimary {
		t.Fatalf("final record type = %q, want %q", finalRecord.RecordType, namedGoalResponseStreamJSONRecordPrimary)
	}
	if finalRecord.Invocation.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("final record status = %q, want COMPLETED", finalRecord.Invocation.Status)
	}
	if got := invocationPrimaryResultText(t, finalRecord.Invocation); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("final record primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
}

func TestNamedGoalResponseStream_DurableFactoryEventsOmitInternalStreamTerms(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/goal durable event response-stream boundary smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-events-%d", time.Now().UnixNano())
	response := postNamedGoalRoutingInvocationOnServer(t, server, goalText)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}

	events := server.GetFactoryEvents(t)
	if len(events) == 0 {
		t.Fatal("expected durable factory events after named @you/goal invocation")
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	assertNamedGoalDurableEventsOmitInternalResponseStreamTerms(t, string(encoded))
}

type namedGoalResponseStreamJSONPrimaryResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}

func runNamedGoalResponseStreamInvocationCLI(
	t *testing.T,
	mockWorkersPath string,
	jsonOutput bool,
	goalText string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	homeDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(
		filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories"),
		goal.PackagedFactoryName,
		factoryconfig.BuiltInGoalFactoryJSON,
	); err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}

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
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		"--output", "response-stream",
		mockWorkersPath,
		goalText,
	}
	if jsonOutput {
		args = append([]string{"--json"}, args...)
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func namedGoalResponseStreamFinalJSONRecord(stdout string) (namedGoalResponseStreamJSONPrimaryResultRecord, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		return namedGoalResponseStreamJSONPrimaryResultRecord{}, fmt.Errorf("empty stdout")
	}
	var final namedGoalResponseStreamJSONPrimaryResultRecord
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record namedGoalResponseStreamJSONPrimaryResultRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return namedGoalResponseStreamJSONPrimaryResultRecord{}, fmt.Errorf("decode line %q: %w", line, err)
		}
		if record.RecordType == namedGoalResponseStreamJSONRecordPrimary {
			final = record
		}
	}
	if final.RecordType == "" {
		return namedGoalResponseStreamJSONPrimaryResultRecord{}, fmt.Errorf("missing primary_result record in %d lines", len(lines))
	}
	return final, nil
}

func assertNamedGoalDurableEventsOmitInternalResponseStreamTerms(t *testing.T, text string) {
	t.Helper()

	forbiddenTerms := []string{
		"SessionResponseStream",
		"SessionResponseStreamEvent",
		"SessionResponseStreamEventKind",
		"ExternalEventType",
		"CompactionSummary",
		"CompactionReason",
		"STREAM_COMPACTION_SIGNAL",
		"PROGRESS_FRAGMENT",
		"RESPONSE_FRAGMENT",
		"response.completed",
		"response.output_text.delta",
		"response.failed",
		"session.created",
	}
	for _, forbidden := range forbiddenTerms {
		if strings.Contains(text, forbidden) {
			t.Fatalf("unexpected internal response-stream term %q", forbidden)
		}
	}
}

