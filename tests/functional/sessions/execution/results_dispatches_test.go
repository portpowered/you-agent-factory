package execution_test

import (
	"testing"

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
