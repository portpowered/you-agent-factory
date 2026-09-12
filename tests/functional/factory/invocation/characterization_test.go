package oneshot_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	interfaces.FactoryEventTypeAgentRunResponse,
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

const (
	localAIFactoryInferenceSuccess      = "factory LocalAI inference COMPLETE"
	localAIFactoryInferenceFailure      = "controlled LocalAI backend protocol failure"
	localAIFactoryInferenceSource       = "hf://characterization/localai-factory-llm.gguf"
	localAIFactoryInferenceRevision     = "0123456789abcdef0123456789abcdef01234567"
	localAIFactoryInferenceBackend      = "localai-llamacpp"
	localAIFactoryInferencePlatformOS   = "linux"
	localAIFactoryInferencePlatformArch = "amd64"
)

// TestLocalAIFactoryInferenceCharacterization freezes the smallest Factory
// inference journey that reaches the managed LocalAI Models path. The test
// replaces only published model effects, then observes the caller-facing
// result and the canonical event ledger produced by Process.Execute.
func TestLocalAIFactoryInferenceCharacterization(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                 string
		response             string
		failure              error
		waitBeforeCompletion bool
		wantStatus           factoryapi.InvocationTerminalStatus
		wantSession          interfaces.FactorySessionLifecycleStatus
	}{
		{
			name: "success", response: localAIFactoryInferenceSuccess,
			waitBeforeCompletion: false,
			wantStatus:           factoryapi.InvocationTerminalStatusCompleted,
			wantSession:          interfaces.FactorySessionLifecycleStatusSucceeded,
		},
		{
			name: "backend protocol failure", failure: errors.New(localAIFactoryInferenceFailure),
			waitBeforeCompletion: true,
			wantStatus:           factoryapi.InvocationTerminalStatusFailed,
			wantSession:          interfaces.FactorySessionLifecycleStatusSucceeded,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			const prompt = "Factory LocalAI characterization prompt"
			run := runLocalAIFactoryInference(t, prompt, test.response, test.failure, test.waitBeforeCompletion)
			if test.failure == nil {
				if run.executeErr != nil {
					t.Fatalf("Process.Execute(Factory LocalAI success) error = %v\nstdout:\n%s\nstderr:\n%s", run.executeErr, run.stdout, run.stderr)
				}
				assertBTRCOneShotResponse(t, run.response, test.wantStatus, run.response.RequestId, run.response.TraceId, test.response)
				assertLocalAIFactoryAcceptedDispatch(t, run.artifact.Events, test.response)
			} else {
				if run.executeErr == nil {
					t.Fatal("Process.Execute(Factory LocalAI protocol failure) error = nil, want terminal invocation error")
				}
				if run.response.ErrorCode == nil || *run.response.ErrorCode != factoryapi.INVOCATIONRUNTIMEFAILURE {
					t.Fatalf("Factory LocalAI failure errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", run.response.ErrorCode)
				}
				if run.response.PrimaryResult != nil {
					t.Fatalf("Factory LocalAI failure primaryResult = %#v, want nil", run.response.PrimaryResult)
				}
				if strings.Contains(run.stderr, localAIFactoryInferenceFailure) {
					t.Fatalf("raw model protocol failure leaked through Factory diagnostics: %q", run.stderr)
				}
				assertLocalAIFactoryFailedDispatch(t, run.artifact.Events)
			}

			if run.response.RequestId == "" || run.response.TraceId == "" {
				t.Fatalf("Factory LocalAI response identity = %#v, want request and trace IDs", run.response)
			}
			requestID, traceID, workID, dispatchID := assertBTRCOneShotCorrelation(t, run.artifact.Events, run.response)
			assertLocalAIFactoryModelEvents(t, run.artifact.Events, requestID, traceID, workID, dispatchID, test.failure == nil, test.response)
			assertLocalAIFactoryEventSpine(t, run.artifact.Events)
			assertLocalAIFactoryTerminalSession(t, run.artifact.Events, test.wantSession)
			assertBTRCOneShotResponseStreamHasOneTerminalRecord(t, run.stdout)
			assertLocalAIFactoryConfigurationFacts(t, run, prompt, test.failure == nil, test.waitBeforeCompletion)
		})
	}
}

