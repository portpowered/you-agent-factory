package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestSessionInvocationAPI_ReturnsPrimaryResult(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	host := startRootRunInvocationHost(t, dir, support.NewStaticSuccessCommandRunner("primary result COMPLETE"))
	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != "primary result COMPLETE" {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, "primary result COMPLETE")
	}
	assertTerminalDispatchForTrace(t, stream, response.TraceId)
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
	functionalevidence.Covers(t, "rest/invokeFactorySessionBySessionId")
}

func TestSessionInvocationAPI_RejectsWhitespaceOnlyText(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	host := startRootRunInvocationHost(t, dir, support.NewStaticSuccessCommandRunner("primary result COMPLETE"))

	response := postInvocationExpectStatus(
		t,
		host.Endpoint(),
		textInvocationRequest(t, "   ", nil),
		http.StatusBadRequest,
	)
	if string(response.Code) != "INVOCATION_INPUT_EMPTY" {
		t.Fatalf("invocation error code = %q, want INVOCATION_INPUT_EMPTY", response.Code)
	}
	assertInvocationWorkListEmpty(t, host.Endpoint())
}

func TestSessionInvocationAPI_RejectsArgsWithoutActiveSignature(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	host := startRootRunInvocationHost(t, dir, support.NewStaticSuccessCommandRunner("primary result COMPLETE"))

	response := postInvocationExpectStatus(
		t,
		host.Endpoint(),
		factoryapi.InvocationRequest{
			Args: &map[string]any{"input": "hello"},
		},
		http.StatusBadRequest,
	)
	if string(response.Code) != "INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE" {
		t.Fatalf("invocation error code = %q, want INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE", response.Code)
	}
	assertInvocationWorkListEmpty(t, host.Endpoint())
}

func TestSessionInvocationAPI_RejectsInvalidStructuredArgValueShape(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	host := startRootRunInvocationHost(t, dir, support.NewStaticSuccessCommandRunner("primary result COMPLETE"))

	response := postInvocationExpectStatus(
		t,
		host.Endpoint(),
		factoryapi.InvocationRequest{
			Args: &map[string]any{"input": 7},
		},
		http.StatusBadRequest,
	)
	if string(response.Code) != "BAD_REQUEST" {
		t.Fatalf("invocation error code = %q, want BAD_REQUEST", response.Code)
	}
	assertInvocationWorkListEmpty(t, host.Endpoint())
}

func TestSessionInvocationAPI_UnresolvedPrimaryResultReturnsFailedStatus(t *testing.T) {
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
	host := startRootRunInvocationHost(t, dir, support.NewStaticSuccessCommandRunner("task output COMPLETE"))
	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on unresolved output", response.PrimaryResult)
	}
	assertTerminalDispatchForTrace(t, stream, response.TraceId)
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
}

func TestSessionInvocationAPI_TimeoutReturnsTimedOutStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	host := startRootRunInvocationHost(t, dir, blocking)

	timeoutMillis := int64(10)
	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke this", &timeoutMillis))
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("invocation status = %q, want TIMED_OUT", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTIMEDOUT {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_TIMED_OUT", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on timeout", response.PrimaryResult)
	}
}

func startRootRunInvocationHost(t *testing.T, dir string, runner workers.CommandRunner) *support.RootRunFunctionalHost {
	t.Helper()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: true,
		FunctionalEdges: wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		},
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})
	return host
}

func assertInvocationWorkListEmpty(t *testing.T, endpoint string) {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(endpoint, "/work"))
	if len(work.Results) != 0 {
		t.Fatalf("rejected invocation exposed %d work results, want none: %#v", len(work.Results), work.Results)
	}
}

func assertInvocationTraceWorkState(
	t *testing.T,
	endpoint string,
	traceID string,
	wantState string,
	wantType factoryapi.WorkStateType,
) {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(endpoint, "/work"))
	matched := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateName(matched.State) != wantState || generatedWorkStateType(matched.State) != wantType {
		t.Fatalf("invocation GET /work state = %#v, want %s/%s", matched.State, wantState, wantType)
	}
}

func TestSessionInvocationAPI_PausedSessionReturnsPausedStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	pause := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		host.Endpoint()+"/factory-sessions/~default/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause session before invocation",
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause || pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	assertSessionPauseLifecycleEvent(t, stream)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_PAUSED") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PAUSED", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, `session "`+factorysessions.DefaultSessionID+`" is paused`) {
		gotMessage := "<nil>"
		if response.Message != nil {
			gotMessage = *response.Message
		}
		t.Fatalf("invocation message = %q, want paused session detail", gotMessage)
	}
	if response.SessionId == nil || *response.SessionId != factorysessions.DefaultSessionID {
		t.Fatalf("invocation sessionId = %#v, want %q", response.SessionId, factorysessions.DefaultSessionID)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on paused output", response.PrimaryResult)
	}
}

func assertSessionPauseLifecycleEvent(t *testing.T, stream *factoryEventHTTPStream) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
			continue
		}
		payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
		if err != nil {
			t.Fatalf("decode SESSION_LIFECYCLE_CONTROL payload: %v", err)
		}
		if payload.Operation != factoryapi.FactorySessionLifecycleControlKindPause || payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
			t.Fatalf("pause lifecycle event = %#v, want accepted pause", payload)
		}
		return
	}
	t.Fatal("canonical session event stream did not expose accepted pause lifecycle control")
}

type blockingInvocationRunner struct {
	started chan struct{}
}

func newBlockingInvocationRunner() *blockingInvocationRunner {
	return &blockingInvocationRunner{started: make(chan struct{}, 1)}
}

func (r *blockingInvocationRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
}

func scaffoldInvocationFactory(t *testing.T, overrides map[string]any) string {
	t.Helper()

	cfg := simplePipelineConfig()
	for key, value := range overrides {
		cfg[key] = value
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	for i := range workTypes {
		if name, _ := workTypes[i]["name"].(string); name == "task" {
			workTypes[i]["handlingBehavior"] = []string{"DEFAULT"}
		}
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	return dir
}

func textInvocationRequest(t *testing.T, text string, timeoutMillis *int64) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind:    &sourceKind,
		Content:       &content,
		TimeoutMillis: timeoutMillis,
	}
}

func postInvocationExpectStatus(
	t *testing.T,
	serverURL string,
	request factoryapi.InvocationRequest,
	wantStatus int,
) factoryapi.ErrorResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want %d", response.StatusCode, wantStatus)
	}

	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	return decoded
}

func postInvocation(t *testing.T, serverURL string, request factoryapi.InvocationRequest) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d", response.StatusCode)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

var _ workers.CommandRunner = (*blockingInvocationRunner)(nil)
