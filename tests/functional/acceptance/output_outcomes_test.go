package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	outputModeResponseStreamJSONRecordFactoryEvent = "factory_event"
	outputModeResponseStreamJSONRecordInvocation   = "invocation_result"
)

func TestPrimaryOutputMode_SuccessfulNamedGoal_WritesAuthoritativePrimaryResultOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI primary output mode named goal acceptance")
	}

	session, mockWorkersPath := prepareNamedGoalOutputAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	goalText := fmt.Sprintf("acceptance-primary-output-%d", time.Now().UnixNano())
	result, err := session.Run(ctx, namedGoalOutputRunArgs(session, "", false, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "primary-output-named-goal", result, err)

	if got := result.Stdout; got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want authoritative primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(result.Stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful primary-only run", result.Stderr)
	}
}

func TestPrimaryOutputMode_JSONMode_WritesCompletedInvocationResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI primary JSON output mode named goal acceptance")
	}

	session, mockWorkersPath := prepareNamedGoalOutputAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	goalText := fmt.Sprintf("acceptance-primary-json-output-%d", time.Now().UnixNano())
	result, err := session.Run(ctx, namedGoalOutputRunArgs(session, "", true, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "primary-json-output-named-goal", result, err)

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &response); decodeErr != nil {
		t.Fatalf("decode primary JSON stdout: %v\nstdout:\n%s", decodeErr, result.Stdout)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
}

func TestStreamOutputMode_HumanMode_RendersCanonicalProgressAndTerminalPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI human response-stream output mode named goal acceptance")
	}

	session, mockWorkersPath := prepareNamedGoalOutputAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	goalText := fmt.Sprintf("acceptance-stream-human-output-%d", time.Now().UnixNano())
	result, err := session.Run(ctx, namedGoalOutputRunArgs(session, "response-stream", false, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "stream-human-output-named-goal", result, err)

	assertHumanResponseStreamAvoidsLegacyDialect(t, result.Stdout)
	lifecycleLines := 0
	for _, line := range strings.Split(result.Stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "--- primary result ---" {
			continue
		}
		if isCustomerFactoryLifecycleLine(trimmed) {
			lifecycleLines++
			continue
		}
		if trimmed == packagedGoalMockWorkerAcceptedSummary {
			continue
		}
		t.Fatalf("unexpected human response-stream line %q outside canonical Factory Event presentation", trimmed)
	}
	if lifecycleLines == 0 {
		t.Fatalf("stdout has no customer Factory lifecycle lines:\n%s", result.Stdout)
	}
	if !strings.HasSuffix(strings.TrimSpace(result.Stdout), packagedGoalMockWorkerAcceptedSummary) {
		t.Fatalf("final response is not last on stdout:\n%s", result.Stdout)
	}
}

func TestStreamOutputMode_JSONMode_EmitsCanonicalNDJSONRecordsWithTerminalInvocationResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI JSON response-stream output mode named goal acceptance")
	}

	session, mockWorkersPath := prepareNamedGoalOutputAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	goalText := fmt.Sprintf("acceptance-stream-json-output-%d", time.Now().UnixNano())
	result, err := session.Run(ctx, namedGoalOutputRunArgs(session, "response-stream", true, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "stream-json-output-named-goal", result, err)

	records, parseErr := parseResponseStreamNDJSONRecords(result.Stdout)
	if parseErr != nil {
		t.Fatalf("parse JSON response-stream stdout: %v\nstdout:\n%s", parseErr, result.Stdout)
	}
	assertResponseStreamJSONRecordsUseCanonicalVocabulary(t, records)

	terminal, terminalErr := responseStreamTerminalInvocation(records)
	if terminalErr != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", terminalErr, result.Stdout)
	}
	if terminal.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal status = %q, want COMPLETED", terminal.Status)
	}
	if got := invocationPrimaryResultText(t, terminal); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("terminal primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}

	invocationCount := 0
	for _, record := range records {
		if record.RecordType == outputModeResponseStreamJSONRecordInvocation {
			invocationCount++
		}
	}
	if invocationCount != 1 {
		t.Fatalf("invocation_result record count = %d, want exactly 1", invocationCount)
	}
	if records[len(records)-1].RecordType != outputModeResponseStreamJSONRecordInvocation {
		t.Fatalf("final record type = %q, want %q", records[len(records)-1].RecordType, outputModeResponseStreamJSONRecordInvocation)
	}
}

