package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestMetricsSessionSelectedServerFailuresFailClosed(t *testing.T) {
	cases := []struct {
		name       string
		sessionID  string
		status     int
		code       factoryapi.ErrorResponseCode
		family     factoryapi.ErrorFamily
		message    string
		handlerMsg string
	}{
		{
			name: "unknown session", sessionID: "missing-session", status: http.StatusNotFound,
			code: factoryapi.ErrorResponseCode("METRICS_SESSION_NOT_FOUND"), family: factoryapi.ErrorFamilyNotFound,
			message: "Factory Session missing-session was not found; use `you session list --scope live`",
		},
		{
			name: "scope unavailable", sessionID: "known-session", status: http.StatusServiceUnavailable,
			code: factoryapi.ErrorResponseCode("METRICS_SESSION_SCOPE_UNAVAILABLE"), family: factoryapi.ErrorFamilyInternalServerError,
			message: "Factory Session known-session has no retained metrics scope; use `you session list --scope live`",
		},
		{
			name: "selected server failure", sessionID: "server-failure-session", status: http.StatusInternalServerError,
			code: factoryapi.ErrorResponseCode("METRICS_QUERY_FAILED"), family: factoryapi.ErrorFamilyInternalServerError,
			handlerMsg: "the selected Factory API is unavailable",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/metrics" {
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				if request.URL.Query().Get("session_id") != test.sessionID {
					writeBoundaryError(writer, http.StatusBadRequest, factoryapi.ErrorResponseCode("WRONG_SCOPE"), factoryapi.ErrorFamilyBadRequest, "wrong session scope")
					return
				}
				if test.status == http.StatusInternalServerError {
					writeBoundaryError(writer, test.status, test.code, test.family, test.handlerMsg)
					return
				}
				writeBoundaryError(writer, test.status, test.code, test.family, test.message)
			})
			inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", test.sessionID)
			err := runtimeMetricsCLIProcess.Execute(inputs.Input)
			assertBoundaryCodedFailure(t, err, inputs, string(test.code))
			assertBoundaryRequestLog(t, server.log, "GET /metrics?session_id="+test.sessionID)
		})
	}
}

func TestMetricsSessionSelectedServerKeepsDisjointFacts(t *testing.T) {
	t.Parallel()
	left := newBoundarySessionFixture(t, "left-server", "same-session", "0.0110")
	right := newBoundarySessionFixture(t, "right-server", "same-session", "0.0220")
	servers := []struct {
		name    string
		fixture boundarySessionFixture
		server  boundaryServer
	}{
		{name: "left", fixture: left},
		{name: "right", fixture: right},
	}
	for index := range servers {
		fixture := servers[index].fixture
		servers[index].server = newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/metrics":
				if request.URL.Query().Get("session_id") != fixture.sessionID {
					writeBoundaryError(writer, http.StatusBadRequest, factoryapi.ErrorResponseCode("WRONG_SCOPE"), factoryapi.ErrorFamilyBadRequest, "wrong session scope")
					return
				}
				writeBoundaryMetricsReport(writer, fixture)
			case "/factory-sessions/" + fixture.sessionID + "/events":
				writeBoundaryEvents(writer, fixture.events)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		})
	}

	for index := range servers {
		selected := servers[index]
		otherRequestsBefore := servers[1-index].server.log.snapshot()
		inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", selected.server.URL(), "metrics", "session", selected.fixture.sessionID)
		if err := runtimeMetricsCLIProcess.Execute(inputs.Input); err != nil {
			t.Fatalf("%s selected-server command error = %v\nstdout:\n%s\nstderr:\n%s", selected.name, err, inputs.Stdout(), inputs.Stderr())
		}
		var document boundarySessionDocument
		if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
			t.Fatalf("%s selected-server report: %v\n%s", selected.name, err, inputs.Stdout())
		}
		if len(document.Attempts) != 1 || document.Attempts[0].DispatchID == nil || *document.Attempts[0].DispatchID != selected.fixture.dispatchID {
			t.Fatalf("%s report attempts = %#v, want dispatch %q", selected.name, document.Attempts, selected.fixture.dispatchID)
		}
		if len(document.Attempts[0].WorkIDs) != 1 || document.Attempts[0].WorkIDs[0] != selected.fixture.workID {
			t.Fatalf("%s report work IDs = %#v, want %q", selected.name, document.Attempts[0].WorkIDs, selected.fixture.workID)
		}
		other := servers[1-index].fixture
		if strings.Contains(inputs.Stdout(), other.dispatchID) || strings.Contains(inputs.Stdout(), other.workID) || strings.Contains(inputs.Stdout(), other.workerSession) {
			t.Fatalf("%s report leaked facts from %s: %s", selected.name, servers[1-index].name, inputs.Stdout())
		}
		if inputs.Stderr() != "" {
			t.Fatalf("%s selected-server stderr = %q, want empty", selected.name, inputs.Stderr())
		}
		assertBoundaryRequestLog(t, selected.server.log,
			"GET /metrics?session_id="+selected.fixture.sessionID,
			"GET /factory-sessions/"+selected.fixture.sessionID+"/events",
		)
		if got := servers[1-index].server.log.snapshot(); !reflect.DeepEqual(got, otherRequestsBefore) {
			t.Fatalf("%s selection contacted %s: before=%#v after=%#v", selected.name, servers[1-index].name, otherRequestsBefore, got)
		}
	}
	functionalevidence.Covers(t, "cli/you.metrics.session")
}

