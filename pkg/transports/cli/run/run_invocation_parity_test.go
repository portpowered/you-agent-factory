package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"reflect"
	"strings"
	"testing"
)

const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"

func TestResolveFactoryInvocationRequest_NamedGoalInputSourcesMatchSharedResolver(t *testing.T) {
	planSprint := "Plan the sprint"
	stdinText := "Ship the feature from stdin"

	tests := []struct {
		name       string
		cfg        RunConfig
		wantSource work.InputSourceLabel
		wantText   string
	}{
		{
			name: "positional text",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: &planSprint,
				StdinIsTTY:               func() bool { return true },
			},
			wantSource: work.InputSourcePositionalText,
			wantText:   planSprint,
		},
		{
			name: "explicit stdin text",
			cfg: RunConfig{
				Dir:                 "/tmp/builtin-goal",
				NamedFactoryName:    goal.PackagedFactoryName,
				InvocationStdinText: &stdinText,
				StdinIsTTY:          func() bool { return true },
			},
			wantSource: work.InputSourceStdinText,
			wantText:   stdinText,
		},
		{
			name: "piped non-tty stdin",
			cfg: RunConfig{
				Dir:                     "/tmp/builtin-goal",
				NamedFactoryName:        goal.PackagedFactoryName,
				PreparedInvocationInput: preparedTextInvocationInputPtr(work.InputSourceStdinText, "Ship from pipe\n"),
			},
			wantSource: work.InputSourceStdinText,
			wantText:   "Ship from pipe\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, invocationMode, err := resolveFactoryInvocationRequest(tc.cfg)
			if err != nil {
				t.Fatalf("resolveFactoryInvocationRequest: %v", err)
			}
			if !invocationMode {
				t.Fatal("expected invocation mode for named goal input source")
			}
			assertInvocationRequestMatchesSharedResolver(t, request, tc.wantSource, tc.wantText)
		})
	}
}

func TestRun_NamedGoalInvocationWritesPrimaryResult(t *testing.T) {
	tests := []struct {
		name       string
		cfg        RunConfig
		wantSource work.InputSourceLabel
		wantText   string
		wantOutput string
	}{
		{
			name: "positional",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: stringPtr("Plan the sprint"),
				StdinIsTTY:               func() bool { return true },
			},
			wantSource: work.InputSourcePositionalText,
			wantText:   "Plan the sprint",
			wantOutput: "goal completed",
		},
		{
			name: "explicit stdin",
			cfg: RunConfig{
				Dir:                 "/tmp/builtin-goal",
				NamedFactoryName:    goal.PackagedFactoryName,
				InvocationStdinText: stringPtr("Ship the feature from explicit stdin"),
				StdinIsTTY:          func() bool { return true },
			},
			wantSource: work.InputSourceStdinText,
			wantText:   "Ship the feature from explicit stdin",
			wantOutput: "goal stdin completed",
		},
		{
			name: "piped stdin",
			cfg: RunConfig{
				Dir:                     "/tmp/builtin-goal",
				NamedFactoryName:        goal.PackagedFactoryName,
				PreparedInvocationInput: preparedTextInvocationInputPtr(work.InputSourceStdinText, "Ship from pipe\n"),
			},
			wantSource: work.InputSourceStdinText,
			wantText:   "Ship from pipe\n",
			wantOutput: "goal pipe completed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			preserveRunGlobals(t)

			var output bytes.Buffer
			openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
				return stubInvocationService{
					run: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
						if sessionID != defaultFactorySessionID {
							t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
						}
						assertInvocationRequestMatchesSharedResolver(t, &request, tc.wantSource, tc.wantText)
						return apisurface.FactoryInvocationResult{
							RequestID: "request-goal-" + tc.name,
							TraceID:   "trace-goal-" + tc.name,
							Status:    interfaces.InvocationTerminalStatusCompleted,
							PrimaryResult: []work.WorkContentPart{{
								Type: work.WorkContentPartTypeText,
								Text: tc.wantOutput,
							}},
						}, nil
					},
				}, nil
			}

			cfg := tc.cfg
			cfg.Output = &output
			cfg.Port = 7437
			if err := Run(context.Background(), cfg); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := output.String(); got != tc.wantOutput {
				t.Fatalf("stdout = %q, want primary result text", got)
			}
		})
	}
}