func TestStreamOutputMode_JSONMode_FailureBeforeSessionWritesNoTerminalRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI JSON response-stream pre-session failure acceptance")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx,
		"--json", "run", "--named", "@you/missing", "--output", "response-stream", "--no-record", "prompt",
	)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("expected pre-session failure, got result=%#v err=%v", result, err)
	}
	if result.Stdout != "" {
		t.Fatalf("stdout = %q, want no Factory Event or terminal record", result.Stdout)
	}
}

func TestStreamOutputMode_PrimaryAndStreamModesAgreeOnTerminalOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI primary-only vs response-stream parity acceptance")
	}

	session, mockWorkersPath := prepareNamedGoalOutputAcceptanceSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	goalText := fmt.Sprintf("acceptance-output-mode-parity-%d", time.Now().UnixNano())

	primaryResult, primaryErr := session.Run(ctx, namedGoalOutputRunArgs(session, "", true, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "output-parity-primary-json", primaryResult, primaryErr)

	streamResult, streamErr := session.Run(ctx, namedGoalOutputRunArgs(session, "response-stream", true, mockWorkersPath, goalText)...)
	session.RequireSuccess(t, "output-parity-response-stream-json", streamResult, streamErr)

	var primaryResponse factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(primaryResult.Stdout)), &primaryResponse); decodeErr != nil {
		t.Fatalf("decode primary JSON stdout: %v\nstdout:\n%s", decodeErr, primaryResult.Stdout)
	}

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

func prepareNamedGoalOutputAcceptanceSession(t *testing.T) (*builtcliacceptance.Session, string) {
	t.Helper()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	_, initOutcome := initializeConfig(t, ctx, session, "output-mode-config-init")
	configPath := initOutcome.ConfigPath
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if writeErr := os.WriteFile(configPath, configBody, 0o600); writeErr != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, writeErr)
	}

	return session, writePackagedGoalMockWorkersConfig(t)
}

func namedGoalOutputRunArgs(
	session *builtcliacceptance.Session,
	outputMode string,
	jsonMode bool,
	mockWorkersPath string,
	goalText string,
) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--quiet",
	)
	if outputMode == "response-stream" {
		args = append(args, "--output", "response-stream")
	}
	args = append(args, mockWorkersPath, goalText)
	if jsonMode {
		args = append([]string{"--json"}, args...)
	}
	return args
}

type responseStreamJSONInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}

type responseStreamJSONFactoryEventRecord struct {
	RecordType string                          `json:"recordType"`
	Event      factorydefinitions.FactoryEvent `json:"event"`
}

type responseStreamParsedRecord struct {
	RecordType string
	Raw        json.RawMessage
	Invocation factoryapi.InvocationResponse
	Event      factorydefinitions.FactoryEvent
}

