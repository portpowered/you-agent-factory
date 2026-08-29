// named_factory_test.go holds customer functional scenarios for named JavaScript
// Factory loading through the public CLI, HTTP API, and Factory Session controls.
package loading_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	namedJavaScriptFactoryName        = "named-javascript-loading"
	namedJavaScriptSuccessResult      = "named-javascript-loading:<NAMED_FACTORY_SUCCESS>"
	namedJavaScriptBusyLoopInline     = "var spin=0; while(true){ spin+=1; }"
	namedJavaScriptSessionControlWait = 10 * time.Second
)

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestNamedJavaScriptFactoryUsesSameFactorySessionControls(t *testing.T) {
	fixture := loadingFixtureForTest(t)
	// The Go test runner reuses one TestMain process for -count repetitions.
	// This is the final selector in the package's normal registration order;
	// release the hosted compatibility runtime after its session cleanups so
	// the next repetition can build its own one-process fixture.
	t.Cleanup(func() {
		if err := fixture.shutdown(); err != nil {
			t.Errorf("reset loading fixture after repeated run: %v", err)
		}
		loadingFixtureMu.Lock()
		if sharedLoadingFixture == fixture {
			sharedLoadingFixture = nil
		}
		loadingFixtureMu.Unlock()
	})
	runNamedJavaScriptFactoryUsesSameFactorySessionControls(t, fixture)
}

// TestNamedJavaScriptFactoryRunsThroughStandardCLI proves a named JavaScript
// Factory resolves by name and completes through the public you run customer
// process boundary with a terminal COMPLETED primary outcome tied to the named
// Factory identity and without private VM internals in success diagnostics.
func runNamedJavaScriptFactoryRunsThroughStandardCLI(t *testing.T, fixture *loadingFixture) {
	factoryName := fixture.namedCLI.name
	namedFactoryDir := fixture.namedCLI.factoryDir
	providerCalls := fixture.provider.CallCount()
	result, inputs := fixture.runCLIInvocationAtRoot(t, []string{
		"you", "--json", "run",
		"--named", factoryName,
		"--output", "primary",
		"--no-record",
		"hello",
	}, t.TempDir(), fixture.homeDir, namedFactoryDir)
	if got := fixture.provider.CallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d for named inline factory without child dispatch", got, providerCalls)
	}
	assertNamedJavaScriptSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestNamedJavaScriptFactoryRunsThroughAPIInvocation proves the same named
// JavaScript Factory completes through the public HTTP sync execution customer
// entry path with a terminal COMPLETED primary outcome consistent with the CLI
// named-factory success path and without private VM internals in success diagnostics.
func runNamedJavaScriptFactoryRunsThroughAPIInvocation(t *testing.T, fixture *loadingFixture) {
	fixture.startAPIServer(t)
	factoryName := fixture.namedAPI.name
	namedFactoryDir := fixture.namedAPI.factoryDir
	requestID := fixture.nextRequestID("named-api")
	providerCalls := fixture.provider.CallCount()
	result := invokeNamedJavaScriptFactoryOverHTTP(t, fixture.baseURL, factoryName, "hello", requestID)
	fixture.trackSession(t, result.SessionId, requestID, namedFactoryDir, fixture.homeDir, "api")
	assertNamedJavaScriptSessionSuccessOutcome(t, result, factoryName)
	if got := fixture.provider.CallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d for named inline factory without child dispatch", got, providerCalls)
	}

	responseJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal sync execution response: %v", err)
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, string(responseJSON))
}

// TestNamedJavaScriptFactoryUsesSameFactorySessionControls proves pause and resume
// Factory Session lifecycle controls apply to a named JavaScript Factory session
// started through the public HTTP customer boundary and remain observable on the
// public session surface for that named Factory identity through HTTP and CLI.
func runNamedJavaScriptFactoryUsesSameFactorySessionControls(t *testing.T, fixture *loadingFixture) {
	fixture.startAPIServer(t)
	providerCalls := fixture.provider.CallCount()
	factoryName := fixture.namedControl.name
	namedFactoryDir := fixture.namedControl.factoryDir
	requestID := fixture.nextRequestID("named-controls")
	sessionID := startNamedJavaScriptFactoryAsyncSession(t, fixture.baseURL, factoryName, requestID)
	fixture.trackSession(t, sessionID, requestID, namedFactoryDir, fixture.homeDir, "api")
	waitForNamedJavaScriptSessionStarted(t, fixture.baseURL, sessionID)

	pause := applyNamedJavaScriptSessionLifecycleControl(
		t,
		fixture.baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("HTTP pause response = %#v, want accepted pause", pause)
	}
	assertNamedJavaScriptDurableSessionStatus(
		t,
		readNamedJavaScriptDurableSession(t, fixture.baseURL, sessionID),
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
		factoryName,
	)

	resume := applyNamedJavaScriptSessionLifecycleControl(
		t,
		fixture.baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("HTTP resume response = %#v, want accepted resume", resume)
	}
	assertNamedJavaScriptDurableSessionStatus(
		t,
		readNamedJavaScriptDurableSession(t, fixture.baseURL, sessionID),
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		factoryName,
	)

	cliPause := runNamedJavaScriptSessionLifecycleCLIJSON(t, fixture, t.TempDir(), "pause", sessionID)
	if cliPause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		cliPause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("CLI pause response = %#v, want accepted pause", cliPause)
	}
	assertNamedJavaScriptDurableSessionStatus(
		t,
		readNamedJavaScriptDurableSession(t, fixture.baseURL, sessionID),
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
		factoryName,
	)

	cliResume := runNamedJavaScriptSessionLifecycleCLIJSON(t, fixture, t.TempDir(), "resume", sessionID)
	if cliResume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		cliResume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("CLI resume response = %#v, want accepted resume", cliResume)
	}
	assertNamedJavaScriptDurableSessionStatus(
		t,
		readNamedJavaScriptDurableSession(t, fixture.baseURL, sessionID),
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		factoryName,
	)
	if got := fixture.provider.CallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d for named busy-loop factory without child dispatch", got, providerCalls)
	}
}

