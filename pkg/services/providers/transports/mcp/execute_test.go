package providersmcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
)

const testExecuteInputJSON = `{
	"provider":"codex",
	"attemptId":"attempt-1",
	"workerType":"agent",
	"workstationName":"ws-1",
	"model":"gpt-test",
	"reasoningEffort":"xhigh",
	"skipPermissions":true,
	"systemPrompt":"system",
	"userMessage":"hello",
	"outputSchema":"{}",
	"resumeSession":{"provider":"codex","kind":"session_id","id":"session-1"},
	"workingDirectory":"/tmp/work",
	"worktree":"/tmp/worktree",
	"envVars":{"KEY":"value"},
	"processEnvironment":["PATH=/bin"]
}`

func TestBind_ExecuteSuccessReturnsDetachedResultFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantResult := providers.ExecuteResult{
		Content: "hello-result",
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     "session_id",
			ID:       "session-attempt-1",
		},
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Progress: []providers.ExecuteProgress{{
				Phase:  "running",
				Detail: "thinking",
			}},
			Metadata: map[string]string{"attempt": "attempt-1"},
		},
	}
	fake := fakeProvidersRoot{
		invoked: &invoked,
		execute: func(
			_ context.Context,
			request providers.ExecuteRequest,
		) (providers.ExecuteResult, error) {
			if request.Provider != providers.IDCodex {
				t.Fatalf("provider = %q, want %q", request.Provider, providers.IDCodex)
			}
			if request.AttemptID != "attempt-1" {
				t.Fatalf("attempt id = %q, want attempt-1", request.AttemptID)
			}
			if request.WorkerType != "agent" ||
				request.WorkstationName != "ws-1" ||
				request.Model != "gpt-test" ||
				request.ReasoningEffort != "xhigh" ||
				!request.SkipPermissions ||
				request.SystemPrompt != "system" ||
				request.UserMessage != "hello" ||
				request.OutputSchema != "{}" ||
				request.WorkingDirectory != "/tmp/work" ||
				request.Worktree != "/tmp/worktree" {
				t.Fatalf("execute request = %#v, want mapped MCP fields", request)
			}
			if request.ResumeSession == nil ||
				request.ResumeSession.Provider != providers.IDCodex ||
				request.ResumeSession.Kind != "session_id" ||
				request.ResumeSession.ID != "session-1" {
				t.Fatalf("resume session = %#v, want codex session_id session-1", request.ResumeSession)
			}
			if request.EnvVars["KEY"] != "value" {
				t.Fatalf("env vars = %#v, want KEY=value", request.EnvVars)
			}
			if len(request.ProcessEnvironment) != 1 || request.ProcessEnvironment[0] != "PATH=/bin" {
				t.Fatalf("process environment = %#v, want PATH=/bin", request.ProcessEnvironment)
			}
			return wantResult.Clone(), nil
		},
	}
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolExecute,
		json.RawMessage(testExecuteInputJSON),
	)
	if err != nil {
		t.Fatalf("CallTool(execute) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	var response providersmcp.ToolResponse[providers.ExecuteResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	got := response.Result
	if got.Content != wantResult.Content {
		t.Fatalf("content = %q, want %q", got.Content, wantResult.Content)
	}
	if got.SessionRef == nil ||
		got.SessionRef.Provider != wantResult.SessionRef.Provider ||
		got.SessionRef.Kind != wantResult.SessionRef.Kind ||
		got.SessionRef.ID != wantResult.SessionRef.ID {
		t.Fatalf("session ref = %#v, want %#v", got.SessionRef, wantResult.SessionRef)
	}
	if got.Diagnostics == nil ||
		got.Diagnostics.DurationMillis != wantResult.Diagnostics.DurationMillis ||
		len(got.Diagnostics.Progress) != 1 ||
		got.Diagnostics.Progress[0].Phase != "running" {
		t.Fatalf("diagnostics = %#v, want %#v", got.Diagnostics, wantResult.Diagnostics)
	}
}

func TestDiscoverTools_ExecuteDiscoveryMatchesHandlerRegistration(t *testing.T) {
	t.Parallel()

	tool, ok := providersmcp.ToolByName(providersmcp.ToolExecute)
	if !ok {
		t.Fatal("ToolByName(execute) ok = false, want true")
	}
	if tool.Name != providersmcp.ToolExecute {
		t.Fatalf("discovered name = %q, want %q", tool.Name, providersmcp.ToolExecute)
	}
	if !providersmcp.IsCanonicalToolHandlerRegistered(providersmcp.ToolExecute) {
		t.Fatal("execute handler is not registered on canonical CallTool path")
	}
	for _, field := range []string{
		"result.Content",
		"result.SessionRef",
		"result.Diagnostics",
	} {
		if !containsString(tool.SuccessStableFields, field) {
			t.Fatalf("success stable fields = %#v, want %q", tool.SuccessStableFields, field)
		}
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("execute input schema properties = %#v, want object map", tool.InputSchema["properties"])
	}
	for _, field := range []string{"provider", "attemptId"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("execute input schema missing %q property", field)
		}
	}
	resultSchema, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("execute output schema properties missing")
	}
	if _, ok := resultSchema["result"]; !ok {
		t.Fatal("execute output schema missing result envelope")
	}
}