func TestMetricsSessionInvalidLensDoesNotContactServer(t *testing.T) {
	server := newBoundaryServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBoundaryError(writer, http.StatusInternalServerError, factoryapi.ErrorResponseCode("UNEXPECTED_REQUEST"), factoryapi.ErrorFamilyInternalServerError, "unexpected request")
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", "session-invalid-lens", "--lens", "forecast")
	err := runtimeMetricsCLIProcess.Execute(inputs.Input)
	assertBoundaryCodedFailure(t, err, inputs, "METRICS_UNSUPPORTED_SESSION_OPTION")
	if len(server.log.snapshot()) != 0 {
		t.Fatalf("invalid lens requests = %#v, want no server request", server.log.snapshot())
	}
}

func TestMetricsSessionReplayFaultsFailClosed(t *testing.T) {
	cases := []struct {
		name       string
		sessionID  string
		writeEvent func(http.ResponseWriter)
	}{
		{
			name: "malformed SSE", sessionID: "malformed-replay-session",
			writeEvent: func(writer http.ResponseWriter) {
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "1")
				_, _ = fmt.Fprint(writer, "data: {not-json}\n\n")
			},
		},
		{
			name: "invalid retained count", sessionID: "invalid-retained-session",
			writeEvent: func(writer http.ResponseWriter) {
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "not-a-count")
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newBoundarySessionFixture(t, strings.ReplaceAll(test.name, " ", "-"), test.sessionID, "0.0100")
			server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/metrics":
					writeBoundaryMetricsReport(writer, fixture)
				case "/factory-sessions/" + test.sessionID + "/events":
					test.writeEvent(writer)
				default:
					writer.WriteHeader(http.StatusNotFound)
				}
			})
			inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", test.sessionID)
			err := runtimeMetricsCLIProcess.Execute(inputs.Input)
			assertBoundaryCodedFailure(t, err, inputs, "METRICS_SESSION_EVENTS_FAILED")
			assertBoundaryRequestLog(t, server.log, "GET /metrics?session_id="+test.sessionID, "GET /factory-sessions/"+test.sessionID+"/events")
		})
	}
}

func TestMetricsSessionTimeoutReturnsNoPartialReport(t *testing.T) {
	fixture := newBoundarySessionFixture(t, "timeout", "timeout-session", "0.0100")
	eventsStarted := make(chan struct{})
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/factory-sessions/timeout-session/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "1")
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			close(eventsStarted)
			<-request.Context().Done()
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	// The deadline is the behavior under test. The readiness select below
	// returns as soon as the real event request reaches the server.
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	inputs := boundaryInputs(t, ctx, "you", "--json", "--server", server.URL(), "metrics", "session", fixture.sessionID)
	command := support.StartProcessCommand(t, runtimeMetricsCLIProcess, inputs.Input)
	command.AcceptError()
	waitBoundarySignal(t, eventsStarted, command.Done(), "timeout event request")
	select {
	case <-command.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the bounded metrics session command")
	}
	err := command.Err()
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout command error = %v, want context deadline", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("timeout stdout = %q, want empty", inputs.Stdout())
	}
	if !strings.Contains(inputs.Stderr(), "METRICS_SESSION_EVENTS_FAILED") {
		t.Fatalf("timeout stderr = %q, want coded replay failure", inputs.Stderr())
	}
}

func TestMetricsSessionCancellationReturnsNoPartialReport(t *testing.T) {
	fixture := newBoundarySessionFixture(t, "cancel", "canceled-session", "0.0100")
	eventsStarted := make(chan struct{})
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/factory-sessions/canceled-session/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "1")
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			close(eventsStarted)
			<-request.Context().Done()
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", fixture.sessionID)
	command := support.StartProcessCommand(t, runtimeMetricsCLIProcess, inputs.Input)
	command.AcceptError()
	waitBoundarySignal(t, eventsStarted, command.Done(), "cancellation event request")
	command.Stop(t)
	if err := command.Err(); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation command error = %v, want context canceled", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("cancellation stdout = %q, want empty", inputs.Stdout())
	}
}