func assertNamedJavaScriptSessionSuccessOutcome(
	t *testing.T,
	response factoryapi.FactorySessionSyncExecutionResponse,
	factoryName string,
) {
	t.Helper()

	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %s, want SUCCEEDED", response.Status)
	}
	if response.Result == nil {
		t.Fatal("session result = nil, want FINAL primary outcome")
	}
	if response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %s, want FINAL", response.Result.ResultStatus)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", response.Result.PrimaryResult)
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if part.Text != namedJavaScriptSuccessResult {
		t.Fatalf("primary result = %q, want exact named-factory string %q", part.Text, namedJavaScriptSuccessResult)
	}
	if response.ResolvedSource.SourceRef == nil ||
		!strings.Contains(*response.ResolvedSource.SourceRef, factoryName) {
		t.Fatalf(
			"resolved source ref = %#v, want named factory reference containing %q",
			response.ResolvedSource.SourceRef,
			factoryName,
		)
	}
}

func scaffoldNamedInlineJavaScriptFactorySource(t *testing.T, factoryName string) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": factoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + namedJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
}

func assertNamedJavaScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()

	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal outcome = %s, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if part.Text != namedJavaScriptSuccessResult {
		t.Fatalf("primary result = %q, want exact named-factory string %q", part.Text, namedJavaScriptSuccessResult)
	}
}

func invokeNamedJavaScriptFactoryOverHTTP(
	t *testing.T,
	baseURL string,
	factoryName string,
	prompt string,
	requestID string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factoryID := factoryName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Args:      &map[string]any{"prompt": prompt},
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: &factoryID,
		},
	})
	if err != nil {
		t.Fatalf("marshal sync execution request: %v", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build sync execution request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var responseBody bytes.Buffer
		_, _ = responseBody.ReadFrom(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, responseBody.String())
	}

	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode sync execution response: %v", err)
	}
	return result
}

func scaffoldNamedBusyLoopJavaScriptFactorySource(t *testing.T, factoryName string) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": factoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   namedJavaScriptBusyLoopInline,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
}

func startNamedJavaScriptFactoryAsyncSession(
	t *testing.T,
	baseURL string,
	factoryName string,
	requestID string,
) string {
	t.Helper()

	factoryID := factoryName
	started := postNamedJavaScriptJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: requestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
				FactoryId: &factoryID,
			},
		},
		"start named JavaScript Factory Session",
	)
	if started.SessionId == "" {
		t.Fatalf("async named JavaScript session id is empty: %#v", started)
	}
	return started.SessionId
}

func applyNamedJavaScriptSessionLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	response := postNamedJavaScriptJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply "+string(operation)+" to named JavaScript session "+sessionID,
	)
	if response.Operation != operation {
		t.Fatalf(
			"named JavaScript session %s lifecycle control operation = %q, want %q",
			sessionID,
			response.Operation,
			operation,
		)
	}
	if response.SessionId != sessionID {
		t.Fatalf("lifecycle control sessionId = %q, want %q", response.SessionId, sessionID)
	}
	return response
}

func readNamedJavaScriptDurableSession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable named JavaScript session %s: %v", sessionID, err)
	}
	if session.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", session.SessionId, sessionID)
	}
	return session
}

func waitForNamedJavaScriptSessionStarted(t *testing.T, baseURL string, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), namedJavaScriptSessionControlWait)
	defer cancel()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
	for {
		event := stream.NextEventContext(ctx)
		if event.Type == factoryapi.FactoryEventTypeSessionStarted {
			return
		}
	}
}

func assertNamedJavaScriptDurableSessionStatus(
	t *testing.T,
	session factoryapi.FactorySessionDurableReadModel,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	factoryName string,
) {
	t.Helper()

	if session.Status != want {
		t.Fatalf("named JavaScript durable session status = %q, want %q", session.Status, want)
	}
	if session.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf(
			"named JavaScript durable session orchestratorKind = %q, want %q",
			session.OrchestratorKind,
			factoryapi.JAVASCRIPT,
		)
	}
	if session.ResolvedSource.SourceRef == nil ||
		!strings.Contains(*session.ResolvedSource.SourceRef, factoryName) {
		t.Fatalf(
			"named JavaScript durable session resolved source ref = %#v, want named factory reference containing %q",
			session.ResolvedSource.SourceRef,
			factoryName,
		)
	}
}

func runNamedJavaScriptSessionLifecycleCLIJSON(
	t *testing.T,
	fixture *loadingFixture,
	workingDir string,
	operation string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--json", "--server", fixture.baseURL,
		"session", operation, sessionID,
	})
	inputs.Input.Env = loadingCustomerEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = workingDir

	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(session %s) error = %v\nstdout:\n%s\nstderr:\n%s",
			operation,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("session %s stderr = %q, want empty stderr on successful JSON invocation", operation, inputs.Stderr())
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode session %s JSON: %v\noutput:\n%s", operation, err, inputs.Stdout())
	}
	return response
}

func postNamedJavaScriptJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()

	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	response, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var out T
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}
