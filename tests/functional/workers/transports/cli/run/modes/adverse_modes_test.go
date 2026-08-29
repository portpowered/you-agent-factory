package modes_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIRunPartialResponseStreamHasOneFailedTerminal(t *testing.T) {
	result := modesFixture(t).execute(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "partial output must not be promoted to success",
		includePrompt: true,
		behavior:      modesRoutePartial,
	})
	if result.err == nil {
		t.Fatal("Process.Execute() error = nil, want provider failure")
	}
	if result.providerCalls < 1 {
		t.Fatalf("provider calls = %d, want at least one dispatch", result.providerCalls)
	}
	terminal := decodeTerminalNDJSONInvocationResult(t, result.stdout)
	assertFailedInvocationResponse(t, terminal.Response)
	if terminal.Response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("terminal status = %q, want FAILED", terminal.Response.Status)
	}
	if strings.Contains(result.stdout, wantPrimaryResult) || invocationPrimaryResultPresent(terminal.Response) {
		t.Fatalf("partial stream produced a successful primary result: stdout=%q response=%#v", result.stdout, terminal.Response)
	}
	assertFailedRunErrorResponse(t, result.stderr, terminal.Response)
}

func TestCLIRunEventPresentationsCorrelateWorkDispatchWorkerAndPrimary(t *testing.T) {
	fixture := modesFixture(t)
	for _, runArgs := range [][]string{
		nil,
		{"--output", "response-stream"},
	} {
		result := fixture.execute(t, modesInvocationSpec{
			globalArgs:    []string{"--json"},
			runArgs:       runArgs,
			prompt:        "correlate work dispatch worker and primary result",
			includePrompt: true,
			behavior:      modesRouteSuccess,
			result:        "correlated primary COMPLETE",
		})
		assertMachineSuccess(t, result, "correlated primary COMPLETE")
		assertModesEventCorrelation(t, result)
	}
}

type modesFactoryEventRecord struct {
	RecordType string                  `json:"recordType"`
	Event      factoryapi.FactoryEvent `json:"event"`
}

func decodeModesFactoryEvents(t *testing.T, stdout string) []factoryapi.FactoryEvent {
	t.Helper()
	var events []factoryapi.FactoryEvent
	for index, line := range nonEmptyLines(stdout) {
		var envelope struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode JSON output record %d: %v\nline: %s", index, err, line)
		}
		if envelope.RecordType != "factory_event" {
			continue
		}
		var record modesFactoryEventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode Factory Event record %d: %v\nline: %s", index, err, line)
		}
		events = append(events, record.Event)
	}
	if len(events) == 0 {
		t.Fatalf("JSON output contains no Factory Event records\nstdout:\n%s", stdout)
	}
	return events
}

type modesCorrelationEvents struct {
	workRequest       *factoryapi.FactoryEvent
	dispatchRequest   *factoryapi.FactoryEvent
	workerAssociation *factoryapi.FactoryEvent
	dispatchResponse  *factoryapi.FactoryEvent
}

func assertModesEventCorrelation(t *testing.T, result modesInvocationResult) {
	t.Helper()
	correlation := collectModesCorrelationEvents(t, decodeModesFactoryEvents(t, result.stdout))
	workID, requestID := assertModesWorkCorrelation(t, correlation.workRequest)
	dispatchID := assertModesDispatchCorrelation(t, correlation.dispatchRequest, workID)
	assertModesWorkerCorrelation(t, correlation.workerAssociation, dispatchID)
	assertModesResponseCorrelation(t, correlation.dispatchResponse, dispatchID, workID)

	terminal := decodeTerminalNDJSONInvocationResult(t, result.stdout).Response
	if terminal.RequestId != requestID {
		t.Fatalf("InvocationResponse request ID = %q, want %q", terminal.RequestId, requestID)
	}
	assertInvocationPrimaryResultText(t, terminal, "correlated primary COMPLETE")
}

