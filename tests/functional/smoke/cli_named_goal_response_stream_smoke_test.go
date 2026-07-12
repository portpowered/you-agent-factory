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

const (
	namedGoalResponseStreamJSONRecordProgress   = "progress"
	namedGoalResponseStreamJSONRecordStreamGap  = "stream_gap"
	namedGoalResponseStreamJSONRecordCompaction = "compaction"
	namedGoalResponseStreamJSONRecordPrimary    = "primary_result"

	namedGoalResponseStreamHumanProgressPrefix = "[you:progress] "
)

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

func TestNamedGoalResponseStream_PrimaryOnlyAndResponseStreamAgreeOnTerminalOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal primary-only vs response-stream parity smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-parity-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	primaryStdout, primaryStderr, err := runNamedGoalPrimaryOnlyInvocationCLI(t, mockWorkersPath, goalText)
	if err != nil {
		t.Fatalf("primary-only invocation: %v\nstdout:\n%s\nstderr:\n%s", err, primaryStdout, primaryStderr)
	}
	var primaryResponse factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(primaryStdout)), &primaryResponse); err != nil {
		t.Fatalf("decode primary-only JSON: %v\nstdout:\n%s", err, primaryStdout)
	}

	streamStdout, streamStderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
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

func TestNamedGoalResponseStream_JSONModeEmitsExactlyOnePrimaryResultRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal JSON response-stream primary_result count smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-json-count-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	stdout, stderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	primaryCount := 0
	for _, record := range records {
		if record.RecordType == namedGoalResponseStreamJSONRecordPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Fatalf("primary_result record count = %d, want exactly 1", primaryCount)
	}
	if records[len(records)-1].RecordType != namedGoalResponseStreamJSONRecordPrimary {
		t.Fatalf("final record type = %q, want %q", records[len(records)-1].RecordType, namedGoalResponseStreamJSONRecordPrimary)
	}
}

func TestNamedGoalResponseStream_JSONModeUsesCanonicalCLIStreamRecordVocabulary(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal JSON response-stream vocabulary smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-json-vocab-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	stdout, stderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", err, stdout)
	}
	assertNamedGoalResponseStreamJSONRecordsUseCanonicalVocabulary(t, records)
}

func TestNamedGoalResponseStream_HumanModeUsesCanonicalProgressPrefixNotLegacyDialect(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal human response-stream vocabulary smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-goal-response-stream-human-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})

	stdout, stderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, false, goalText)
	if err != nil {
		t.Fatalf("human response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	assertNamedGoalHumanResponseStreamAvoidsLegacyDialect(t, stdout)
	if strings.Contains(stdout, namedGoalResponseStreamHumanProgressPrefix) {
		for _, line := range strings.Split(stdout, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "--- primary result ---" {
				continue
			}
			if strings.HasPrefix(trimmed, namedGoalResponseStreamHumanProgressPrefix) {
				continue
			}
			if trimmed == packagedGoalMockWorkerAcceptedSummary {
				continue
			}
			t.Fatalf("unexpected human response-stream line %q outside canonical progress/primary-result contract", trimmed)
		}
	} else if got := strings.TrimSpace(stdout); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want primary result %q when no live progress arrived", got, packagedGoalMockWorkerAcceptedSummary)
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

type namedGoalResponseStreamJSONProgressRecord struct {
	RecordType string  `json:"recordType"`
	Sequence   int64   `json:"sequence,omitempty"`
	DispatchID *string `json:"dispatchId,omitempty"`
	Kind       string  `json:"kind"`
	EventType  string  `json:"eventType"`
	Payload    string  `json:"payload"`
}

type namedGoalResponseStreamParsedRecord struct {
	RecordType string
	Raw        json.RawMessage
	Invocation factoryapi.InvocationResponse
	Progress   namedGoalResponseStreamJSONProgressRecord
}

func runNamedGoalPrimaryOnlyInvocationCLI(
	t *testing.T,
	mockWorkersPath string,
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

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), runErr
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
	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		return namedGoalResponseStreamJSONPrimaryResultRecord{}, err
	}
	terminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		return namedGoalResponseStreamJSONPrimaryResultRecord{}, err
	}
	return namedGoalResponseStreamJSONPrimaryResultRecord{
		RecordType: namedGoalResponseStreamJSONRecordPrimary,
		Invocation: terminal,
	}, nil
}

