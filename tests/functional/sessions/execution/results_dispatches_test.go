package execution_test

import (
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestAPIResultAndResultsExposeTerminalInvocationData proves terminal invocation
// and durable session result reads expose success primary-result content plus
// typed non-success terminal statuses through the public Factory Session API.
func TestAPIResultAndResultsExposeTerminalInvocationData(t *testing.T) {
	t.Run("successfulInvocationExposesPrimaryResultOnInvocationAndWorkReads", func(t *testing.T) {
		dir := scaffoldInvocationFactory(t, nil)
		server := startInvocationServer(
			t,
			dir,
			support.NewStaticSuccessCommandRunner(terminalSuccessPrimaryResult),
			nil,
		)

		response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
		assertInvocationPrimaryResultText(t, response, terminalSuccessPrimaryResult)
		assertTerminalWorkPrimaryText(t, server.URL(), terminalSuccessPrimaryResult)
	})

	t.Run("unresolvedPrimaryResultReturnsFailedTerminalStatus", func(t *testing.T) {
		dir := scaffoldInvocationFactory(t, map[string]any{
			"invocationReturn": map[string]any{
				"policy":        "EXPLICIT",
				"workTypeName":  "summary",
				"terminalState": "complete",
			},
			"workTypes": append(simplePipelineConfig()["workTypes"].([]map[string]any), map[string]any{
				"name": "summary",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			}),
		})
		server := startInvocationServer(
			t,
			dir,
			support.NewStaticSuccessCommandRunner("task output COMPLETE"),
			nil,
		)

		response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("invocation status = %q, want FAILED", response.Status)
		}
		if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
			t.Fatalf("invocation errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", response.ErrorCode)
		}
		if response.PrimaryResult != nil {
			t.Fatalf("invocation primaryResult = %#v, want nil on unresolved output", response.PrimaryResult)
		}
	})

	t.Run("timeoutReturnsTimedOutTerminalStatus", func(t *testing.T) {
		dir := scaffoldInvocationFactory(t, nil)
		blocking := newBlockingInvocationRunner()
		server := startInvocationServer(t, dir, blocking, nil)

		timeoutMillis := int64(10)
		response := postInvocation(
			t,
			server.URL(),
			textInvocationRequest(t, "invoke this", &timeoutMillis),
		)
		if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
			t.Fatalf("invocation status = %q, want TIMED_OUT", response.Status)
		}
		if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTIMEDOUT {
			t.Fatalf("invocation errorCode = %#v, want INVOCATION_TIMED_OUT", response.ErrorCode)
		}
		if response.PrimaryResult != nil {
			t.Fatalf("invocation primaryResult = %#v, want nil on timeout", response.PrimaryResult)
		}
		waitForBlockingInvocationStart(t, blocking)
	})

	t.Run("durableResultsReadExposesFinalPrimaryResult", func(t *testing.T) {
		dir := scaffoldInlineJavaScriptFactory(t)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			UseMockWorkers:            true,
			Edges: serviceedges.Edges{
				ProviderCommandRunner: support.NewRecordingCommandRunner("unexpected live provider execution"),
			},
		})

		started := startInlineJavaScriptSync(t, server.URL(), dir)
		if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
		}
		if started.Result == nil {
			t.Fatal("sync result = nil, want FINAL primary outcome")
		}
		assertFactorySessionResultPrimaryText(t, *started.Result, terminalSuccessPrimaryResult)

		finalResult := readDurableSessionResult(t, server.URL(), started.SessionId)
		if finalResult.SessionId != started.SessionId {
			t.Fatalf("result sessionId = %q, want %q", finalResult.SessionId, started.SessionId)
		}
		assertFactorySessionResultPrimaryText(t, finalResult, terminalSuccessPrimaryResult)
	})

	functionalevidence.Covers(t, "rest/getFactorySessionResults", "rest/invokeFactorySessionBySessionId")
}

