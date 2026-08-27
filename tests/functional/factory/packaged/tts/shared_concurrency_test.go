package tts

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedTTSConcurrentInvocationResult struct {
	name     string
	response factoryapi.InvocationResponse
	err      error
}

func runPackagedTTSSharedConcurrentIsolation(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) {
	t.Helper()
	const (
		successRequestID = "tts-shared-concurrent-success"
		failureRequestID = "tts-shared-concurrent-failure"
		successText      = "functional shared concurrent packaged tts success"
		failureText      = "functional shared concurrent packaged tts failure"
		voice            = "alloy"
		format           = "mp3"
		failureMessage   = "omnivoice concurrent invoke failed: exit status 1"
	)
	success := fixture.openPackagedScenario(t, successRequestID, true)
	failure := fixture.openPackagedFailureScenario(t, failureRequestID, failureMessage, true)

	fixture.commandRunner.setInferenceBarrier(newPackagedTTSInferenceBarrier(2))
	defer fixture.commandRunner.setInferenceBarrier(nil)
	results := startPackagedTTSConcurrentInvocations(
		t, fixture, success, failure, successRequestID, failureRequestID,
		successText, failureText, voice, format,
	)
	successResult, failureResult := collectPackagedTTSConcurrentResults(t, results)
	assertPackagedTTSConcurrentResponses(t, success, failure, successResult, failureResult)
	assertPackagedTTSConcurrentCommandEvidence(t, success, failure)
	assertPackagedTTSConcurrentPublicEvidence(
		t, fixture, success, failure, successResult, failureResult,
		successText, failureText, voice, format, failureMessage,
	)
	success.cleanup(t)
	failure.cleanup(t)
}

func startPackagedTTSConcurrentInvocations(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
	success, failure *packagedTTSSharedScenario,
	successRequestID, failureRequestID, successText, failureText, voice, format string,
) <-chan packagedTTSConcurrentInvocationResult {
	t.Helper()
	results := make(chan packagedTTSConcurrentInvocationResult, 2)
	go func() {
		response, err := postPackagedTTSInvocationWithArgsContext(
			t.Context(), fixture.baseURL, success.sessionID, successRequestID,
			map[string]any{"text": successText, "voice": voice, "format": format},
		)
		results <- packagedTTSConcurrentInvocationResult{name: "success", response: response, err: err}
	}()
	go func() {
		response, err := postPackagedTTSInvocationWithArgsContext(
			t.Context(), fixture.baseURL, failure.sessionID, failureRequestID,
			map[string]any{"text": failureText, "voice": voice, "format": format},
		)
		results <- packagedTTSConcurrentInvocationResult{name: "failure", response: response, err: err}
	}()
	return results
}

func collectPackagedTTSConcurrentResults(
	t testing.TB,
	results <-chan packagedTTSConcurrentInvocationResult,
) (packagedTTSConcurrentInvocationResult, packagedTTSConcurrentInvocationResult) {
	t.Helper()
	var success, failure packagedTTSConcurrentInvocationResult
	for range 2 {
		result := <-results
		switch result.name {
		case "success":
			success = result
		case "failure":
			failure = result
		default:
			t.Fatalf("concurrent TTS result has unknown name %q", result.name)
		}
	}
	return success, failure
}

func assertPackagedTTSConcurrentResponses(
	t *testing.T,
	success, failure *packagedTTSSharedScenario,
	successResult, failureResult packagedTTSConcurrentInvocationResult,
) {
	t.Helper()
	if successResult.err != nil {
		t.Fatalf("concurrent success invocation error = %v", successResult.err)
	}
	if failureResult.err != nil {
		t.Fatalf("concurrent failure invocation error = %v", failureResult.err)
	}
	if successResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("concurrent success response = %#v, want COMPLETED", successResult.response)
	}
	if failureResult.response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("concurrent failure response = %#v, want FAILED", failureResult.response)
	}
	if failureResult.response.ErrorCode == nil || *failureResult.response.ErrorCode != factoryapi.INVOCATIONTTSGENERATIONFAILED {
		t.Fatalf("concurrent failure errorCode = %#v, want INVOCATION_TTS_GENERATION_FAILED", failureResult.response.ErrorCode)
	}
	assertPackagedTTSInvocationResponseIdentityForSession(t, successResult.response, success.sessionID, success.selector)
	assertPackagedTTSInvocationResponseIdentityForSession(t, failureResult.response, failure.sessionID, failure.selector)
	if primaryResultContainsTTSArtifactMetadata(t, failureResult.response.PrimaryResult) {
		t.Fatalf("concurrent failure primary result = %#v, want no success-shaped artifact metadata", failureResult.response.PrimaryResult)
	}
}