func parseNamedGoalResponseStreamNDJSONRecords(stdout string) ([]namedGoalResponseStreamParsedRecord, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty stdout")
	}

	records := make([]namedGoalResponseStreamParsedRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var envelope struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			return nil, fmt.Errorf("decode line %q: %w", line, err)
		}
		record := namedGoalResponseStreamParsedRecord{
			RecordType: strings.TrimSpace(envelope.RecordType),
			Raw:        json.RawMessage(line),
		}
		switch record.RecordType {
		case namedGoalResponseStreamJSONRecordPrimary:
			var primary namedGoalResponseStreamJSONPrimaryResultRecord
			if err := json.Unmarshal([]byte(line), &primary); err != nil {
				return nil, fmt.Errorf("decode primary_result line %q: %w", line, err)
			}
			record.Invocation = primary.Invocation
		case namedGoalResponseStreamJSONRecordProgress:
			if err := json.Unmarshal([]byte(line), &record.Progress); err != nil {
				return nil, fmt.Errorf("decode progress line %q: %w", line, err)
			}
		case namedGoalResponseStreamJSONRecordStreamGap, namedGoalResponseStreamJSONRecordCompaction:
			// Canonical gap/compaction records only need recordType validation here.
		default:
			return nil, fmt.Errorf("unsupported recordType %q in line %q", record.RecordType, line)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no NDJSON records in stdout")
	}
	return records, nil
}

func namedGoalResponseStreamTerminalInvocation(
	records []namedGoalResponseStreamParsedRecord,
) (factoryapi.InvocationResponse, error) {
	if len(records) == 0 {
		return factoryapi.InvocationResponse{}, fmt.Errorf("no records")
	}
	if records[len(records)-1].RecordType != namedGoalResponseStreamJSONRecordPrimary {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"final record type = %q, want %q",
			records[len(records)-1].RecordType,
			namedGoalResponseStreamJSONRecordPrimary,
		)
	}
	return records[len(records)-1].Invocation, nil
}

func assertNamedGoalInvocationTerminalOutcomeParity(
	t *testing.T,
	primaryOnly factoryapi.InvocationResponse,
	streamTerminal factoryapi.InvocationResponse,
) {
	t.Helper()

	if primaryOnly.Status != streamTerminal.Status {
		t.Fatalf("status mismatch: primary-only = %q, response-stream = %q", primaryOnly.Status, streamTerminal.Status)
	}
	primaryText := invocationPrimaryResultText(t, primaryOnly)
	streamText := invocationPrimaryResultText(t, streamTerminal)
	if primaryText != streamText {
		t.Fatalf("primaryResult mismatch: primary-only = %q, response-stream = %q", primaryText, streamText)
	}
	if primaryOnly.ErrorCode == nil && streamTerminal.ErrorCode != nil {
		t.Fatalf("errorCode mismatch: primary-only = nil, response-stream = %q", *streamTerminal.ErrorCode)
	}
	if primaryOnly.ErrorCode != nil && streamTerminal.ErrorCode == nil {
		t.Fatalf("errorCode mismatch: primary-only = %q, response-stream = nil", *primaryOnly.ErrorCode)
	}
	if primaryOnly.ErrorCode != nil && streamTerminal.ErrorCode != nil &&
		*primaryOnly.ErrorCode != *streamTerminal.ErrorCode {
		t.Fatalf("errorCode mismatch: primary-only = %q, response-stream = %q", *primaryOnly.ErrorCode, *streamTerminal.ErrorCode)
	}
}

func assertNamedGoalResponseStreamJSONRecordsUseCanonicalVocabulary(
	t *testing.T,
	records []namedGoalResponseStreamParsedRecord,
) {
	t.Helper()

	for _, record := range records {
		switch record.RecordType {
		case namedGoalResponseStreamJSONRecordProgress:
			if record.Progress.Kind == "" || record.Progress.EventType == "" {
				t.Fatalf("progress record missing canonical kind/eventType: %#v", record.Progress)
			}
			if strings.Contains(string(record.Raw), "PROGRESS_FRAGMENT") ||
				strings.Contains(string(record.Raw), "RESPONSE_FRAGMENT") ||
				strings.Contains(string(record.Raw), "response.output_text.delta") {
				t.Fatalf("progress record uses legacy fragment dialect: %s", string(record.Raw))
			}
		case namedGoalResponseStreamJSONRecordStreamGap, namedGoalResponseStreamJSONRecordCompaction:
			continue
		case namedGoalResponseStreamJSONRecordPrimary:
			continue
		default:
			t.Fatalf("unsupported recordType %q", record.RecordType)
		}
	}
}

func assertNamedGoalHumanResponseStreamAvoidsLegacyDialect(t *testing.T, stdout string) {
	t.Helper()

	forbiddenTerms := []string{
		"PROGRESS_FRAGMENT",
		"RESPONSE_FRAGMENT",
		"response.output_text.delta",
		"response.completed",
		"response.failed",
		"SessionResponseStream",
		"--- invocation outcome ---",
	}
	for _, forbidden := range forbiddenTerms {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("human response-stream stdout contains legacy/internal term %q", forbidden)
		}
	}
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