func parseResponseStreamNDJSONRecords(stdout string) ([]responseStreamParsedRecord, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty stdout")
	}

	records := make([]responseStreamParsedRecord, 0, len(lines))
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
		if recordType != outputModeResponseStreamJSONRecordInvocation &&
			recordType != outputModeResponseStreamJSONRecordFactoryEvent {
			return nil, fmt.Errorf("unsupported recordType %q in line %q", recordType, line)
		}
		record := responseStreamParsedRecord{
			RecordType: recordType,
			Raw:        json.RawMessage(line),
		}
		switch recordType {
		case outputModeResponseStreamJSONRecordInvocation:
			if err := requireNDJSONRecordShape(line, "response"); err != nil {
				return nil, err
			}
			var invocation responseStreamJSONInvocationResultRecord
			if err := json.Unmarshal([]byte(line), &invocation); err != nil {
				return nil, fmt.Errorf("decode invocation_result line %q: %w", line, err)
			}
			record.Invocation = invocation.Response
		case outputModeResponseStreamJSONRecordFactoryEvent:
			if err := requireNDJSONRecordShape(line, "event"); err != nil {
				return nil, err
			}
			var factoryEvent responseStreamJSONFactoryEventRecord
			if err := json.Unmarshal([]byte(line), &factoryEvent); err != nil {
				return nil, fmt.Errorf("decode factory_event line %q: %w", line, err)
			}
			if factoryEvent.Event.SchemaVersion == "" || factoryEvent.Event.Id == "" || factoryEvent.Event.Type == "" {
				return nil, fmt.Errorf("factory_event line is incomplete: %q", line)
			}
			record.Event = factoryEvent.Event
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no NDJSON records in stdout")
	}
	return records, nil
}

func requireNDJSONRecordShape(line, payloadKey string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return err
	}
	if len(fields) != 2 || len(fields["recordType"]) == 0 || len(fields[payloadKey]) == 0 {
		return fmt.Errorf("record must contain only recordType and %s: %q", payloadKey, line)
	}
	return nil
}

func responseStreamTerminalInvocation(records []responseStreamParsedRecord) (factoryapi.InvocationResponse, error) {
	if len(records) == 0 {
		return factoryapi.InvocationResponse{}, fmt.Errorf("no records")
	}
	if records[len(records)-1].RecordType != outputModeResponseStreamJSONRecordInvocation {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"final record type = %q, want %q",
			records[len(records)-1].RecordType,
			outputModeResponseStreamJSONRecordInvocation,
		)
	}
	return records[len(records)-1].Invocation, nil
}

func assertInvocationTerminalOutcomeParity(
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

func assertResponseStreamJSONRecordsUseCanonicalVocabulary(t *testing.T, records []responseStreamParsedRecord) {
	t.Helper()

	previousSequence := -1
	previousSessionSequence := -1
	for _, record := range records {
		switch record.RecordType {
		case outputModeResponseStreamJSONRecordFactoryEvent:
			if record.Event.Context.Sequence <= previousSequence {
				t.Fatalf("Factory Event sequence %d follows %d", record.Event.Context.Sequence, previousSequence)
			}
			previousSequence = record.Event.Context.Sequence
			if record.Event.Context.SessionSequence != nil {
				if *record.Event.Context.SessionSequence <= previousSessionSequence {
					t.Fatalf("Factory Session sequence %d follows %d", *record.Event.Context.SessionSequence, previousSessionSequence)
				}
				previousSessionSequence = *record.Event.Context.SessionSequence
			}
			var payload any
			if err := json.Unmarshal(record.Event.Payload, &payload); err != nil {
				t.Fatalf("decode Factory Event payload: %v", err)
			}
			if key := firstPrivateFactoryEventPayloadKey(payload); key != "" {
				t.Fatalf("factory_event contains provider-only field %q: %s", key, record.Raw)
			}
		case outputModeResponseStreamJSONRecordInvocation:
			continue
		default:
			t.Fatalf("unsupported recordType %q", record.RecordType)
		}
	}
}

func firstPrivateFactoryEventPayloadKey(value any) string {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range []string{"diagnostics", "response", "providerSession", "provider_session", "providerSessionRef", "textDelta", "toolCallId", "toolCalls"} {
			if _, exists := value[key]; exists {
				return key
			}
		}
		for _, child := range value {
			if key := firstPrivateFactoryEventPayloadKey(child); key != "" {
				return key
			}
		}
	case []any:
		for _, child := range value {
			if key := firstPrivateFactoryEventPayloadKey(child); key != "" {
				return key
			}
		}
	}
	return ""
}

func assertHumanResponseStreamAvoidsLegacyDialect(t *testing.T, stdout string) {
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

func isCustomerFactoryLifecycleLine(line string) bool {
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

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}