type localAIFactoryInferenceRun struct {
	artifact       *interfaces.ReplayArtifact
	response       factoryapi.InvocationResponse
	stdout         string
	stderr         string
	executeErr     error
	modelCacheRoot string
	source         string
	revision       string
	backend        string
	platform       modelprovider.AssetHostPlatform
	assetNetwork   *localAIFactoryAssetHTTP
	resolver       *localAIFactoryRevisionRecorder
	backendSelect  *localAIFactoryBackendSelectionRecorder
	launcher       *localAIFactoryHostLauncher
	negotiator     *localAIFactoryHostProtocolRecorder
	compatibility  *localAIFactoryHostCompatibilityRecorder
	invocation     *localAIFactoryInvocationProtocolRecorder
}

func runLocalAIFactoryInference(t *testing.T, prompt, response string, failure error, waitBeforeCompletion bool) localAIFactoryInferenceRun {
	t.Helper()

	home := t.TempDir()
	factoryDir := support.ScaffoldFactory(t, localAIFactoryInferenceFactoryConfig("http://localai-factory-characterization.invalid"))
	support.WriteWorkstationConfig(t, factoryDir, "execute-llm", "---\ntype: INFERENCE_RUN\n---\nReturn the model result.\n")
	writeLocalAIFactoryModelOverlay(t, home, localAIFactoryInferenceSource)
	writeLocalAIFactoryModelCache(t, home, localAIFactoryInferenceSource+"@"+localAIFactoryInferenceRevision)
	selection := localAIFactoryBackendSelection()
	writeLocalAIFactoryBackendCache(t, home, selection)

	assetNetwork := &localAIFactoryAssetHTTP{}
	resolver := &localAIFactoryRevisionRecorder{revision: localAIFactoryInferenceRevision}
	backendSelect := &localAIFactoryBackendSelectionRecorder{selection: selection}
	launcher := &localAIFactoryHostLauncher{
		endpoint: "http://localai-factory-characterization.invalid",
	}
	negotiator := &localAIFactoryHostProtocolRecorder{}
	compatibility := &localAIFactoryHostCompatibilityRecorder{}
	invocation := &localAIFactoryInvocationProtocolRecorder{response: response, failure: failure}
	assetFiles := localAIFactoryAssetFileSystem{home: home}
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:            assetNetwork,
		ModelAssetMakeDirectories:       assetFiles.MkdirAll,
		ModelAssetInspectPath:           assetFiles.Stat,
		ModelAssetResolveHomeDirectory:  assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:    func(string) string { return "" },
		ModelAssetWriteFile:             assetFiles.WriteFile,
		ModelAssetRenamePath:            assetFiles.Rename,
		ModelAssetRemovePath:            assetFiles.Remove,
		ModelAssetReadFile:              assetFiles.ReadFile,
		ModelAssetReadDirectory:         assetFiles.ReadDir,
		ModelAssetCreateFile:            assetFiles.Create,
		ModelAssetOpenFile:              assetFiles.Open,
		ModelHostProcessLauncher:        launcher,
		ModelHostProtocolNegotiator:     negotiator,
		ModelHostCompatibilityChecker:   compatibility,
		ModelAssetHostPlatform:          modelprovider.AssetHostPlatform{OperatingSystem: localAIFactoryInferencePlatformOS, Architecture: localAIFactoryInferencePlatformArch},
		ModelResolveBackendArtifact:     backendSelect.Resolve,
		ModelResolveHuggingFaceRevision: resolver.Resolve,
		ModelHostHTTPClient:             assetNetwork,
		ModelRuntimeHTTPClient:          assetNetwork,
		ModelInvocationProtocolClient:   invocation,
	})
	support.CleanupProcess(t, process)
	t.Cleanup(launcher.releaseWait)

	artifactPath := filepath.Join(t.TempDir(), "localai-factory-inference.replay.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		"--record", artifactPath,
		"--output", "response-stream",
		prompt,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	executeErr := process.Execute(inputs.Input)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if !waitBeforeCompletion {
		launcher.releaseWait()
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close Factory LocalAI characterization process: %v", err)
	}
	if !waitBeforeCompletion {
		waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := launcher.awaitWait(waitContext); err != nil {
			t.Fatalf("wait for Factory LocalAI host Wait completion: %v", err)
		}
	}

	return localAIFactoryInferenceRun{
		artifact: artifact, response: support.DecodeInvocationResponseJSON(t, inputs.Stdout()),
		stdout: inputs.Stdout(), stderr: inputs.Stderr(), executeErr: executeErr,
		modelCacheRoot: filepath.Join(home, ".agent-factory", "models"),
		source:         localAIFactoryInferenceSource, revision: localAIFactoryInferenceRevision,
		backend:      localAIFactoryInferenceBackend,
		platform:     modelprovider.AssetHostPlatform{OperatingSystem: localAIFactoryInferencePlatformOS, Architecture: localAIFactoryInferencePlatformArch},
		assetNetwork: assetNetwork, resolver: resolver, backendSelect: backendSelect,
		launcher: launcher, negotiator: negotiator, compatibility: compatibility,
		invocation: invocation,
	}
}

