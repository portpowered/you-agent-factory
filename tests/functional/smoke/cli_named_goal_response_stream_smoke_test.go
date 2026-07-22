package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	namedGoalResponseStreamJSONRecordResponseEvent = "response_event"
	namedGoalResponseStreamJSONRecordInvocation    = "invocation_result"
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

func TestNamedGoalResponseStream_JSONModeEmitsInvocationResultRecord(t *testing.T) {
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
	if finalRecord.RecordType != namedGoalResponseStreamJSONRecordInvocation {
		t.Fatalf("final record type = %q, want %q", finalRecord.RecordType, namedGoalResponseStreamJSONRecordInvocation)
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

func TestNamedGoalResponseStream_JSONModeEmitsExactlyOneInvocationResultRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal JSON response-stream invocation_result count smoke")
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

func TestNamedGoalResponseStream_HumanModeUsesCanonicalHumanFormatNotLegacyDialect(t *testing.T) {
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
	lifecycleLines := 0
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "--- primary result ---" {
			continue
		}
		if isNamedGoalCustomerFactoryLifecycleLine(trimmed) {
			lifecycleLines++
			continue
		}
		if trimmed == packagedGoalMockWorkerAcceptedSummary {
			continue
		}
		t.Fatalf("unexpected human response-stream line %q outside canonical Factory Event presentation", trimmed)
	}
	if lifecycleLines == 0 {
		t.Fatalf("stdout has no customer Factory lifecycle lines:\n%s", stdout)
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout), packagedGoalMockWorkerAcceptedSummary) {
		t.Fatalf("final response is not last on stdout:\n%s", stdout)
	}
}

func isNamedGoalCustomerFactoryLifecycleLine(line string) bool {
	closingBracket := strings.Index(line, "] ")
	if !strings.HasPrefix(line, "[") || closingBracket < 2 {
		return false
	}
	message := line[closingBracket+2:]
	for _, prefix := range []string{
		"work accepted", "work moved", "Factory Session started", "Factory Session completed",
		"workstation queued", "workstation started", "workstation completed", "workstation failed", "workstation interrupted",
		"inference started", "inference completed", "inference failed", "workflow phase", "workflow checkpoint written",
		"final output updated",
	} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
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

type namedGoalResponseStreamJSONInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}

type namedGoalResponseStreamJSONResponseEventRecord struct {
	RecordType string                               `json:"recordType"`
	Event      factorysessions.FactoryResponseEvent `json:"event"`
}

type namedGoalResponseStreamParsedRecord struct {
	RecordType string
	Raw        json.RawMessage
	Invocation factoryapi.InvocationResponse
	Event      factorysessions.FactoryResponseEvent
}

func runNamedGoalPrimaryOnlyInvocationCLI(
	t *testing.T,
	mockWorkersPath string,
	goalText string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, publicGoal.PackagedFactoryName)

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
		"--named", publicGoal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

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
	support.InstallPackagedFactory(t, homeDir, publicGoal.PackagedFactoryName)

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
		"--named", publicGoal.PackagedFactoryName,
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
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func namedGoalResponseStreamFinalJSONRecord(stdout string) (namedGoalResponseStreamJSONInvocationResultRecord, error) {
	records, err := parseNamedGoalResponseStreamNDJSONRecords(stdout)
	if err != nil {
		return namedGoalResponseStreamJSONInvocationResultRecord{}, err
	}
	terminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		return namedGoalResponseStreamJSONInvocationResultRecord{}, err
	}
	return namedGoalResponseStreamJSONInvocationResultRecord{
		RecordType: namedGoalResponseStreamJSONRecordInvocation,
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
		recordType := strings.TrimSpace(envelope.RecordType)
		if recordType != namedGoalResponseStreamJSONRecordInvocation &&
			recordType != namedGoalResponseStreamJSONRecordResponseEvent {
			return nil, fmt.Errorf("unsupported recordType %q in line %q", recordType, line)
		}
		record := namedGoalResponseStreamParsedRecord{
			RecordType: recordType,
			Raw:        json.RawMessage(line),
		}
		switch recordType {
		case namedGoalResponseStreamJSONRecordInvocation:
			var invocation namedGoalResponseStreamJSONInvocationResultRecord
			if err := json.Unmarshal([]byte(line), &invocation); err != nil {
				return nil, fmt.Errorf("decode invocation_result line %q: %w", line, err)
			}
			record.Invocation = invocation.Invocation
		case namedGoalResponseStreamJSONRecordResponseEvent:
			var responseEvent namedGoalResponseStreamJSONResponseEventRecord
			if err := json.Unmarshal([]byte(line), &responseEvent); err != nil {
				return nil, fmt.Errorf("decode response_event line %q: %w", line, err)
			}
			if err := factorysessions.ValidateFactoryResponseEvent(responseEvent.Event); err != nil {
				return nil, fmt.Errorf("validate response_event line %q: %w", line, err)
			}
			record.Event = responseEvent.Event
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
	if records[len(records)-1].RecordType != namedGoalResponseStreamJSONRecordInvocation {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"final record type = %q, want %q",
			records[len(records)-1].RecordType,
			namedGoalResponseStreamJSONRecordInvocation,
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
		case namedGoalResponseStreamJSONRecordResponseEvent:
			if record.Event.Kind == "" || record.Event.Phase == "" {
				t.Fatalf("response_event record missing canonical kind/phase: %#v", record.Event)
			}
			if strings.Contains(string(record.Raw), "PROGRESS_FRAGMENT") ||
				strings.Contains(string(record.Raw), "RESPONSE_FRAGMENT") ||
				strings.Contains(string(record.Raw), "response.output_text.delta") {
				t.Fatalf("response_event record uses legacy fragment dialect: %s", string(record.Raw))
			}
		case namedGoalResponseStreamJSONRecordInvocation:
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
		`"recordType":"progress"`,
		`"recordType":"primary_result"`,
		`"recordType":"stream_gap"`,
		`"recordType":"compaction"`,
		"[you:progress] ",
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