// TestAPIDispatchListAndDetailExposePublicCorrelation proves durable Factory
// Session dispatch list and detail reads expose the same public dispatch
// identifier and compatible correlation fields so customers can join summaries
// to detail without private runtime handles.
func TestAPIDispatchListAndDetailExposePublicCorrelation(t *testing.T) {
	dir := scaffoldDispatchCorrelationFactory(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewRecordingCommandRunner("unexpected live provider execution"),
		},
	})

	started := startDispatchCorrelationSync(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable Factory Session identifier")
	}

	listed := listFactorySessionDispatches(t, server.URL(), started.SessionId)
	if len(listed.Dispatches) == 0 {
		t.Fatalf("dispatch list is empty, want at least one public dispatch summary")
	}
	if listed.SessionId != started.SessionId {
		t.Fatalf("dispatch list sessionId = %q, want %q", listed.SessionId, started.SessionId)
	}

	var matched bool
	for _, summary := range listed.Dispatches {
		if summary.Label == nil || *summary.Label != dispatchCorrelationChildLabel {
			continue
		}
		if summary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch summary status = %q, want COMPLETED", summary.Status)
		}
		detail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertDispatchListDetailPublicCorrelation(t, started.SessionId, summary, detail)
		matched = true
		break
	}
	if !matched {
		t.Fatalf(
			"dispatch list = %#v, want one completed dispatch labeled %q",
			listed.Dispatches,
			dispatchCorrelationChildLabel,
		)
	}

	functionalevidence.Covers(t, "rest/getFactorySessionDispatch", "rest/listFactorySessionDispatches")
}

// TestAPIPartialResultIsAvailableBeforeTerminalCompletion proves a durable
// Factory Session exposes an observable partial-result projection through the
// public results read surface while the session is still non-terminal and
// before final completion is available.
func TestAPIPartialResultIsAvailableBeforeTerminalCompletion(t *testing.T) {
	dir := scaffoldPartialResultFactory(t)
	provider := newPartialResultBlockingProvider(partialResultWorkflowName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderOverride: provider},
	})

	started := startPartialResultAsync(t, server.URL())
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable Factory Session identifier")
	}
	t.Cleanup(func() {
		terminateDurableSession(t, server.URL(), started.SessionId)
	})

	waitForDurableSessionStatus(
		t,
		server.URL(),
		started.SessionId,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		5*time.Second,
	)
	waitForFactoryDispatchStatus(
		t,
		server.URL(),
		started.SessionId,
		partialResultFirstDispatchID,
		factoryapi.FactoryDispatchStatusCOMPLETED,
		5*time.Second,
	)
	waitForFactoryDispatchStatus(
		t,
		server.URL(),
		started.SessionId,
		partialResultSecondDispatchID,
		factoryapi.FactoryDispatchStatusRUNNING,
		5*time.Second,
	)

	partial := waitForDurablePartialResult(t, server.URL(), started.SessionId, 5*time.Second)
	if partial.SessionId != started.SessionId {
		t.Fatalf("partial result sessionId = %q, want %q", partial.SessionId, started.SessionId)
	}
	if partial.SessionStatus == nil || *partial.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("partial result sessionStatus = %#v, want RUNNING", partial.SessionStatus)
	}
	if partial.Mode == nil || *partial.Mode != factoryapi.FactorySessionResultModePartial {
		t.Fatalf("partial result mode = %#v, want partial", partial.Mode)
	}
	assertPartialResultObservableBeforeTerminal(t, partial)

	session := readDurableSession(t, server.URL(), started.SessionId)
	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING before terminal completion", session.Status)
	}
	if session.ResultSummary != nil &&
		session.ResultSummary.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want no FINAL summary before terminal completion", session.ResultSummary)
	}
	if session.LatestCheckpoint == nil || session.LatestCheckpoint.Label == nil ||
		*session.LatestCheckpoint.Label != partialResultCheckpointLabel {
		t.Fatalf("latestCheckpoint = %#v, want label %q", session.LatestCheckpoint, partialResultCheckpointLabel)
	}

	final := readDurableSessionResultWithMode(t, server.URL(), started.SessionId, "final")
	if final.ResultStatus != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("final-mode resultStatus = %q, want NOT_READY while session is running", final.ResultStatus)
	}
	if final.Availability == nil || final.Availability.Reason == nil ||
		*final.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("final-mode availability = %#v, want RESULT_NOT_READY", final.Availability)
	}
}