func localAIFactoryInferenceFactoryConfig(endpoint string) map[string]any {
	return map[string]any{
		"name": "localai-factory-inference-characterization",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{{
				"name": "prompt", "externalName": "prompt", "required": true,
				"bindings": []map[string]any{{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}, {"kind": "NAMED"}},
			}},
		},
		"workTypes": []map[string]any{{
			"name": "task", "handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name": "llm-cache", "type": interfaces.ResourceTypeModel, "capacity": 1,
			"model": modelprovider.BuiltInModelNameLLM, "backend": localAIFactoryInferenceBackend, "loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "llm-worker", "type": interfaces.WorkerTypeInference,
			"model": modelprovider.BuiltInModelNameLLM, "modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityLocal, "command": "llama-cpp",
			"args":      []string{"--grpc-endpoint", endpoint},
			"resources": []map[string]any{{"name": "llm-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name":    modelprovider.OperationOMNI,
				"inputs":  []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}},
				"outputs": []map[string]any{{"name": "text", "contentTypes": []string{"TEXT"}, "required": true}},
			}},
		}},
		"workstations": []map[string]any{{
			"name": "execute-llm", "type": interfaces.WorkstationTypeInference,
			"operation": modelprovider.OperationOMNI, "worker": "llm-worker",
			"body":              "Return the model result.",
			"operationBindings": []map[string]any{{"slot": "prompt", "selector": map[string]any{"type": "TEXT"}}},
			"inputs":            []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":           []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure":         []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func assertLocalAIFactoryEventSpine(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("Factory LocalAI replay has no canonical events")
	}
	for index, event := range events {
		if event.Context.Sequence != index || strings.TrimSpace(event.Id) == "" {
			t.Fatalf("Factory LocalAI event[%d] = sequence:%d id:%q, want contiguous sequence and ID", index, event.Context.Sequence, event.Id)
		}
	}
	wantOrder := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeModelRequest,
		interfaces.FactoryEventTypeModelResponse,
		interfaces.FactoryEventTypeDispatchResponse,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted,
	}
	previous := -1
	for _, eventType := range wantOrder {
		positions := make([]int, 0, 1)
		for index, event := range events {
			if event.Type == eventType {
				positions = append(positions, index)
			}
		}
		if len(positions) != 1 || positions[0] <= previous {
			t.Fatalf("Factory LocalAI event %s positions = %#v, want one after %d; types=%v", eventType, positions, previous, localAIFactoryEventTypes(events))
		}
		previous = positions[0]
	}
}

func localAIFactoryEventTypes(events []interfaces.FactoryEvent) []interfaces.FactoryEventType {
	result := make([]interfaces.FactoryEventType, len(events))
	for index, event := range events {
		result[index] = event.Type
	}
	return result
}