func collectModesCorrelationEvents(t *testing.T, events []factoryapi.FactoryEvent) modesCorrelationEvents {
	t.Helper()
	var workRequest, dispatchRequest, workerAssociation, dispatchResponse *factoryapi.FactoryEvent
	for index := range events {
		event := &events[index]
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if workRequest != nil {
				t.Fatalf("Factory Event stream contains duplicate WORK_REQUEST events")
			}
			workRequest = event
		case factoryapi.FactoryEventTypeDispatchRequest:
			if dispatchRequest != nil {
				t.Fatalf("Factory Event stream contains duplicate DISPATCH_REQUEST events")
			}
			dispatchRequest = event
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			if workerAssociation != nil {
				t.Fatalf("Factory Event stream contains duplicate worker-session association events")
			}
			workerAssociation = event
		case factoryapi.FactoryEventTypeDispatchResponse:
			if dispatchResponse != nil {
				t.Fatalf("Factory Event stream contains duplicate DISPATCH_RESPONSE events")
			}
			dispatchResponse = event
		}
	}
	if workRequest == nil || dispatchRequest == nil || workerAssociation == nil || dispatchResponse == nil {
		t.Fatalf("Factory Event stream missing correlation events: work_request=%t dispatch_request=%t worker_session=%t dispatch_response=%t", workRequest != nil, dispatchRequest != nil, workerAssociation != nil, dispatchResponse != nil)
	}
	return modesCorrelationEvents{
		workRequest: workRequest, dispatchRequest: dispatchRequest,
		workerAssociation: workerAssociation, dispatchResponse: dispatchResponse,
	}
}

func assertModesWorkCorrelation(t *testing.T, workRequest *factoryapi.FactoryEvent) (string, string) {
	t.Helper()
	workPayload, err := workRequest.Payload.AsWorkRequestEventPayload()
	if err != nil || workPayload.Works == nil || len(*workPayload.Works) != 1 || (*workPayload.Works)[0].WorkId == nil {
		t.Fatalf("WORK_REQUEST payload = %#v, decode error = %v; want one Work ID", workPayload, err)
	}
	workID := *(*workPayload.Works)[0].WorkId
	if workID == "" {
		t.Fatal("WORK_REQUEST Work ID is empty")
	}
	return workID, requiredModesContextID(t, "WORK_REQUEST", workRequest.Context.RequestId)
}

func assertModesDispatchCorrelation(t *testing.T, dispatchRequest *factoryapi.FactoryEvent, workID string) string {
	t.Helper()
	dispatchID := requiredModesContextID(t, "DISPATCH_REQUEST", dispatchRequest.Context.DispatchId)
	if dispatchRequest.Context.WorkIds == nil || !containsModesID(*dispatchRequest.Context.WorkIds, workID) {
		t.Fatalf("DISPATCH_REQUEST context work IDs = %#v, want %q", dispatchRequest.Context.WorkIds, workID)
	}
	dispatchPayload, err := dispatchRequest.Payload.AsDispatchRequestEventPayload()
	if err != nil || len(dispatchPayload.Inputs) != 1 || dispatchPayload.Inputs[0].WorkId != workID {
		t.Fatalf("DISPATCH_REQUEST payload = %#v, decode error = %v; want Work ID %q", dispatchPayload, err, workID)
	}
	return dispatchID
}

func assertModesWorkerCorrelation(t *testing.T, workerAssociation *factoryapi.FactoryEvent, dispatchID string) {
	t.Helper()
	workerPayload, err := workerAssociation.Payload.AsDispatchWorkerSessionAssociationEventPayload()
	if err != nil || workerPayload.WorkerSessionId == "" {
		t.Fatalf("worker-session association payload = %#v, decode error = %v; want non-empty Worker Session ID", workerPayload, err)
	}
	associationDispatchID := requiredModesContextID(t, "worker-session association", workerAssociation.Context.DispatchId)
	if associationDispatchID != dispatchID {
		t.Fatalf("worker-session association dispatch ID = %q, want %q", associationDispatchID, dispatchID)
	}
}

