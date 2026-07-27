package output_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	goalFactoryName          = "@you/goal"
	factoryEventRecordType   = "factory_event"
	invocationResultType     = "invocation_result"
	wantInvocationResultText = "mock worker accepted"
)

// TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult proves a successful
// CLI NDJSON response-stream run emits public Factory Event records followed by
// exactly one terminal InvocationResult that decodes through the public contract.
func TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult(t *testing.T) {
	stdout := runGoalResponseStream(t)
	records := decodeNDJSONRecords(t, stdout)

	if len(records) < 2 {
		t.Fatalf("NDJSON records = %d, want Factory Events followed by one invocation result\nstdout:\n%s", len(records), stdout)
	}

	factoryEventCount := 0
	invocationResultCount := 0
	for index, record := range records {
		switch record.RecordType {
		case factoryEventRecordType:
			if invocationResultCount != 0 {
				t.Fatalf("Factory Event record %d follows terminal invocation result", index)
			}
			assertFactoryEventRecord(t, record, index)
			factoryEventCount++
		case invocationResultType:
			invocationResultCount++
			if index != len(records)-1 {
				t.Fatalf("invocation_result record index = %d, want terminal index %d", index, len(records)-1)
			}
			assertInvocationResultRecord(t, record, index)
		default:
			t.Fatalf("record %d has unsupported recordType %q", index, record.RecordType)
		}
	}
	if factoryEventCount == 0 {
		t.Fatal("response stream contains no canonical Factory Event records")
	}
	if invocationResultCount != 1 {
		t.Fatalf("invocation_result record count = %d, want exactly 1", invocationResultCount)
	}

	for _, forbidden := range []string{
		"FactoryResponseEvent", "response_event", "provider_session", "providerSession",
		"textDelta", "toolCallId", "toolCalls",
		`"recordType":"progress"`, `"recordType":"compaction"`, `"recordType":"primary_result"`,
		`"primary_result":`,
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("NDJSON exposed private or retired stream vocabulary %q:\n%s", forbidden, stdout)
		}
	}
}

// TestCLINDJSONSequenceIsMonotonic proves CLI NDJSON response-stream Factory Event
// records expose strictly increasing context.sequence values and, when present,
// strictly increasing Factory Session sequence values, with no records after the
// terminal InvocationResult.
func TestCLINDJSONSequenceIsMonotonic(t *testing.T) {
	stdout := runGoalResponseStream(t)
	records := decodeNDJSONRecords(t, stdout)

	previousSequence := -1
	previousSessionSequence := -1
	factoryEventCount := 0
	invocationResultSeen := false
	for index, record := range records {
		switch record.RecordType {
		case factoryEventRecordType:
			if invocationResultSeen {
				t.Fatalf("Factory Event record %d follows terminal invocation result", index)
			}
			assertFactoryEventSequenceMonotonic(t, record, index, &previousSequence, &previousSessionSequence)
			factoryEventCount++
		case invocationResultType:
			if invocationResultSeen {
				t.Fatalf("second invocation_result at record %d", index)
			}
			invocationResultSeen = true
			if index != len(records)-1 {
				t.Fatalf("invocation_result record index = %d, want terminal index %d", index, len(records)-1)
			}
		default:
			t.Fatalf("record %d has unsupported recordType %q", index, record.RecordType)
		}
	}
	if factoryEventCount == 0 {
		t.Fatal("response stream contains no Factory Event records to order")
	}
	if !invocationResultSeen {
		t.Fatal("response stream missing terminal invocation_result")
	}
}

// TestCLINDJSONFailureEndsWithOneTerminalResult proves a deterministic terminal
// invocation failure under CLI NDJSON response-stream mode ends with exactly one
// failed InvocationResult and emits no stream records after that terminal record.
func TestCLINDJSONFailureEndsWithOneTerminalResult(t *testing.T) {
	stdout := runGoalResponseStreamFailure(t)
	records := decodeNDJSONRecords(t, stdout)

	if len(records) < 2 {
		t.Fatalf("NDJSON records = %d, want Factory Events followed by one terminal invocation result\nstdout:\n%s", len(records), stdout)
	}

	invocationResultCount := 0
	invocationResultIndex := -1
	for index, record := range records {
		switch record.RecordType {
		case factoryEventRecordType:
			if invocationResultCount != 0 {
				t.Fatalf("Factory Event record %d follows terminal invocation result", index)
			}
		case invocationResultType:
			invocationResultCount++
			invocationResultIndex = index
		default:
			t.Fatalf("record %d has unsupported recordType %q", index, record.RecordType)
		}
	}
	if invocationResultCount != 1 {
		t.Fatalf("invocation_result record count = %d, want exactly 1", invocationResultCount)
	}
	if invocationResultIndex != len(records)-1 {
		t.Fatalf("invocation_result record index = %d, want terminal index %d", invocationResultIndex, len(records)-1)
	}
	assertFailedInvocationResultRecord(t, records[invocationResultIndex], invocationResultIndex)
}