func assertLocalAIFactoryModelEvents(t *testing.T, events []interfaces.FactoryEvent, requestID, traceID, workID, dispatchID string, success bool, wantText string) {
	t.Helper()
	var modelRequestEvent, modelResponseEvent *interfaces.FactoryEvent
	for index := range events {
		switch events[index].Type {
		case interfaces.FactoryEventTypeModelRequest:
			if modelRequestEvent != nil {
				t.Fatalf("duplicate MODEL_REQUEST events")
			}
			modelRequestEvent = &events[index]
		case interfaces.FactoryEventTypeModelResponse:
			if modelResponseEvent != nil {
				t.Fatalf("duplicate MODEL_RESPONSE events")
			}
			modelResponseEvent = &events[index]
		}
	}
	if modelRequestEvent == nil || modelResponseEvent == nil {
		t.Fatalf("Factory LocalAI model events = request:%v response:%v, want both", modelRequestEvent != nil, modelResponseEvent != nil)
	}
	for _, event := range []*interfaces.FactoryEvent{modelRequestEvent, modelResponseEvent} {
		if event.Context.RequestID == nil || *event.Context.RequestID != requestID ||
			event.Context.DispatchID == nil || *event.Context.DispatchID != dispatchID ||
			!reflect.DeepEqual(btrcOneShotStringSlice(event.Context.TraceIDs), []string{traceID}) ||
			!reflect.DeepEqual(btrcOneShotStringSlice(event.Context.WorkIDs), []string{workID}) {
			t.Fatalf("Factory LocalAI model event correlation = %#v, want request:%q trace:%q work:%q dispatch:%q", event.Context, requestID, traceID, workID, dispatchID)
		}
	}
	var requestPayload workerexecution.ModelRequestEventPayload
	if err := modelRequestEvent.DecodePayload(&requestPayload); err != nil {
		t.Fatalf("decode Factory LocalAI MODEL_REQUEST: %v", err)
	}
	if requestPayload.ModelRequestID == "" || requestPayload.Model != modelprovider.BuiltInModelNameLLM ||
		requestPayload.Operation != modelprovider.OperationOMNI || requestPayload.Worker != "llm-worker" ||
		requestPayload.ProviderLocality != string(interfaces.ModelLocalityLocal) {
		t.Fatalf("Factory LocalAI MODEL_REQUEST payload = %#v, want llm/OMNI/local worker facts", requestPayload)
	}
	var responsePayload workerexecution.ModelResponseEventPayload
	if err := modelResponseEvent.DecodePayload(&responsePayload); err != nil {
		t.Fatalf("decode Factory LocalAI MODEL_RESPONSE: %v", err)
	}
	if responsePayload.ModelRequestID != requestPayload.ModelRequestID || responsePayload.Model != requestPayload.Model ||
		responsePayload.Operation != requestPayload.Operation || responsePayload.Worker != requestPayload.Worker {
		t.Fatalf("Factory LocalAI MODEL_RESPONSE identity = %#v, want request payload identity %#v", responsePayload, requestPayload)
	}
	if success {
		if responsePayload.Outcome != workerexecution.InferenceOutcomeSucceeded || responsePayload.OutputContent == nil || len(*responsePayload.OutputContent) != 1 {
			t.Fatalf("successful Factory LocalAI MODEL_RESPONSE = %#v, want one output part", responsePayload)
		}
		part := (*responsePayload.OutputContent)[0]
		if string(part.Type.Normalized()) != workexecutionTextType || part.Text != wantText {
			t.Fatalf("successful Factory LocalAI MODEL_RESPONSE content = %#v, want text %q", part, wantText)
		}
	} else if responsePayload.Outcome != workerexecution.InferenceOutcomeFailed || responsePayload.FailureDetail == nil || responsePayload.OutputContent != nil {
		t.Fatalf("failed Factory LocalAI MODEL_RESPONSE = %#v, want typed failure without output", responsePayload)
	}
}

const workexecutionTextType = "text"

func assertLocalAIFactoryAcceptedDispatch(t *testing.T, events []interfaces.FactoryEvent, wantText string) {
	t.Helper()
	var payload workerexecution.DispatchResponseEventPayload
	count := 0
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		count++
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode accepted Factory LocalAI DISPATCH_RESPONSE: %v", err)
		}
	}
	if count != 1 || payload.Outcome != workerexecution.OutcomeAccepted || payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("accepted Factory LocalAI dispatch = %#v, want one accepted output Work", payload)
	}
	outputWork := (*payload.OutputWork)[0]
	if outputWork.State == nil || outputWork.State.Name != "complete" || outputWork.Content == nil || len(outputWork.Content) != 1 || outputWork.Content[0].Text != wantText {
		t.Fatalf("accepted Factory LocalAI output Work = %#v, want complete text %q", outputWork, wantText)
	}
}