func assertModesResponseCorrelation(t *testing.T, dispatchResponse *factoryapi.FactoryEvent, dispatchID, workID string) {
	t.Helper()
	responseDispatchID := requiredModesContextID(t, "DISPATCH_RESPONSE", dispatchResponse.Context.DispatchId)
	if responseDispatchID != dispatchID {
		t.Fatalf("DISPATCH_RESPONSE dispatch ID = %q, want %q", responseDispatchID, dispatchID)
	}
	if dispatchResponse.Context.WorkIds == nil || !containsModesID(*dispatchResponse.Context.WorkIds, workID) {
		t.Fatalf("DISPATCH_RESPONSE context work IDs = %#v, want %q", dispatchResponse.Context.WorkIds, workID)
	}
	responsePayload, err := dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil || responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 || (*responsePayload.OutputWork)[0].WorkId == nil || *(*responsePayload.OutputWork)[0].WorkId != workID {
		t.Fatalf("DISPATCH_RESPONSE payload = %#v, decode error = %v; want terminal Work ID %q", responsePayload, err, workID)
	}
	if (*responsePayload.OutputWork)[0].State == nil || string((*responsePayload.OutputWork)[0].State.Type) != "TERMINAL" {
		t.Fatalf("DISPATCH_RESPONSE output Work state = %#v, want TERMINAL", (*responsePayload.OutputWork)[0].State)
	}
}

func requiredModesContextID(t *testing.T, name string, value *string) string {
	t.Helper()
	if value == nil || *value == "" {
		t.Fatalf("%s context identity = %#v, want non-empty value", name, value)
	}
	return *value
}

func containsModesID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCLIRunTimeoutRecoversOnSameProcess(t *testing.T) {
	fixture := modesFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	handle := fixture.start(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "provider timeout must be terminal and recoverable",
		includePrompt: true,
		behavior:      modesRouteBlock,
		context:       ctx,
	})
	handle.route.WaitStarted(t)
	timeoutResult := handle.wait(t)
	if timeoutResult.err == nil || !strings.Contains(timeoutResult.err.Error(), "INVOCATION_TIMED_OUT") {
		t.Fatalf("timeout error = %v, want INVOCATION_TIMED_OUT", timeoutResult.err)
	}
	terminal := decodeTerminalNDJSONInvocationResult(t, timeoutResult.stdout)
	assertInvocationOutcome(t, terminal.Response, "TIMED_OUT", "INVOCATION_TIMED_OUT")
	if invocationPrimaryResultPresent(terminal.Response) {
		t.Fatalf("timeout response contains primary result: %#v", terminal.Response.PrimaryResult)
	}
	if timeoutResult.providerCalls != 1 || handle.route.active.Load() != 0 {
		t.Fatalf("timeout route calls=%d active=%d, want one call and no active call", timeoutResult.providerCalls, handle.route.active.Load())
	}

	recovery := fixture.execute(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "timeout recovery succeeds with fresh state",
		includePrompt: true,
		behavior:      modesRouteSuccess,
		result:        "timeout recovery COMPLETE",
	})
	assertMachineSuccess(t, recovery, "timeout recovery COMPLETE")
	assertFreshInvocation(t, timeoutResult, recovery)
}

func TestCLIRunCancellationRecoversOnSameProcess(t *testing.T) {
	fixture := modesFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	handle := fixture.start(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "provider cancellation must be terminal and recoverable",
		includePrompt: true,
		behavior:      modesRouteBlock,
		context:       ctx,
	})
	handle.route.WaitStarted(t)
	cancel()
	canceled := handle.wait(t)
	if canceled.err == nil || !strings.Contains(canceled.err.Error(), "INVOCATION_CANCELED") {
		t.Fatalf("cancellation error = %v, want INVOCATION_CANCELED", canceled.err)
	}
	terminal := decodeTerminalNDJSONInvocationResult(t, canceled.stdout)
	assertInvocationOutcome(t, terminal.Response, "CANCELED", "INVOCATION_CANCELED")
	if invocationPrimaryResultPresent(terminal.Response) {
		t.Fatalf("canceled response contains primary result: %#v", terminal.Response.PrimaryResult)
	}
	if canceled.providerCalls != 1 || handle.route.active.Load() != 0 {
		t.Fatalf("canceled route calls=%d active=%d, want one call and no active call", canceled.providerCalls, handle.route.active.Load())
	}

	recovery := fixture.execute(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "cancellation recovery succeeds with fresh state",
		includePrompt: true,
		behavior:      modesRouteSuccess,
		result:        "cancellation recovery COMPLETE",
	})
	assertMachineSuccess(t, recovery, "cancellation recovery COMPLETE")
	assertFreshInvocation(t, canceled, recovery)
}

