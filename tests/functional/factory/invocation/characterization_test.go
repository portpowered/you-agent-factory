package oneshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	btrcOneShotPrimaryResult = "btrc one-shot primary result COMPLETE"
	btrcOneShotFailureStderr = "btrc one-shot provider rejection"
)

var btrcOneShotEventOrder = []interfaces.FactoryEventType{
	interfaces.FactoryEventTypeRunRequest,
	interfaces.FactoryEventTypeInitialStructureRequest,
	interfaces.FactoryEventTypeSessionStarted,
	interfaces.FactoryEventTypeFactoryStateResponse,
	interfaces.FactoryEventTypeWorkRequest,
	interfaces.FactoryEventTypeDispatchRequest,
	interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	interfaces.FactoryEventTypeModelRequest,
	interfaces.FactoryEventTypeModelResponse,
	interfaces.FactoryEventTypeDispatchResponse,
	interfaces.FactoryEventTypeFactoryStateResponse,
	interfaces.FactoryEventTypeRunResponse,
	interfaces.FactoryEventTypeSessionResultUpdated,
	interfaces.FactoryEventTypeSessionCompleted,
}

// TestBTRCP0OneShotSuccessCharacterization freezes the successful one-shot
// CLI invocation at the root Process.Execute boundary. The recorded artifact
// is the expected source for canonical Factory Event order; the returned JSON
// response is asserted independently as the caller-visible terminal result.
func TestBTRCP0OneShotSuccessCharacterization(t *testing.T) {
	runner := support.NewRecordingCommandRunner(btrcOneShotPrimaryResult)
	run := runBTRCOneShot(t, runner)

	if run.executeErr != nil {
		t.Fatalf("Process.Execute(one-shot success) error = %v\nstdout:\n%s\nstderr:\n%s", run.executeErr, run.stdout, run.stderr)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want exactly one", runner.CallCount())
	}

	assertBTRCOneShotEventOrder(t, run.artifact.Events)
	requestID, traceID, workID, dispatchID := assertBTRCOneShotCorrelation(t, run.artifact.Events, run.response)
	assertBTRCOneShotAcceptedDispatch(t, run.artifact.Events, workID, dispatchID)
	assertBTRCOneShotTerminalSession(t, run.artifact.Events, btrcOneShotSessionSucceeded)
	assertBTRCOneShotResponse(t, run.response, factoryapi.InvocationTerminalStatusCompleted, requestID, traceID, btrcOneShotPrimaryResult)
	assertBTRCOneShotResponseStreamHasOneTerminalRecord(t, run.stdout)
}

// TestBTRCP0OneShotProviderFailureCharacterization freezes the current
// terminal provider failure path, including the emitted failed dispatch,
// typed failure metadata, CLI error, and exactly-once terminal publication.
func TestBTRCP0OneShotProviderFailureCharacterization(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 7,
		Stderr:   []byte(btrcOneShotFailureStderr),
	})
	run := runBTRCOneShot(t, runner)

	if run.executeErr == nil {
		t.Fatal("Process.Execute(one-shot provider failure) error = nil, want terminal invocation error")
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want exactly one", runner.CallCount())
	}

	assertBTRCOneShotEventOrder(t, run.artifact.Events)
	requestID, traceID, workID, dispatchID := assertBTRCOneShotCorrelation(t, run.artifact.Events, run.response)
	assertBTRCOneShotFailedDispatch(t, run.artifact.Events, workID, dispatchID)
	assertBTRCOneShotTerminalSession(t, run.artifact.Events, btrcOneShotSessionSucceeded)
	assertBTRCOneShotResponse(t, run.response, factoryapi.InvocationTerminalStatusFailed, requestID, traceID, "")
	if run.response.ErrorCode == nil || *run.response.ErrorCode != factoryapi.INVOCATIONRUNTIMEFAILURE {
		t.Fatalf("failure response errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", run.response.ErrorCode)
	}
	if run.response.PrimaryResult != nil {
		t.Fatalf("failure response primaryResult = %#v, want nil", run.response.PrimaryResult)
	}
	if !strings.Contains(run.stderr, string(factoryapi.INVOCATIONRUNTIMEFAILURE)) {
		t.Fatalf("failure stderr = %q, want typed invocation error code", run.stderr)
	}
	assertBTRCOneShotResponseStreamHasOneTerminalRecord(t, run.stdout)
}

const btrcOneShotSessionSucceeded = interfaces.FactorySessionLifecycleStatusSucceeded

type btrcOneShotRun struct {
	artifact   *interfaces.ReplayArtifact
	response   factoryapi.InvocationResponse
	stdout     string
	stderr     string
	executeErr error
}

type btrcOneShotProviderRunner interface {
	platformprocess.CommandRunner
	CallCount() int
}