func assertLocalAIFactoryFailedDispatch(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	var payload workerexecution.DispatchResponseEventPayload
	count := 0
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		count++
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode failed Factory LocalAI DISPATCH_RESPONSE: %v", err)
		}
	}
	if count != 1 || payload.Outcome != workerexecution.OutcomeFailed || payload.FailureDetail == nil || payload.ProviderFailure == nil {
		t.Fatalf("failed Factory LocalAI dispatch = %#v, want terminal typed failure", payload)
	}
	if payload.FailureDetail.Reason != workerexecution.WorkFailureTypeUnknown ||
		payload.ProviderFailure.Family != workerexecution.WorkFailureFamilyTerminal ||
		payload.ProviderFailure.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("failed Factory LocalAI classification = detail:%#v provider:%#v, want terminal unknown provider-neutral metadata", payload.FailureDetail, payload.ProviderFailure)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 || (*payload.OutputWork)[0].State == nil || (*payload.OutputWork)[0].State.Name != "failed" {
		t.Fatalf("failed Factory LocalAI output Work = %#v, want one failed Work", payload.OutputWork)
	}
}

func assertLocalAIFactoryTerminalSession(t *testing.T, events []interfaces.FactoryEvent, wantStatus interfaces.FactorySessionLifecycleStatus) {
	t.Helper()
	resultCount, completedCount := 0, 0
	var resultPayload interfaces.FactorySessionResultUpdatedEventPayload
	var completedPayload interfaces.FactorySessionCompletedEventPayload
	for _, event := range events {
		switch event.Type {
		case interfaces.FactoryEventTypeSessionStarted:
			if event.Context.SessionID == nil || *event.Context.SessionID != "~default" {
				t.Fatalf("Factory LocalAI %s session ID = %#v, want ~default", event.Type, event.Context.SessionID)
			}
		case interfaces.FactoryEventTypeSessionResultUpdated:
			if event.Context.SessionID == nil || *event.Context.SessionID != "~default" {
				t.Fatalf("Factory LocalAI %s session ID = %#v, want ~default", event.Type, event.Context.SessionID)
			}
			resultCount++
			if err := event.DecodePayload(&resultPayload); err != nil {
				t.Fatalf("decode Factory LocalAI SESSION_RESULT_UPDATED: %v", err)
			}
		case interfaces.FactoryEventTypeSessionCompleted:
			if event.Context.SessionID == nil || *event.Context.SessionID != "~default" {
				t.Fatalf("Factory LocalAI %s session ID = %#v, want ~default", event.Type, event.Context.SessionID)
			}
			completedCount++
			if err := event.DecodePayload(&completedPayload); err != nil {
				t.Fatalf("decode Factory LocalAI SESSION_COMPLETED: %v", err)
			}
		}
	}
	if resultCount != 1 || completedCount != 1 || resultPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal || completedPayload.FinalStatus != wantStatus || completedPayload.ResultStatus == nil || *completedPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal {
		t.Fatalf("Factory LocalAI terminal session = result:%d/%#v completed:%d/%#v, want one FINAL and %s", resultCount, resultPayload, completedCount, completedPayload, wantStatus)
	}
}