func TestMetricsSessionCostReadFailureDoesNotWritePartialReport(t *testing.T) {
	fixture := newBoundarySessionFixture(t, "cost-failure", "cost-failure-session", "0.0100")
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/factory-sessions/cost-failure-session/events":
			writeBoundaryEvents(writer, fixture.events)
		case "/metrics/costs":
			writeBoundaryJSON(writer, http.StatusInternalServerError, map[string]any{
				"code": "COST_FIXTURE_FAILED", "family": "INTERNAL_SERVER_ERROR",
				"message": "the cost fixture is unavailable", "details": "private-cost-payload",
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", fixture.sessionID, "--lens", "cost")
	err := runtimeMetricsCLIProcess.Execute(inputs.Input)
	assertBoundaryCodedFailure(t, err, inputs, "COST_FIXTURE_FAILED")
	if strings.Contains(inputs.Stderr(), "private-cost-payload") {
		t.Fatalf("cost failure leaked response details: %q", inputs.Stderr())
	}
	assertBoundaryRequestLog(t, server.log,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /factory-sessions/"+fixture.sessionID+"/events",
		"GET /metrics/costs?session_id="+fixture.sessionID,
	)
}

func TestMetricsSessionCancellationDoesNotCorruptConcurrentReport(t *testing.T) {
	survivor := newBoundarySessionFixture(t, "survivor", "survivor-session", "0.0110")
	canceled := newBoundarySessionFixture(t, "canceled", "canceled-session", "0.0220")
	survivorStarted := make(chan struct{})
	canceledStarted := make(chan struct{})
	survivorServer := newBoundaryServer(t, blockingOrFiniteSessionHandler(survivor, survivorStarted, false))
	canceledServer := newBoundaryServer(t, blockingOrFiniteSessionHandler(canceled, canceledStarted, true))

	survivorInputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", survivorServer.URL(), "metrics", "session", survivor.sessionID)
	canceledInputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", canceledServer.URL(), "metrics", "session", canceled.sessionID)
	survivorCommand := support.StartProcessCommand(t, runtimeMetricsCLIProcess, survivorInputs.Input)
	canceledCommand := support.StartProcessCommand(t, runtimeMetricsCLIProcess, canceledInputs.Input)
	canceledCommand.AcceptError()
	waitBoundarySignal(t, survivorStarted, survivorCommand.Done(), "survivor event request")
	waitBoundarySignal(t, canceledStarted, canceledCommand.Done(), "canceled event request")

	select {
	case <-survivorCommand.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for surviving report")
	}
	if err := survivorCommand.Err(); err != nil {
		t.Fatalf("surviving command error = %v", err)
	}
	canceledCommand.Stop(t)
	if err := canceledCommand.Err(); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled command error = %v, want context canceled", err)
	}
	if survivorInputs.Stdout() == "" || !strings.Contains(survivorInputs.Stdout(), survivor.dispatchID) || strings.Contains(survivorInputs.Stdout(), canceled.dispatchID) {
		t.Fatalf("survivor output = %q, want only survivor dispatch", survivorInputs.Stdout())
	}
	if canceledInputs.Stdout() != "" {
		t.Fatalf("canceled output = %q, want empty", canceledInputs.Stdout())
	}
}

func blockingOrFiniteSessionHandler(
	fixture boundarySessionFixture,
	started chan<- struct{},
	block bool,
) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/factory-sessions/" + fixture.sessionID + "/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(fixture.events)))
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			if block {
				<-request.Context().Done()
				return
			}
			writeBoundaryEvents(writer, fixture.events)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}
}

func waitBoundarySignal(t *testing.T, signal <-chan struct{}, done <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-done:
		t.Fatalf("%s command completed before the server boundary was observed", name)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func assertBoundaryCodedFailure(t *testing.T, err error, inputs *support.CapturedInputs, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatal("command error = nil, want failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("failure stdout = %q, want empty", inputs.Stdout())
	}
	var response factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
		t.Fatalf("decode failure diagnostic: %v; stderr=%q", decodeErr, inputs.Stderr())
	}
	if string(response.Code) != wantCode {
		t.Fatalf("failure code = %q, want %q; stderr=%q", response.Code, wantCode, inputs.Stderr())
	}
}