// This witness records the current public-surface blocker; it is not the MODE-U10 acceptance witness.
func TestCLIRunConcurrentSessionsExposeMissingFactorySessionAuthority(t *testing.T) {
	fixture := modesFixture(t)
	first := fixture.start(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "first keyed session holds the provider route",
		includePrompt: true,
		behavior:      modesRouteBlock,
		result:        "first keyed session COMPLETE",
	})
	first.route.WaitStarted(t)

	second := fixture.start(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "response-stream"},
		prompt:        "second keyed session must not share the default runtime",
		includePrompt: true,
		behavior:      modesRouteBlock,
		result:        "second keyed session COMPLETE",
	})
	secondResult := second.wait(t)
	if secondResult.err == nil ||
		!strings.Contains(secondResult.err.Error(), "Factory Definitions runtime is already bound: ~default") {
		t.Fatalf("second concurrent invocation error = %v, want the stable default-runtime binding blocker", secondResult.err)
	}
	if secondResult.providerCalls != 0 || strings.TrimSpace(secondResult.stdout) != "" {
		t.Fatalf("second concurrent invocation calls=%d stdout=%q, want no provider dispatch or success output", secondResult.providerCalls, secondResult.stdout)
	}

	first.cancel()
	firstResult := first.wait(t)
	if firstResult.err == nil || !strings.Contains(firstResult.err.Error(), "INVOCATION_CANCELED") {
		t.Fatalf("first concurrent invocation error = %v, want cancellation while the second invocation is rejected", firstResult.err)
	}
	firstTerminal := decodeTerminalNDJSONInvocationResult(t, firstResult.stdout).Response
	assertInvocationOutcome(t, firstTerminal, "CANCELED", "INVOCATION_CANCELED")
	if firstResult.resources.id == secondResult.resources.id ||
		firstResult.resources.workingRoot == secondResult.resources.workingRoot {
		t.Fatalf("concurrent invocation resources were not distinct: first=%#v second=%#v", firstResult.resources, secondResult.resources)
	}
	if pathExists(firstResult.resources.workingRoot) || pathExists(secondResult.resources.workingRoot) {
		t.Fatalf("concurrent invocation roots remain: first=%q second=%q", firstResult.resources.workingRoot, secondResult.resources.workingRoot)
	}
}