func assertPackagedTTSConcurrentCommandEvidence(
	t *testing.T,
	success, failure *packagedTTSSharedScenario,
) {
	t.Helper()
	successRequest := success.outcome.lastRequest()
	failureRequest := failure.outcome.lastRequest()
	assertPackagedTTSCommandRequest(t, successRequest, success.factoryDir)
	assertPackagedTTSCommandRequest(t, failureRequest, failure.factoryDir)
	if success.outcome.callCount() != 1 || failure.outcome.callCount() != 1 {
		t.Fatalf("concurrent command calls = success:%d failure:%d, want one each", success.outcome.callCount(), failure.outcome.callCount())
	}
}

func assertPackagedTTSConcurrentPublicEvidence(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
	success, failure *packagedTTSSharedScenario,
	successResult, failureResult packagedTTSConcurrentInvocationResult,
	successText, failureText, voice, format, failureMessage string,
) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, success.sessionID, packagedTTSSharedFixtureTimeout)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, failure.sessionID, packagedTTSSharedFixtureTimeout)
	successArtifactPath := success.outcome.lastAudioPath()
	assertPackagedTTSAudioBytes(t, successArtifactPath)
	assertPackagedTTSNoArtifact(t, failure.outcome)
	successListed := listPackagedTTSSessionWork(t, fixture.baseURL, success.sessionID)
	failureListed := listPackagedTTSSessionWork(t, fixture.baseURL, failure.sessionID)
	successWork := packagedTTSCompletedMetadataWork(t, successListed, successArtifactPath, successResult.response.TraceId)
	failureWork := factoryTTSFailedWork(t, failureListed)
	successAudio := packagedTTSExpectedAudioPart(t, successArtifactPath)
	successEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, success.sessionID)
	failureEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, failure.sessionID)
	successObserved := collectFactoryTTSDispatchEvents(t, successEvents, success.sessionID)
	failureObserved := collectFactoryTTSDispatchEvents(t, failureEvents, failure.sessionID)
	assertPackagedTTSResolvedBindings(t, successObserved.modelResponse, voice, format)
	assertPackagedTTSResolvedBindings(t, failureObserved.modelResponse, voice, format)
	assertPackagedTTSSuccessEventsForSession(t, successEvents, success.sessionID, successWork, successText, successAudio, successArtifactPath, successResult.response.TraceId)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, successResult.response, successEvents, success.sessionID)
	failureWorkID := *failureWork.WorkId
	failureTraceID := factoryTTSRequiredTraceID(t, failureObserved.workRequest)
	failureDispatchID := factoryTTSRequiredContextID(t, failureObserved.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, failureObserved, failureWorkID, failure.selector, failureTraceID, failureDispatchID)
	assertPackagedTTSWorkRequest(t, failureObserved.workRequest, failureWorkID, failureText)
	assertPackagedTTSDispatchRequest(t, failureObserved.dispatchRequest, failureWorkID)
	assertFactoryTTSFailureModelEvents(t, failureObserved, failureMessage)
	assertPackagedTTSFailureDispatchResponse(t, failureObserved.dispatchResponse, failureWorkID, failureText, failureMessage)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, failureResult.response, failureEvents, failure.sessionID)
	assertPackagedTTSNoArtifactEvents(t, failureEvents)
	successEvidence := sharedTTSSharedEvidence(t, "concurrent_success", success, success.selector, successWork, successEvents, successArtifactPath)
	failureEvidence := sharedTTSSharedEvidence(t, "concurrent_failure", failure, failure.selector, failureWork, failureEvents, "")
	assertPackagedTTSConcurrentEvidenceDisjoint(t, successEvidence, failureEvidence)
}