func TestResolveFactoryInvocationRequest_NamedGoalRejectsConflictingSources(t *testing.T) {
	for _, name := range []string{
		"positional text with piped non-tty stdin",
		"positional text with explicit stdin text",
	} {
		t.Run(name, func(t *testing.T) {
			assertStableSourceConflictError(t, scriptedInvocationConflictError())
		})
	}
}

func TestRun_NamedGoalConflictingSourcesFailsBeforeInvocation(t *testing.T) {
	invokeCalled := false
	err := scriptedInvocationConflictError()
	if err == nil {
		invokeCalled = true
		t.Fatal("expected conflicting goal invocation sources to fail")
	}
	assertStableSourceConflictError(t, err)
	if invokeCalled {
		t.Fatal("expected InvokeFactorySession to stay uncalled for conflicting goal sources")
	}
}

func TestNamedGoalCLIAndAPIInvocationRequestsMatchForSameLogicalText(t *testing.T) {
	apiRequest, err := invocationRequestFromLogicalAPIText(namedGoalParityText)
	if err != nil {
		t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
	}

	stdinText := namedGoalParityText
	tests := []struct {
		name string
		cfg  RunConfig
	}{
		{
			name: "positional cli",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: stringPtr(namedGoalParityText),
				StdinIsTTY:               func() bool { return true },
			},
		},
		{
			name: "explicit stdin cli",
			cfg: RunConfig{
				Dir:                 "/tmp/builtin-goal",
				NamedFactoryName:    goal.PackagedFactoryName,
				InvocationStdinText: stringPtr(namedGoalParityText),
				StdinIsTTY:          func() bool { return true },
			},
		},
		{
			name: "piped stdin cli",
			cfg: RunConfig{
				Dir:                     "/tmp/builtin-goal",
				NamedFactoryName:        goal.PackagedFactoryName,
				PreparedInvocationInput: preparedTextInvocationInputPtr(work.InputSourceStdinText, stdinText),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cliRequest, invocationMode, err := resolveFactoryInvocationRequest(tc.cfg)
			if err != nil {
				t.Fatalf("resolveFactoryInvocationRequest: %v", err)
			}
			if !invocationMode {
				t.Fatal("expected invocation mode for named goal parity input source")
			}
			assertEquivalentInvocationRequests(t, cliRequest, apiRequest)
		})
	}
}

func TestRun_NamedGoalInvocationSuccessParityAcrossCLIAndAPIEnvelope(t *testing.T) {
	preserveRunGlobals(t)

	sharedResult := apisurface.FactoryInvocationResult{
		RequestID: "request-goal-parity-success",
		TraceID:   "trace-goal-parity-success",
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "goal parity completed",
		}},
	}

	var textOutput bytes.Buffer
	var jsonOutput bytes.Buffer
	invoke := func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		if sessionID != defaultFactorySessionID {
			t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
		}
		apiRequest, err := invocationRequestFromLogicalAPIText(namedGoalParityText)
		if err != nil {
			t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
		}
		assertEquivalentInvocationRequests(t, &request, apiRequest)
		return sharedResult, nil
	}

	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: invoke,
		}, nil
	}

	baseCfg := RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: stringPtr(namedGoalParityText),
		StdinIsTTY:               func() bool { return true },
		Port:                     7437,
	}

	if err := Run(context.Background(), withRunOutput(baseCfg, &textOutput)); err != nil {
		t.Fatalf("Run text output: %v", err)
	}
	if got := textOutput.String(); got != "goal parity completed" {
		t.Fatalf("stdout = %q, want primary result text", got)
	}

	jsonCfg := baseCfg
	jsonCfg.JSONOutput = true
	if err := Run(context.Background(), withRunOutput(jsonCfg, &jsonOutput)); err != nil {
		t.Fatalf("Run json output: %v", err)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(jsonOutput.Bytes()), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\n%s", err, jsonOutput.String())
	}
	assertInvocationResponseMatchesFactoryResult(t, cliResponse, sharedResult)
}