func TestBind_ExecuteFailuresReturnIdentityAndCatalogEnvelopes(t *testing.T) {
	t.Parallel()

	assertExecuteRootErrorEnvelopes(t, []executeRootErrorEnvelopeCase{
		{
			name:          "invalid provider id",
			rootErr:       providers.ErrInvalidID,
			wantCode:      "provider.identity.invalid",
			wantMessage:   "provider id is invalid",
			wantRetryable: false,
		},
		{
			name:          "unknown provider",
			rootErr:       providers.ErrUnknownProvider,
			wantCode:      "provider.catalog.unknown",
			wantMessage:   "provider is unknown",
			wantRetryable: false,
		},
	})
}

func TestBind_ExecuteFailuresReturnCancelAndTimeoutEnvelopes(t *testing.T) {
	t.Parallel()

	assertExecuteRootErrorEnvelopes(t, []executeRootErrorEnvelopeCase{
		{
			name:          "execute canceled sentinel",
			rootErr:       providers.ErrExecuteCancelled,
			wantCode:      "provider.execution.canceled",
			wantMessage:   "provider execution was canceled",
			wantRetryable: false,
			wantKind:      "canceled",
		},
		{
			name:          "execute timeout sentinel",
			rootErr:       providers.ErrExecuteTimeout,
			wantCode:      "provider.execution.timed_out",
			wantMessage:   "provider execution timed out",
			wantRetryable: true,
			wantKind:      "timeout",
		},
		{
			name: "execute failure canceled",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindCanceled,
				Message: "attempt canceled by provider",
			},
			wantCode:      "provider.execution.canceled",
			wantMessage:   "attempt canceled by provider",
			wantRetryable: false,
			wantKind:      "canceled",
		},
		{
			name: "execute failure timeout",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindTimeout,
				Message: "attempt exceeded deadline",
			},
			wantCode:      "provider.execution.timed_out",
			wantMessage:   "attempt exceeded deadline",
			wantRetryable: true,
			wantKind:      "timeout",
		},
	})
}

func TestBind_ExecuteFailuresReturnExecuteFailureKindEnvelopes(t *testing.T) {
	t.Parallel()

	assertExecuteRootErrorEnvelopes(t, []executeRootErrorEnvelopeCase{
		{
			name: "execute failure authentication",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindAuthentication,
				Message: "invalid credentials",
			},
			wantCode:      "provider.execution.authentication",
			wantMessage:   "invalid credentials",
			wantRetryable: false,
			wantKind:      "authentication",
		},
		{
			name: "execute failure invalid request",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: "missing user message",
			},
			wantCode:      "provider.execution.invalid_request",
			wantMessage:   "missing user message",
			wantRetryable: false,
			wantKind:      "invalid_request",
		},
		{
			name: "execute failure throttled",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindThrottled,
				Message: "rate limited",
			},
			wantCode:      "provider.execution.throttled",
			wantMessage:   "rate limited",
			wantRetryable: true,
			wantKind:      "throttled",
		},
		{
			name: "execute failure dependency",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindDependency,
				Message: "provider binary missing",
			},
			wantCode:      "provider.execution.dependency",
			wantMessage:   "provider binary missing",
			wantRetryable: true,
			wantKind:      "dependency",
		},
		{
			name: "execute failure unknown",
			rootErr: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindUnknown,
				Message: "unexpected provider exit",
			},
			wantCode:      "provider.execution.unknown",
			wantMessage:   "unexpected provider exit",
			wantRetryable: false,
			wantKind:      "unknown",
		},
	})
}