func TestCLIRunEmptyPrimaryAndSelectorRecovery(t *testing.T) {
	fixture := modesFixture(t)
	prior := fixture.execute(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "primary"},
		prompt:        "prior primary must not leak into an empty result",
		includePrompt: true,
		behavior:      modesRouteSuccess,
		result:        "prior primary COMPLETE",
	})
	assertJSONPrimarySuccess(t, prior, "prior primary COMPLETE")

	emptyPrompt := "successful empty primary result"
	emptyJSON := fixture.execute(t, modesInvocationSpec{
		globalArgs:    []string{"--json"},
		runArgs:       []string{"--output", "primary"},
		prompt:        emptyPrompt,
		includePrompt: true,
		behavior:      modesRouteSuccess,
		emptyResult:   true,
	})
	if emptyJSON.err != nil {
		t.Fatalf("empty JSON invocation error = %v\nstdout=%s\nstderr=%s", emptyJSON.err, emptyJSON.stdout, emptyJSON.stderr)
	}
	response := decodeInvocationResponse(t, emptyJSON.stdout)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("empty primary status = %q, want COMPLETED", response.Status)
	}
	assertInvocationPrimaryResultText(t, response, emptyPrompt)
	if strings.Contains(emptyJSON.stdout, "prior primary COMPLETE") {
		t.Fatalf("empty primary reused prior content: %q", emptyJSON.stdout)
	}
	if emptyJSON.stderr != "" || emptyJSON.providerCalls != 1 {
		t.Fatalf("empty JSON stderr=%q calls=%d, want empty stderr and one dispatch", emptyJSON.stderr, emptyJSON.providerCalls)
	}

	rawPrompt := "successful empty raw primary result"
	emptyRaw := fixture.execute(t, modesInvocationSpec{
		runArgs:       []string{"--output", "primary"},
		prompt:        rawPrompt,
		includePrompt: true,
		behavior:      modesRouteSuccess,
		emptyResult:   true,
	})
	if emptyRaw.err != nil || emptyRaw.stdout != rawPrompt || emptyRaw.stderr != "" || emptyRaw.providerCalls != 1 {
		t.Fatalf("empty raw result = err=%v stdout=%q stderr=%q calls=%d, want submitted prompt, no diagnostics, one dispatch", emptyRaw.err, emptyRaw.stdout, emptyRaw.stderr, emptyRaw.providerCalls)
	}

	selected := fixture.execute(t, modesInvocationSpec{
		runArgs:       []string{"--output", "primary"},
		prompt:        "explicit selector before default recovery",
		includePrompt: true,
		behavior:      modesRouteSuccess,
	})
	if strings.TrimSpace(selected.stdout) != wantPrimaryResult {
		t.Fatalf("selected primary stdout = %q, want %q", selected.stdout, wantPrimaryResult)
	}
	defaultMode := fixture.execute(t, modesInvocationSpec{
		prompt:        "default selector after explicit recovery",
		includePrompt: true,
		behavior:      modesRouteSuccess,
	})
	assertHumanResponseStream(t, defaultMode.stdout)
	if defaultMode.stderr == "" {
		t.Fatal("default output stderr is empty, want human progress after selector reset")
	}
}

func TestCLIRunStdinBoundaryUsesInclusiveWorkPayloadLimit(t *testing.T) {
	fixture := modesFixture(t)
	const maxWorkPayloadBytes = 64 << 10
	const jsonStringOverhead = 2
	acceptedInputBytes := maxWorkPayloadBytes - jsonStringOverhead
	accepted := fixture.execute(t, modesInvocationSpec{
		globalArgs: []string{"--json"},
		runArgs:    []string{"--output", "primary", "-"},
		stdin:      strings.Repeat("x", acceptedInputBytes),
		behavior:   modesRouteSuccess,
		result:     "maximum accepted Work payload COMPLETE",
	})
	assertJSONPrimarySuccess(t, accepted, "maximum accepted Work payload COMPLETE")
	if len(accepted.requests) != 1 {
		t.Fatalf("accepted stdin provider requests = %d, want one dispatch", len(accepted.requests))
	}

	over := fixture.execute(t, modesInvocationSpec{
		globalArgs: []string{"--json"},
		runArgs:    []string{"--output", "primary", "-"},
		stdin:      strings.Repeat("x", acceptedInputBytes+1),
		behavior:   modesRouteSuccess,
	})
	if over.err == nil || !strings.Contains(over.err.Error(), "payload exceeds byte limit") || !strings.Contains(over.err.Error(), "payloadLimitBytes=65536") {
		t.Fatalf("over-limit stdin error = %v, want the stable Work payload-limit diagnostic", over.err)
	}
	if !strings.Contains(over.err.Error(), "payloadBytes=65537") {
		t.Fatalf("over-limit stdin error = %v, want the 65,537-byte payload measurement", over.err)
	}
	if over.stdout != "" || over.providerCalls != 0 {
		t.Fatalf("over-limit stdin stdout=%q provider_calls=%d, want empty output and no dispatch", over.stdout, over.providerCalls)
	}
	if !strings.Contains(over.stderr, "payload exceeds byte limit") {
		t.Fatalf("over-limit stdin stderr = %q, want Work payload-limit diagnostic", over.stderr)
	}

	exactMiB := fixture.execute(t, modesInvocationSpec{
		globalArgs:     []string{"--json"},
		runArgs:        []string{"--output", "primary", "-"},
		stdin:          strings.Repeat(" ", (1<<20)-4) + "true",
		stdinSignature: true,
		behavior:       modesRouteSuccess,
		result:         "exact 1 MiB stdin COMPLETE",
	})
	assertJSONPrimarySuccess(t, exactMiB, "exact 1 MiB stdin COMPLETE")
	if len(exactMiB.requests) != 1 {
		t.Fatalf("exact 1 MiB stdin provider requests = %d, want one dispatch", len(exactMiB.requests))
	}

	overMiB := fixture.execute(t, modesInvocationSpec{
		globalArgs:     []string{"--json"},
		runArgs:        []string{"--output", "primary", "-"},
		stdin:          strings.Repeat(" ", 1<<20) + "x",
		stdinSignature: true,
		behavior:       modesRouteSuccess,
	})
	if overMiB.err == nil || !strings.Contains(overMiB.err.Error(), "invocation stdin exceeds the 1048576-byte limit") {
		t.Fatalf("1 MiB plus one stdin error = %v, want stable CLI collector limit diagnostic", overMiB.err)
	}
	if overMiB.stdout != "" || overMiB.providerCalls != 0 || !strings.Contains(overMiB.stderr, "invocation stdin exceeds the 1048576-byte limit") {
		t.Fatalf("1 MiB plus one stdin stdout=%q stderr=%q provider_calls=%d, want no output and no dispatch", overMiB.stdout, overMiB.stderr, overMiB.providerCalls)
	}
}