func runBTRCOneShot(t *testing.T, runner btrcOneShotProviderRunner) btrcOneShotRun {
	t.Helper()

	factoryDir := scaffoldBTRCOneShotFactory(t)
	artifactPath := filepath.Join(t.TempDir(), "btrc-one-shot.replay.json")
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", factoryPath,
		"--record", artifactPath,
		"--output", "response-stream",
		"btrc one-shot request",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir

	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	executeErr := process.Execute(inputs.Input)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	return btrcOneShotRun{
		artifact:   artifact,
		response:   support.DecodeInvocationResponseJSON(t, inputs.Stdout()),
		stdout:     inputs.Stdout(),
		stderr:     inputs.Stderr(),
		executeErr: executeErr,
	}
}

func scaffoldBTRCOneShotFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func assertBTRCOneShotEventOrder(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	got := make([]interfaces.FactoryEventType, len(events))
	for index, event := range events {
		got[index] = event.Type
		if event.Context.Sequence != index {
			t.Fatalf("event[%d] sequence = %d, want %d", index, event.Context.Sequence, index)
		}
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("event[%d] id is empty", index)
		}
	}
	if !reflect.DeepEqual(got, btrcOneShotEventOrder) {
		t.Fatalf("one-shot canonical event order = %v, want %v", got, btrcOneShotEventOrder)
	}
}

func assertBTRCOneShotCorrelation(
	t *testing.T,
	events []interfaces.FactoryEvent,
	response factoryapi.InvocationResponse,
) (string, string, string, string) {
	t.Helper()
	if response.RequestId == "" || response.TraceId == "" {
		t.Fatalf("invocation identity = request:%q trace:%q, want both non-empty", response.RequestId, response.TraceId)
	}

	var requestEvent *interfaces.FactoryEvent
	var workID string
	var dispatchID string
	for index := range events {
		event := &events[index]
		if event.Type == interfaces.FactoryEventTypeWorkRequest {
			if requestEvent != nil {
				t.Fatalf("duplicate WORK_REQUEST events at %d and %d", requestEvent.Context.Sequence, event.Context.Sequence)
			}
			requestEvent = event
			workIDs := btrcOneShotStringSlice(event.Context.WorkIDs)
			if len(workIDs) != 1 {
				t.Fatalf("WORK_REQUEST work ids = %#v, want one", workIDs)
			}
			workID = workIDs[0]
			if event.Context.RequestID == nil || *event.Context.RequestID != response.RequestId {
				t.Fatalf("WORK_REQUEST request id = %v, want %q", event.Context.RequestID, response.RequestId)
			}
			if got := btrcOneShotStringSlice(event.Context.TraceIDs); !reflect.DeepEqual(got, []string{response.TraceId}) {
				t.Fatalf("WORK_REQUEST trace ids = %#v, want [%q]", got, response.TraceId)
			}
		}
		if event.Type == interfaces.FactoryEventTypeDispatchRequest {
			if event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) == "" {
				t.Fatalf("DISPATCH_REQUEST context = %#v, want dispatch identity", event.Context)
			}
			if dispatchID != "" {
				t.Fatalf("duplicate DISPATCH_REQUEST events for one-shot dispatch %q", dispatchID)
			}
			dispatchID = *event.Context.DispatchID
			if got := btrcOneShotStringSlice(event.Context.WorkIDs); !reflect.DeepEqual(got, []string{workID}) {
				t.Fatalf("DISPATCH_REQUEST work ids = %#v, want [%q]", got, workID)
			}
			if got := btrcOneShotStringSlice(event.Context.TraceIDs); !reflect.DeepEqual(got, []string{response.TraceId}) {
				t.Fatalf("DISPATCH_REQUEST trace ids = %#v, want [%q]", got, response.TraceId)
			}
		}
	}
	if requestEvent == nil || workID == "" || dispatchID == "" {
		t.Fatalf("one-shot correlation = request:%v work:%q dispatch:%q, want all identities", requestEvent != nil, workID, dispatchID)
	}
	if response.WorkId != nil && *response.WorkId != workID {
		t.Fatalf("invocation workId = %q, want %q", *response.WorkId, workID)
	}
	return response.RequestId, response.TraceId, workID, dispatchID
}

func assertBTRCOneShotAcceptedDispatch(
	t *testing.T,
	events []interfaces.FactoryEvent,
	wantWorkID, wantDispatchID string,
) {
	t.Helper()
	var payload workerexecution.DispatchResponseEventPayload
	count := 0
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		count++
		if event.Context.DispatchID == nil || *event.Context.DispatchID != wantDispatchID {
			t.Fatalf("DISPATCH_RESPONSE dispatch id = %#v, want %q", event.Context.DispatchID, wantDispatchID)
		}
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode accepted DISPATCH_RESPONSE: %v", err)
		}
	}
	if count != 1 {
		t.Fatalf("DISPATCH_RESPONSE count = %d, want exactly one", count)
	}
	if payload.Outcome != workerexecution.OutcomeAccepted || payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("accepted dispatch payload = %#v, want one accepted output work", payload)
	}
	outputWork := (*payload.OutputWork)[0]
	if outputWork.WorkID != wantWorkID || outputWork.State == nil || outputWork.State.Name != "complete" {
		t.Fatalf("accepted output work = %#v, want %q in complete state", outputWork, wantWorkID)
	}
	if payload.Output == nil || !strings.Contains(*payload.Output, btrcOneShotPrimaryResult) {
		t.Fatalf("accepted dispatch output = %#v, want primary result", payload.Output)
	}
}