type executeRootErrorEnvelopeCase struct {
	name          string
	rootErr       error
	wantCode      string
	wantMessage   string
	wantRetryable bool
	wantKind      string
}

func assertExecuteRootErrorEnvelopes(t *testing.T, cases []executeRootErrorEnvelopeCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := mustCallExecute(t, fakeProvidersRoot{
				execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
					return providers.ExecuteResult{}, tc.rootErr
				},
			})
			envelope := assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantRetryable)
			if envelope.Message != tc.wantMessage {
				t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, tc.wantMessage, envelope)
			}
			if tc.wantKind != "" {
				kind, ok := envelope.Details["kind"].(string)
				if !ok || kind != tc.wantKind {
					t.Fatalf("error.details.kind = %#v, want %q; envelope = %#v", envelope.Details["kind"], tc.wantKind, envelope)
				}
			}
		})
	}
}

func TestBind_ExecuteFailureCodesAreDistinct(t *testing.T) {
	t.Parallel()

	throttledRaw := mustCallExecute(t, fakeProvidersRoot{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled}
		},
	})
	canceledRaw := mustCallExecute(t, fakeProvidersRoot{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
		},
	})
	unknownRaw := mustCallExecute(t, fakeProvidersRoot{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrUnknownProvider
		},
	})

	throttled := assertTypedToolErrorEnvelope(t, throttledRaw, "provider.execution.throttled", true)
	canceled := assertTypedToolErrorEnvelope(t, canceledRaw, "provider.execution.canceled", false)
	unknown := assertTypedToolErrorEnvelope(t, unknownRaw, "provider.catalog.unknown", false)

	if throttled.Code == canceled.Code || throttled.Code == unknown.Code || canceled.Code == unknown.Code {
		t.Fatalf(
			"execute failure codes must be distinct: throttled=%q canceled=%q unknown=%q",
			throttled.Code,
			canceled.Code,
			unknown.Code,
		)
	}
}

func TestBind_ExecuteNilServiceReturnsUnavailableEnvelope(t *testing.T) {
	t.Parallel()

	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: nil})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolExecute,
		json.RawMessage(`{"provider":"codex","attemptId":"attempt-1"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(execute) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"provider.service.unavailable",
		false,
	)
	if envelope.Message != "providers service is unavailable" {
		t.Fatalf("error.message = %q, want unavailable message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ExecuteMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.Bind(providersmcp.RootDependencies{
		Providers: fakeProvidersRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolExecute,
		json.RawMessage(`{`),
	)
	if err != nil {
		t.Fatalf("CallTool(execute) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode execute input") {
		t.Fatalf("error.message = %q, want decode execute input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for malformed JSON")
	}
}

func TestBind_ExecuteWrappedFailuresPreserveTypedCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		rootErr  error
		wantCode string
	}{
		{
			name:     "wrapped canceled",
			rootErr:  fmt.Errorf("provider attempt: %w", providers.ErrExecuteCancelled),
			wantCode: "provider.execution.canceled",
		},
		{
			name:     "wrapped timeout",
			rootErr:  fmt.Errorf("provider attempt: %w", providers.ErrExecuteTimeout),
			wantCode: "provider.execution.timed_out",
		},
		{
			name: "wrapped execute failure",
			rootErr: fmt.Errorf("provider attempt: %w", providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindAuthentication,
			}),
			wantCode: "provider.execution.authentication",
		},
		{
			name:     "wrapped unknown provider",
			rootErr:  fmt.Errorf("lookup provider: %w", providers.ErrUnknownProvider),
			wantCode: "provider.catalog.unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := mustCallExecute(t, fakeProvidersRoot{
				execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
					return providers.ExecuteResult{}, tc.rootErr
				},
			})
			assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantCode == "provider.execution.timed_out")
		})
	}
}

func mustCallExecute(t *testing.T, fake fakeProvidersRoot) json.RawMessage {
	t.Helper()

	var invoked bool
	fake.invoked = &invoked
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolExecute,
		json.RawMessage(`{"provider":"codex","attemptId":"attempt-1"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(execute) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	return raw
}