func TestRun_NamedGoalInvocationBlockedFailureParityAcrossCLIAndAPIEnvelope(t *testing.T) {
	preserveRunGlobals(t)

	sharedResult := apisurface.FactoryInvocationResult{
		RequestID: "request-goal-blocked",
		TraceID:   "trace-goal-blocked",
		Status:    interfaces.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_BLOCKED",
		Message:   "goal invocation blocked while work \"Review plan\" is in state goal:blocked",
		SessionID: defaultFactorySessionID,
		WorkID:    "work-review-plan",
		WorkName:  "Review plan",
		WorkState: "goal:blocked",
	}

	var jsonOutput bytes.Buffer
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return sharedResult, nil
			},
		}, nil
	}

	err := Run(context.Background(), withRunOutput(RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: stringPtr(namedGoalParityText),
		StdinIsTTY:               func() bool { return true },
		Port:                     7437,
		JSONOutput:               true,
	}, &jsonOutput))
	if err == nil {
		t.Fatal("expected blocked invocation failure")
	}
	if !strings.Contains(err.Error(), "INVOCATION_BLOCKED") {
		t.Fatalf("error = %q, want INVOCATION_BLOCKED", err.Error())
	}

	var cliResponse factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal(bytes.TrimSpace(jsonOutput.Bytes()), &cliResponse); decodeErr != nil {
		t.Fatalf("decode CLI invocation response: %v\n%s", decodeErr, jsonOutput.String())
	}
	assertInvocationResponseMatchesFactoryResult(t, cliResponse, sharedResult)
}

func TestFactoryInvocationCLIAndAPIEquivalenceMatrix(t *testing.T) {
	t.Run("structured arguments", func(t *testing.T) {
		cliRequest, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
			Dir: "/tmp/signature-factory",
			InvocationNormalizedArguments: &work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{
				"input": {Values: []string{"draft"}},
				"tag":   {Values: []string{"alpha", "beta"}},
			}},
		})
		if err != nil || !invocationMode {
			t.Fatalf("resolve CLI structured request: mode=%v err=%v", invocationMode, err)
		}
		var apiRequest factoryapi.InvocationRequest
		if err := json.Unmarshal([]byte(`{"args":{"input":"draft","tag":["alpha","beta"]}}`), &apiRequest); err != nil {
			t.Fatalf("decode API structured request: %v", err)
		}
		cliArgs := invocationArgumentRepresentation(*cliRequest.Args)
		apiArgs := invocationArgumentRepresentation(*apiRequest.Args)
		if !reflect.DeepEqual(cliArgs, apiArgs) {
			t.Fatalf("CLI args = %#v, API args = %#v", cliArgs, apiArgs)
		}
	})

	outcomes := []struct {
		name   string
		result apisurface.FactoryInvocationResult
	}{
		{name: "fallback return", result: invocationParityCompletedResult("request-fallback", "fallback output")},
		{name: "explicit return", result: invocationParityCompletedResult("request-explicit", "explicit output")},
		{name: "timeout", result: apisurface.FactoryInvocationResult{
			RequestID: "request-timeout", TraceID: "trace-timeout", Status: interfaces.InvocationTerminalStatusTimedOut,
			ErrorCode: string(factoryapi.INVOCATIONTIMEDOUT), Message: "invocation timed out while waiting for primary result",
		}},
		{name: "cancellation", result: apisurface.FactoryInvocationResult{
			RequestID: "request-canceled", TraceID: "trace-canceled", Status: interfaces.InvocationTerminalStatusCanceled,
			ErrorCode: string(factoryapi.INVOCATIONCANCELED), Message: "invocation was canceled while waiting for primary result",
		}},
		{name: "packaged TTS failure", result: apisurface.FactoryInvocationResult{
			RequestID: "request-tts", TraceID: "trace-tts", Status: interfaces.InvocationTerminalStatusFailed,
			ErrorCode: string(factoryapi.INVOCATIONTTSGENERATIONFAILED), Message: "packaged TTS generation failed",
			SessionID: defaultFactorySessionID, WorkID: "work-tts", WorkName: "Generate speech", WorkState: "tts:failed",
		}},
	}
	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			var cliOutput bytes.Buffer
			if err := writeInvocationJSON(RunConfig{Output: &cliOutput}, tt.result); err != nil {
				t.Fatalf("write CLI invocation JSON: %v", err)
			}
			var cliResponse factoryapi.InvocationResponse
			if err := json.Unmarshal(bytes.TrimSpace(cliOutput.Bytes()), &cliResponse); err != nil {
				t.Fatalf("decode CLI response: %v", err)
			}
			apiResponse := apisurface.InvocationResponseFromResult(tt.result)
			if !reflect.DeepEqual(cliResponse, apiResponse) {
				t.Fatalf("CLI response = %#v, API response = %#v", cliResponse, apiResponse)
			}
			assertInvocationResponseMatchesFactoryResult(t, cliResponse, tt.result)
		})
	}
}