func assertBTRCOneShotFailedDispatch(
	t *testing.T,
	events []interfaces.FactoryEvent,
	wantWorkID, wantDispatchID string,
) {
	t.Helper()
	var payload workerexecution.DispatchResponseEventPayload
	count := 0
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		count++
		if event.Context.DispatchID == nil || *event.Context.DispatchID != wantDispatchID {
			t.Fatalf("failed DISPATCH_RESPONSE dispatch id = %#v, want %q", event.Context.DispatchID, wantDispatchID)
		}
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode failed DISPATCH_RESPONSE: %v", err)
		}
	}
	if count != 1 {
		t.Fatalf("failed DISPATCH_RESPONSE count = %d, want exactly one", count)
	}
	if payload.Outcome != workerexecution.OutcomeFailed || payload.FailureDetail == nil || payload.ProviderFailure == nil {
		t.Fatalf("failed dispatch payload = %#v, want typed failure metadata", payload)
	}
	if payload.FailureDetail.Reason != workerexecution.WorkFailureTypeUnknown ||
		payload.ProviderFailure.Family != workerexecution.WorkFailureFamilyTerminal ||
		payload.ProviderFailure.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("failed dispatch classification = detail:%#v provider:%#v, want terminal unknown", payload.FailureDetail, payload.ProviderFailure)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("failed output work = %#v, want one terminal work", payload.OutputWork)
	}
	outputWork := (*payload.OutputWork)[0]
	if outputWork.WorkID != wantWorkID || outputWork.State == nil || outputWork.State.Name != "failed" {
		t.Fatalf("failed output work = %#v, want %q in failed state", outputWork, wantWorkID)
	}
}

func assertBTRCOneShotTerminalSession(
	t *testing.T,
	events []interfaces.FactoryEvent,
	wantStatus interfaces.FactorySessionLifecycleStatus,
) {
	t.Helper()
	resultCount := 0
	completedCount := 0
	var resultPayload interfaces.FactorySessionResultUpdatedEventPayload
	var completedPayload interfaces.FactorySessionCompletedEventPayload
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeSessionStarted ||
			event.Type == interfaces.FactoryEventTypeSessionResultUpdated ||
			event.Type == interfaces.FactoryEventTypeSessionCompleted {
			if event.Context.SessionID == nil || *event.Context.SessionID != "~default" {
				t.Fatalf("%s session id = %#v, want ~default", event.Type, event.Context.SessionID)
			}
		}
		switch event.Type {
		case interfaces.FactoryEventTypeSessionResultUpdated:
			resultCount++
			if err := event.DecodePayload(&resultPayload); err != nil {
				t.Fatalf("decode SESSION_RESULT_UPDATED: %v", err)
			}
		case interfaces.FactoryEventTypeSessionCompleted:
			completedCount++
			if err := event.DecodePayload(&completedPayload); err != nil {
				t.Fatalf("decode SESSION_COMPLETED: %v", err)
			}
		}
	}
	if resultCount != 1 || completedCount != 1 {
		t.Fatalf("terminal session publication = result:%d completed:%d, want exactly one each", resultCount, completedCount)
	}
	if resultPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal {
		t.Fatalf("SESSION_RESULT_UPDATED resultStatus = %q, want FINAL", resultPayload.ResultStatus)
	}
	if completedPayload.FinalStatus != wantStatus || completedPayload.ResultStatus == nil || *completedPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal {
		t.Fatalf("SESSION_COMPLETED projection = %#v, want %s/FINAL", completedPayload, wantStatus)
	}
}

func assertBTRCOneShotResponse(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantStatus factoryapi.InvocationTerminalStatus,
	wantRequestID, wantTraceID, wantPrimary string,
) {
	t.Helper()
	if response.Status != wantStatus || response.RequestId != wantRequestID || response.TraceId != wantTraceID {
		t.Fatalf("InvocationResponse identity/status = %#v, want status:%q request:%q trace:%q", response, wantStatus, wantRequestID, wantTraceID)
	}
	if wantPrimary == "" {
		return
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("successful primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primaryResult text part: %v", err)
	}
	if part.Text != wantPrimary {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, wantPrimary)
	}
}

func assertBTRCOneShotResponseStreamHasOneTerminalRecord(t *testing.T, stdout string) {
	t.Helper()
	lines := strings.Split(stdout, "\n")
	terminalRecords := 0
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode response-stream record %d: %v\nline: %s", index, err, line)
		}
		switch record.RecordType {
		case "factory_event":
		case "invocation_result":
			terminalRecords++
		default:
			t.Fatalf("response-stream record %d type = %q, want factory_event or invocation_result", index, record.RecordType)
		}
	}
	if terminalRecords != 1 {
		t.Fatalf("response-stream invocation_result records = %d, want exactly one", terminalRecords)
	}
}

func btrcOneShotStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

var _ platformprocess.CommandRunner = (*support.RecordingCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*support.ShapedProviderCommandRunner)(nil)