func assertMachineSuccess(t *testing.T, result modesInvocationResult, want string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("machine invocation error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertSingleRequestForRoot(t, result)
	if result.providerCalls != 1 || result.stderr != "" {
		t.Fatalf("machine success calls=%d stderr=%q, want one call and empty stderr", result.providerCalls, result.stderr)
	}
	response := decodeTerminalNDJSONInvocationResult(t, result.stdout).Response
	assertInvocationPrimaryResultText(t, response, want)
}

func assertJSONPrimarySuccess(t *testing.T, result modesInvocationResult, want string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("JSON primary invocation error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertSingleRequestForRoot(t, result)
	if result.providerCalls != 1 || result.stderr != "" {
		t.Fatalf("JSON primary success calls=%d stderr=%q, want one call and empty stderr", result.providerCalls, result.stderr)
	}
	assertInvocationPrimaryResultText(t, decodeInvocationResponse(t, result.stdout), want)
}

func assertInvocationOutcome(t *testing.T, response factoryapi.InvocationResponse, status, code string) {
	t.Helper()
	if string(response.Status) != status || response.ErrorCode == nil || string(*response.ErrorCode) != code {
		t.Fatalf("InvocationResponse = %#v, want status %q and error code %q", response, status, code)
	}
}

func assertFreshInvocation(t *testing.T, prior, recovery modesInvocationResult) {
	t.Helper()
	if prior.resources.id == recovery.resources.id || prior.resources.workingRoot == recovery.resources.workingRoot {
		t.Fatalf("recovery reused invocation identity: prior=%#v recovery=%#v", prior.resources, recovery.resources)
	}
	if pathExists(prior.resources.workingRoot) || pathExists(recovery.resources.workingRoot) {
		t.Fatalf("invocation roots remain after completion: prior=%q recovery=%q", prior.resources.workingRoot, recovery.resources.workingRoot)
	}
}

func assertSingleRequestForRoot(t *testing.T, result modesInvocationResult) {
	t.Helper()
	if len(result.requests) != 1 {
		t.Fatalf("provider requests for %s = %d, want one", result.resources.id, len(result.requests))
	}
	if result.requests[0].WorkDir != result.resources.workingRoot {
		t.Fatalf("provider work directory = %q, want invocation root %q", result.requests[0].WorkDir, result.resources.workingRoot)
	}
}