type ndjsonRecord struct {
	RecordType string
	Payload    json.RawMessage
	Raw        string
}

func decodeNDJSONRecords(t *testing.T, stdout string) []ndjsonRecord {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	records := make([]ndjsonRecord, 0, len(lines))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("decode NDJSON record %d: %v\nline: %s", index, err, line)
		}
		var recordType string
		if err := json.Unmarshal(fields["recordType"], &recordType); err != nil {
			t.Fatalf("decode recordType for record %d: %v\nline: %s", index, err, line)
		}
		payloadKey := "event"
		if recordType == invocationResultType {
			payloadKey = "response"
		}
		if len(fields) != 2 || len(fields[payloadKey]) == 0 {
			t.Fatalf("record %d fields = %v, want only recordType and %s", index, mapKeys(fields), payloadKey)
		}
		records = append(records, ndjsonRecord{RecordType: recordType, Payload: fields[payloadKey], Raw: line})
	}
	return records
}

func assertFactoryEventRecord(t *testing.T, record ndjsonRecord, index int) {
	t.Helper()
	var event factorydefinitions.FactoryEvent
	if err := json.Unmarshal(record.Payload, &event); err != nil {
		t.Fatalf("decode Factory Event record %d: %v\nline: %s", index, err, record.Raw)
	}
	if event.SchemaVersion == "" || event.Id == "" || event.Type == "" {
		t.Fatalf("Factory Event record %d is incomplete: %#v", index, event)
	}

	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode Factory Event payload for record %d: %v", index, err)
	}
	if key := privatePayloadKey(payload); key != "" {
		t.Fatalf("Factory Event record %d exposes provider-only field %q: %s", index, key, record.Raw)
	}
}

func assertFactoryEventSequenceMonotonic(
	t *testing.T,
	record ndjsonRecord,
	index int,
	previousSequence *int,
	previousSessionSequence *int,
) {
	t.Helper()
	var event factorydefinitions.FactoryEvent
	if err := json.Unmarshal(record.Payload, &event); err != nil {
		t.Fatalf("decode Factory Event record %d: %v\nline: %s", index, err, record.Raw)
	}
	if event.Context.Sequence <= *previousSequence {
		t.Fatalf("Factory Event sequence %d follows %d", event.Context.Sequence, *previousSequence)
	}
	*previousSequence = event.Context.Sequence
	if event.Context.SessionSequence != nil {
		if *event.Context.SessionSequence <= *previousSessionSequence {
			t.Fatalf("Factory Session sequence %d follows %d", *event.Context.SessionSequence, *previousSessionSequence)
		}
		*previousSessionSequence = *event.Context.SessionSequence
	}
}

func assertInvocationResultRecord(t *testing.T, record ndjsonRecord, index int) {
	t.Helper()
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(record.Payload, &response); err != nil {
		t.Fatalf("decode invocation result record %d: %v\nline: %s", index, err, record.Raw)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text content part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text content: %v", err)
	}
	if part.Text != wantInvocationResultText {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, wantInvocationResultText)
	}
}

func assertFailedInvocationResultRecord(t *testing.T, record ndjsonRecord, index int) {
	t.Helper()
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(record.Payload, &response); err != nil {
		t.Fatalf("decode invocation result record %d: %v\nline: %s", index, err, record.Raw)
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusFailed)
	}
	if response.ErrorCode == nil || response.Message == nil {
		t.Fatalf("failed InvocationResponse lacks error detail: %#v", response)
	}
}

func privatePayloadKey(value any) string {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range []string{"diagnostics", "response", "providerSession", "provider_session", "textDelta", "toolCallId", "toolCalls"} {
			if _, exists := value[key]; exists {
				return key
			}
		}
		for _, child := range value {
			if key := privatePayloadKey(child); key != "" {
				return key
			}
		}
	case []any:
		for _, child := range value {
			if key := privatePayloadKey(child); key != "" {
				return key
			}
		}
	}
	return ""
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func runGoalResponseStream(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "--json", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic event integrity contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	return inputs.Stdout()
}

func runGoalResponseStreamFailure(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, rejectingGoalMockWorkers())
	args := []string{
		"you", "--json", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic terminal failure",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err == nil {
		t.Fatalf("Process.Execute(%v) error = nil, want terminal invocation failure\nstdout:\n%s\nstderr:\n%s", args, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stdout() == "" {
		t.Fatalf("stdout empty, want NDJSON stream ending in failed invocation_result\nstderr:\n%s", inputs.Stderr())
	}
	return inputs.Stdout()
}

func rejectingGoalMockWorkers() *workers.MockWorkersConfig {
	exitCode := 7
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stderr:   "deterministic worker rejection",
				ExitCode: &exitCode,
			},
		}},
	}
}