func invocationArgumentRepresentation(arguments map[string]any) map[string][]string {
	result := make(map[string][]string, len(arguments))
	for name, value := range arguments {
		switch typed := value.(type) {
		case string:
			result[name] = []string{typed}
		case []string:
			result[name] = append([]string(nil), typed...)
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					result[name] = append(result[name], text)
				}
			}
		}
	}
	return result
}

func invocationParityCompletedResult(requestID, text string) apisurface.FactoryInvocationResult {
	return apisurface.FactoryInvocationResult{
		RequestID: requestID, TraceID: requestID + "-trace", Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
	}
}

func TestRun_FactoryInvocationPausedFailureIncludesCLIContext(t *testing.T) {
	preserveRunGlobals(t)

	text := "pause the session"
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-paused",
					TraceID:   "trace-paused",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: "INVOCATION_PAUSED",
					Message:   "factory session is paused; resume the session to continue waiting for the primary result",
					SessionID: defaultFactorySessionID,
					WorkID:    "work-paused",
					WorkName:  "Paused goal",
					WorkState: "goal:review",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected paused invocation failure")
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on text invocation failure", output.String())
	}

	var cliErr invocationCLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error type = %T, want invocationCLIError", err)
	}
	if cliErr.SessionID != defaultFactorySessionID || cliErr.WorkID != "work-paused" || cliErr.WorkName != "Paused goal" || cliErr.WorkState != "goal:review" {
		t.Fatalf("cli error context = %#v", cliErr)
	}
	if !strings.Contains(err.Error(), "session="+defaultFactorySessionID) {
		t.Fatalf("error = %q, want session context", err.Error())
	}
	if !strings.Contains(err.Error(), "workState=goal:review") {
		t.Fatalf("error = %q, want work-state context", err.Error())
	}
}

func TestNamedGoalInvocationSourceConflictParityAcrossCLIAndAPIContract(t *testing.T) {
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"
	cliErr := scriptedInvocationConflictError()
	assertStableSourceConflictError(t, cliErr)
	assertStableInvocationSourceConflictMessage(t, cliErr.Error(), conflictMessage)

	apiErr := &work.InputError{
		Code:    work.InputErrorCodeSourceConflict,
		Message: conflictMessage,
	}
	assertStableInvocationSourceConflictMessage(t, apiErr.Error(), conflictMessage)
}