func assertLocalAIFactoryConfigurationFacts(t *testing.T, run localAIFactoryInferenceRun, prompt string, success, waitBeforeCompletion bool) {
	t.Helper()
	if got := run.resolver.Sources(); len(got) == 0 || got[0] != run.source {
		t.Fatalf("Factory LocalAI resolved sources = %#v, want %q", got, run.source)
	}
	requests := run.backendSelect.Requests()
	if len(requests) == 0 || requests[0].Backend != run.backend || requests[0].ProtocolVersion != "localai-backend-v1" || requests[0].Platform != run.platform {
		t.Fatalf("Factory LocalAI backend selection requests = %#v, want backend/protocol/platform facts", requests)
	}
	spec, ok := run.launcher.LastSpec()
	modelCachePath := filepath.Join(run.modelCacheRoot, "LLM", run.revision)
	backendCachePath := filepath.Join(run.modelCacheRoot, "backend-artifacts")
	if !ok || !localAIFactoryArgsContainPath(spec.Args, "--cache-path", modelCachePath) || !localAIFactoryArgsContainPath(spec.Args, "--backend-cache-path", backendCachePath) {
		t.Fatalf("Factory LocalAI host start spec = %#v, want materialized model/backend cache-path propagation", spec)
	}
	negotiation := run.negotiator.Requests()
	if len(negotiation) == 0 || negotiation[0].ProtocolVersion != "localai-backend-v1" || negotiation[0].Backend != run.backend || negotiation[0].ModelName != modelprovider.BuiltInModelNameLLM || negotiation[0].Revision != run.revision || negotiation[0].Platform != run.platform {
		t.Fatalf("Factory LocalAI protocol negotiation = %#v, want propagated model facts", negotiation)
	}
	compatibility := run.compatibility.Requests()
	if len(compatibility) == 0 || compatibility[0].Backend != run.backend || compatibility[0].ModelName != modelprovider.BuiltInModelNameLLM || compatibility[0].Revision != run.revision || compatibility[0].Platform != run.platform {
		t.Fatalf("Factory LocalAI compatibility = %#v, want backend/model/revision/platform facts", compatibility)
	}
	invocation := run.invocation.Request()
	if run.invocation.Calls() != 1 || invocation.Operation != modelprovider.OperationOMNI || len(invocation.Inputs) != 1 || invocation.Inputs[0].Slot != "prompt" || invocation.Inputs[0].Content != prompt {
		t.Fatalf("Factory LocalAI invocation = calls:%d request:%#v, want one OMNI prompt", run.invocation.Calls(), invocation)
	}
	if run.assetNetwork.Calls() != 0 {
		t.Fatalf("Factory LocalAI model network calls = %d, want zero from selected cache", run.assetNetwork.Calls())
	}
	waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if waitBeforeCompletion {
		waitStarted := make(chan struct{})
		waitResult := make(chan error, 1)
		go func() {
			close(waitStarted)
			waitResult <- run.launcher.awaitWait(waitContext)
		}()
		<-waitStarted
		run.launcher.releaseWait()
		if err := <-waitResult; err != nil {
			t.Fatalf("wait for Factory LocalAI host Wait completion: %v", err)
		}
	} else if err := run.launcher.awaitWait(waitContext); err != nil {
		t.Fatalf("wait for Factory LocalAI host Wait completion: %v", err)
	}
	if run.launcher.Active() || run.launcher.Starts() != run.launcher.Stops() || run.launcher.Stops() != run.launcher.Waits() {
		t.Fatalf("Factory LocalAI host lifecycle = starts:%d stops:%d waits:%d active:%t, want fully released", run.launcher.Starts(), run.launcher.Stops(), run.launcher.Waits(), run.launcher.Active())
	}
	if !success && run.response.PrimaryResult != nil {
		t.Fatalf("failed Factory LocalAI response primary result = %#v, want nil", run.response.PrimaryResult)
	}
}

func localAIFactoryArgsContainPath(args []string, flag, want string) bool {
	for index, arg := range args {
		if arg == flag && index+1 < len(args) && strings.Contains(filepath.Clean(args[index+1]), filepath.Clean(want)) {
			return true
		}
	}
	return false
}

func writeLocalAIFactoryModelOverlay(t *testing.T, home, source string) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create Factory LocalAI operator config directory: %v", err)
	}
	data, err := json.Marshal(map[string]any{"models": map[string]any{modelprovider.BuiltInModelNameLLM: map[string]any{"source": source}}})
	if err != nil {
		t.Fatalf("marshal Factory LocalAI operator config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write Factory LocalAI operator config: %v", err)
	}
}

func writeLocalAIFactoryModelCache(t *testing.T, home, source string) {
	t.Helper()
	name := "localai-llm.gguf"
	body := []byte("factory LocalAI model fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", source, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create Factory LocalAI model cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write Factory LocalAI model cache: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "model", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	})
	if err != nil {
		t.Fatalf("marshal Factory LocalAI model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write Factory LocalAI model metadata: %v", err)
	}
}

func localAIFactoryBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Bytes:    28, SHA256: "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}
}

func writeLocalAIFactoryBackendCache(t *testing.T, home string, selection serviceedges.ModelBackendArtifactSelection) {
	t.Helper()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://" + localAIFactoryInferenceBackend + "/release://" + urlHash
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create Factory LocalAI backend cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), []byte("localai-llamacpp/linux-amd64"), 0o644); err != nil {
		t.Fatalf("write Factory LocalAI backend cache: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	})
	if err != nil {
		t.Fatalf("marshal Factory LocalAI backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write Factory LocalAI backend metadata: %v", err)
	}
}
