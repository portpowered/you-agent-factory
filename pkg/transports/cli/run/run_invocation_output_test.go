package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NamedFactoryModelNotReadyKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "hi there"
	var output bytes.Buffer
	core, observedLogs := observer.New(zap.InfoLevel)

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-not-ready",
					TraceID:   "trace-tts-not-ready",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: interfaces.TTSInvocationErrorCodeModelNotReady,
					Message:   "model not available: required assets missing",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
		Logger:                   zap.New(core),
	})
	if err == nil {
		t.Fatal("expected model-not-ready invocation failure")
	}
	if !strings.Contains(err.Error(), interfaces.TTSInvocationErrorCodeModelNotReady) {
		t.Fatalf("error = %q, want %s", err.Error(), interfaces.TTSInvocationErrorCodeModelNotReady)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty without success metadata", output.String())
	}

	startLogs := observedLogs.FilterMessage("packaged tts invocation started").All()
	if len(startLogs) != 1 {
		t.Fatalf("packaged start logs = %d, want 1", len(startLogs))
	}
	if got := startLogs[0].ContextMap()["tts_backend"]; got == "" {
		t.Fatal("expected tts_backend field in packaged start log")
	}
}

func TestRun_NamedFactoryGenerationFailureKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "hi there"
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-failed",
					TraceID:   "trace-tts-failed",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: interfaces.TTSInvocationErrorCodeGenerationFailed,
					Message:   "omnivoice invoke failed: exit status 1",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected generation failure")
	}
	if !strings.Contains(err.Error(), interfaces.TTSInvocationErrorCodeGenerationFailed) {
		t.Fatalf("error = %q, want %s", err.Error(), interfaces.TTSInvocationErrorCodeGenerationFailed)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty without success metadata", output.String())
	}
}

func TestRun_NamedFactoryStdinInvocationWritesMetadataPrimaryResult(t *testing.T) {
	preserveRunGlobals(t)

	stdinText := "hi there"
	metadataJSON := `{"artifactPath":"/tmp/speech.wav","mediaType":"audio/wav","backend":"OMNIVOICE_Q4_K_M/LLAMACPP"}`
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				if got := extractInvocationText(t, &request); got != stdinText {
					t.Fatalf("invocation text = %q, want %q", got, stdinText)
				}
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-stdin",
					TraceID:   "trace-tts-stdin",
					Status:    interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: metadataJSON,
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                 "/tmp/builtin-tts",
		NamedFactoryName:    "@you/tts",
		InvocationStdinText: &stdinText,
		StdinIsTTY:          func() bool { return true },
		Output:              &output,
		Port:                7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != metadataJSON {
		t.Fatalf("stdout = %q, want packaged TTS metadata JSON", got)
	}

	var metadata interfaces.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(output.String()), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.ArtifactPath != "/tmp/speech.wav" || metadata.MediaType != "audio/wav" || metadata.Backend == "" {
		t.Fatalf("metadata = %#v, want artifact path, media type, and backend", metadata)
	}
}

func TestRun_FactoryInvocationWritesPrimaryTextOnly(t *testing.T) {
	preserveRunGlobals(t)

	text := "Fix the lint issues"
	var output bytes.Buffer
	var captured *testRuntimeSelections

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, edges serviceedges.Edges) (sessionInvocationRunner, error) {
		captured = cfg
		_ = edges
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				if got := extractInvocationText(t, &request); got != text {
					t.Fatalf("invocation text = %q, want %q", got, text)
				}
				return apisurface.FactoryInvocationResult{
					RequestID: "request-123",

					TraceID: "trace-123",
					Status:  interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "final output",
					}},
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
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "final output" {
		t.Fatalf("stdout = %q, want only primary result text", got)
	}
	if captured == nil {
		t.Fatal("expected invocation run to build a service config")
	}
	if captured.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want service", captured.RuntimeMode)
	}
	if captured.WorkFile != "" {
		t.Fatalf("work file = %q, want empty for invocation mode", captured.WorkFile)
	}
}

func TestRun_FactoryInvocationPreservesOrderedPrimaryContentParts(t *testing.T) {
	for _, test := range []struct {
		name       string
		jsonOutput bool
		wantText   string
	}{
		{name: "text", wantText: "first part\nsecond part"},
		{name: "json", jsonOutput: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			preserveRunGlobals(t)

			text := "preserve these parts"
			var output bytes.Buffer
			openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
				return stubInvocationService{
					run: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
						return apisurface.FactoryInvocationResult{
							RequestID: "request-multipart",
							TraceID:   "trace-multipart",
							Status:    interfaces.InvocationTerminalStatusCompleted,
							PrimaryResult: []work.WorkContentPart{
								{Type: work.WorkContentPartTypeText, Text: "first part"},
								{Type: work.WorkContentPartTypeText, Text: "second part"},
							},
						}, nil
					},
				}, nil
			}

			err := Run(context.Background(), RunConfig{
				FactoryConfigPath:        "/tmp/factory.json",
				InvocationPositionalText: &text,
				StdinIsTTY:               func() bool { return true },
				JSONOutput:               test.jsonOutput,
				Output:                   &output,
				Port:                     7437,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			if !test.jsonOutput {
				if got := output.String(); got != test.wantText {
					t.Fatalf("stdout = %q, want ordered text parts", got)
				}
				return
			}

			var response factoryapi.InvocationResponse
			if err := json.Unmarshal(output.Bytes(), &response); err != nil {
				t.Fatalf("decode JSON response: %v\n%s", err, output.String())
			}
			assertGeneratedWorkContentPartsFromResponse(t, response.PrimaryResult, []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "first part"},
				{Type: work.WorkContentPartTypeText, Text: "second part"},
			})
		})
	}
}

func TestRun_FactoryInvocationFailureKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "Fix the lint issues"
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-123",
					TraceID:   "trace-123",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
					Message:   "primary result could not be resolved",
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
		t.Fatal("expected invocation failure")
	}
	if !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("error = %q, want stable unresolved code", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invocation failure", output.String())
	}
}

func TestRunRemoteInvocationUsesSelectedEndpointAndNormalizedRequest(t *testing.T) {
	var gotRequest factoryapi.FactorySessionExecutionRequest
	resultCalls := 0
	server := httptest.NewServer(remoteSuccessHandler(t, &gotRequest, &resultCalls))
	defer server.Close()

	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	text := "same normalized prompt"
	var output bytes.Buffer
	err = RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments(text),
		JSONOutput:              true,
		Output:                  &output,
	}, server.URL+"/selected", NewRemoteInvocation(transport))
	if err != nil {
		t.Fatalf("RunRemoteInvocation: %v", err)
	}
	assertRemoteSuccessRequest(t, gotRequest, text)
	if resultCalls != 2 {
		t.Fatalf("durable result calls = %d, want a not-ready poll followed by one authoritative terminal read", resultCalls)
	}
	assertRemoteSuccessResponse(t, output.Bytes())
}

func assertRemoteSuccessRequest(t *testing.T, request factoryapi.FactorySessionExecutionRequest, text string) {
	t.Helper()
	if request.Source.Kind != factoryapi.FactorySessionExecutionSourceKindFactoryId || request.Source.FactoryId == nil || *request.Source.FactoryId != "@you/research" {
		t.Fatalf("remote request source = %#v, want selected Factory ID", request.Source)
	}
	if request.Args == nil || (*request.Args)["prompt"] != text {
		t.Fatalf("remote request args = %#v, want normalized prompt", request.Args)
	}
	if request.RequestId == "" {
		t.Fatal("remote request ID is empty")
	}
}

func assertRemoteSuccessResponse(t *testing.T, payload []byte) {
	t.Helper()
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(payload), &response); err != nil {
		t.Fatalf("decode CLI response: %v; output=%q", err, string(payload))
	}
	if response.SessionId == nil || *response.SessionId != "dur-sess-remote" || response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI response identity/status = (%#v, %q)", response.SessionId, response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("CLI primary result = %#v, want one canonical result part", response.PrimaryResult)
	}
}

type remoteDurableFailureCase struct {
	name          string
	sessionStatus factoryapi.FactorySessionDurableLifecycleStatus
	resultStatus  factoryapi.FactorySessionResultStatus
	availability  *factoryapi.FactorySessionResultAvailabilityDetail
	wantStatus    factoryapi.InvocationTerminalStatus
	wantErrorCode string
	wantMessage   string
}

func TestRunRemoteInvocationMapsCanonicalDurableFailuresToInvocationEnvelope(t *testing.T) {
	for _, test := range remoteDurableFailureCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runRemoteDurableFailureCase(t, test)
		})
	}
}

func TestWriteRemoteInvocationResultPreservesTerminalOutputModes(t *testing.T) {
	completed := apisurface.FactoryInvocationResult{
		RequestID:     "request-output",
		Status:        interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "remote complete"}},
		SessionID:     "session-output",
	}

	t.Run("JSON response stream writes one terminal record", func(t *testing.T) {
		var output bytes.Buffer
		cfg := RunConfig{Output: &output, JSONOutput: true, InvocationOutputMode: InvocationOutputResponseStream}
		if err := writeRemoteInvocationResult(cfg, completed); err != nil {
			t.Fatalf("writeRemoteInvocationResult: %v", err)
		}
		var record remoteInvocationNDJSONRecord
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
			t.Fatalf("decode terminal record: %v; output=%q", err, output.String())
		}
		if record.RecordType != "invocation_result" || record.Response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("terminal record = %#v, want completed invocation_result", record)
		}
	})

	t.Run("human response stream writes primary result", func(t *testing.T) {
		var output bytes.Buffer
		cfg := RunConfig{Output: &output, InvocationOutputMode: InvocationOutputResponseStream}
		if err := writeRemoteInvocationResult(cfg, completed); err != nil {
			t.Fatalf("writeRemoteInvocationResult: %v", err)
		}
		if got := output.String(); got != "remote complete" {
			t.Fatalf("human output = %q, want primary result", got)
		}
	})

	t.Run("human response stream failure keeps safe session context", func(t *testing.T) {
		var output bytes.Buffer
		cfg := RunConfig{Output: &output, InvocationOutputMode: InvocationOutputResponseStream}
		failure := completed
		failure.Status = interfaces.InvocationTerminalStatusFailed
		failure.ErrorCode = "INVOCATION_RUNTIME_FAILURE"
		failure.Message = "server-safe failure"
		if err := writeRemoteInvocationResult(cfg, failure); err == nil {
			t.Fatal("writeRemoteInvocationResult returned nil for terminal failure")
		}
		for _, want := range []string{"--- invocation outcome ---", "error: INVOCATION_RUNTIME_FAILURE", "message: server-safe failure", "session: session-output"} {
			if !strings.Contains(output.String(), want) {
				t.Fatalf("human failure output = %q, want %q", output.String(), want)
			}
		}
	})

	t.Run("nil output is rejected", func(t *testing.T) {
		if err := writeRemoteInvocationResult(RunConfig{InvocationOutputMode: InvocationOutputResponseStream}, completed); err == nil {
			t.Fatal("writeRemoteInvocationResult accepted missing output")
		}
	})
}

func remoteDurableFailureCases() []remoteDurableFailureCase {
	return []remoteDurableFailureCase{
		{
			name:          "failed runtime",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusFailed,
			resultStatus:  factoryapi.FactorySessionResultStatusFailedWithPartial,
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_RUNTIME_FAILURE",
			wantMessage:   "server-safe failure",
		},
		{
			name:          "awaiting approval",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			resultStatus:  factoryapi.FactorySessionResultStatusNotReady,
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_NEEDS_HUMAN",
		},
		{
			name:          "paused",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusPaused,
			resultStatus:  factoryapi.FactorySessionResultStatusPartial,
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_PAUSED",
		},
		{
			name:          "timed out",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusTimedOut,
			resultStatus:  factoryapi.FactorySessionResultStatusUnavailable,
			wantStatus:    factoryapi.InvocationTerminalStatusTimedOut,
			wantErrorCode: "INVOCATION_TIMED_OUT",
		},
		{
			name:          "canceled",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusCanceled,
			resultStatus:  factoryapi.FactorySessionResultStatusUnavailable,
			wantStatus:    factoryapi.InvocationTerminalStatusCanceled,
			wantErrorCode: "INVOCATION_CANCELED",
		},
		{
			name:          "interrupted",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
			resultStatus:  factoryapi.FactorySessionResultStatusUnavailable,
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_INTERRUPTED",
		},
		{
			name:          "blocked availability",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusRunning,
			resultStatus:  factoryapi.FactorySessionResultStatusUnavailable,
			availability: &factoryapi.FactorySessionResultAvailabilityDetail{
				Reason:    stringPtr("BLOCKED"),
				Retryable: boolPtr(false),
			},
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_BLOCKED",
		},
		{
			name:          "unresolved terminal result",
			sessionStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			resultStatus:  factoryapi.FactorySessionResultStatusUnavailable,
			availability: &factoryapi.FactorySessionResultAvailabilityDetail{
				Retryable: boolPtr(false),
			},
			wantStatus:    factoryapi.InvocationTerminalStatusFailed,
			wantErrorCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		},
	}
}

func runRemoteDurableFailureCase(t *testing.T, test remoteDurableFailureCase) {
	const sessionID = "dur-sess-failure"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{
				SessionId: sessionID,
				Status:    factoryapi.FactorySessionDurableLifecycleStatusQueued,
			})
		case http.MethodGet:
			failure := &factoryapi.FailureDetail{Message: "server-safe failure"}
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
				SessionId:     sessionID,
				ResultStatus:  test.resultStatus,
				SessionStatus: durableLifecycleStatusPtr(test.sessionStatus),
				Availability:  test.availability,
				FailureDetail: failure,
			})
		default:
			t.Errorf("method = %s, want POST or GET", r.Method)
		}
	}))
	defer server.Close()

	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	var output bytes.Buffer
	err = RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments("failure input"),
		JSONOutput:              true,
		Output:                  &output,
	}, server.URL, NewRemoteInvocation(transport))
	if err == nil || !strings.Contains(err.Error(), test.wantErrorCode) {
		t.Fatalf("RunRemoteInvocation error = %v, want %s", err, test.wantErrorCode)
	}
	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); decodeErr != nil {
		t.Fatalf("decode failure response: %v; output=%q", decodeErr, output.String())
	}
	if response.Status != test.wantStatus || response.ErrorCode == nil || string(*response.ErrorCode) != test.wantErrorCode {
		t.Fatalf("response = %#v, want status=%s code=%s", response, test.wantStatus, test.wantErrorCode)
	}
	if test.wantMessage != "" && (response.Message == nil || *response.Message != test.wantMessage) {
		t.Fatalf("response message = %#v, want server-safe failure message", response.Message)
	}
}

func TestRunRemoteInvocationResultTransportAndMalformedFailuresStayDistinct(t *testing.T) {
	for _, test := range []struct {
		name          string
		writeResult   func(http.ResponseWriter)
		wantCode      string
		wantBodyValue string
	}{
		{
			name: "transport api failure",
			writeResult: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"code":"UNAVAILABLE","message":"result service unavailable","detail":"secret result payload"}`)
			},
			wantCode:      RemoteDurableResultCode,
			wantBodyValue: "result service unavailable",
		},
		{
			name: "malformed result",
			writeResult: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{SessionId: "dur-sess-malformed"})
			},
			wantCode: RemoteDurableResponseInvalidCode,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			const sessionID = "dur-sess-malformed"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{
						SessionId: sessionID,
						Status:    factoryapi.FactorySessionDurableLifecycleStatusQueued,
					})
					return
				}
				test.writeResult(w)
			}))
			defer server.Close()

			transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
			if err != nil {
				t.Fatalf("NewProtocol: %v", err)
			}
			secret := "do not echo this remote input"
			var output bytes.Buffer
			err = RunRemoteInvocation(context.Background(), RunConfig{
				Dir:                     "factory",
				NamedFactoryName:        "@you/research",
				PreparedInvocationInput: preparedRemoteArguments(secret),
				JSONOutput:              true,
				Output:                  &output,
			}, server.URL, NewRemoteInvocation(transport))
			if err == nil || !strings.Contains(err.Error(), test.wantCode) {
				t.Fatalf("RunRemoteInvocation error = %v, want %s", err, test.wantCode)
			}
			if !strings.Contains(err.Error(), server.URL) {
				t.Fatalf("error = %q, want safe selected endpoint", err)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "secret result payload") {
				t.Fatalf("error leaked request or response payload: %q", err)
			}
			if test.wantBodyValue != "" && !strings.Contains(err.Error(), test.wantBodyValue) {
				t.Fatalf("error = %q, want customer-safe API message", err)
			}
			if output.Len() != 0 {
				t.Fatalf("stdout = %q, want no terminal success or failure record", output.String())
			}
		})
	}
}

func TestRunRemoteInvocationPassesPreparedArguments(t *testing.T) {
	text := "local adapter must not run"
	var got RemoteInvocationRequest
	remote := remoteInvocationOperationFunc(func(_ context.Context, request RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		got = request
		return factoryapi.FactorySessionExecutionResponse{
			SessionId: "dur-sess-arguments", Status: factoryapi.FactorySessionDurableLifecycleStatusRunning,
		}, nil
	})
	var output bytes.Buffer
	err := RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments(text),
		Output:                  &output,
	}, "http://selected.test", remote)
	if err != nil {
		t.Fatalf("RunRemoteInvocation: %v", err)
	}
	if got.Server != "http://selected.test" {
		t.Fatalf("remote server = %q, want selected server", got.Server)
	}
	if got.Request.Args == nil || (*got.Request.Args)["prompt"] != text {
		t.Fatalf("remote normalized arguments = %#v, want prompt=%q", got.Request.Args, text)
	}
	if got.Request.RequestId == "" {
		t.Fatal("remote request identity is empty")
	}
	if output.String() != "Factory session dur-sess-arguments accepted (RUNNING).\n" {
		t.Fatalf("stdout = %q, want durable acceptance", output.String())
	}
}

func TestRemoteDurableRequestMapsPolicyAndWait(t *testing.T) {

	timeout := int64(1750)
	requestID := "caller-request-1"
	args := map[string]any{"prompt": "ship it", "count": 2}
	definition := &interfaces.FactoryConfig{
		Name: "remote-inline",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				DefaultPolicy: json.RawMessage(`{"mode":"READ_ONLY"}`),
			},
		},
	}
	load := func(string) (*interfaces.FactoryConfig, error) { return definition, nil }
	cfg := RunConfig{
		FactoryConfigPath:     "remote-factory.json",
		LoadFactoryConfigFile: load,
	}
	first, err := remoteDurableRequestFromRunConfig(cfg, factoryapi.InvocationRequest{
		Args:          &args,
		RequestId:     &requestID,
		TimeoutMillis: &timeout,
	})
	if err != nil {
		t.Fatalf("remoteDurableRequestFromRunConfig: %v", err)
	}
	if first.Source.Kind != factoryapi.FactorySessionExecutionSourceKindFactoryInline || first.Source.FactoryInline == nil {
		t.Fatalf("source = %#v, want inline Factory source", first.Source)
	}
	if first.Args == nil || (*first.Args)["prompt"] != "ship it" || (*first.Args)["count"] != 2 {
		t.Fatalf("args = %#v, want normalized values", first.Args)
	}
	if first.RequestedPolicy == nil || first.RequestedPolicy.AdditionalProperties["mode"] != "READ_ONLY" {
		t.Fatalf("requested policy = %#v, want authored policy", first.RequestedPolicy)
	}
	if first.Wait == nil || first.Wait.TimeoutMillis == nil || *first.Wait.TimeoutMillis != timeout {
		t.Fatalf("wait = %#v, want timeout %d", first.Wait, timeout)
	}
	if first.RequestId != requestID {
		t.Fatalf("request ID = %q, want caller identity", first.RequestId)
	}
}

func TestRemoteDurableRequestDerivesStableIdentity(t *testing.T) {
	args := map[string]any{"prompt": "ship it"}
	cfg := RunConfig{
		FactoryConfigPath: "remote-factory.json",
		LoadFactoryConfigFile: func(string) (*interfaces.FactoryConfig, error) {
			return &interfaces.FactoryConfig{Name: "remote-inline"}, nil
		},
	}
	first, err := remoteDurableRequestFromRunConfig(cfg, factoryapi.InvocationRequest{Args: &args})
	if err != nil {
		t.Fatalf("remoteDurableRequestFromRunConfig retry: %v", err)
	}
	second, err := remoteDurableRequestFromRunConfig(cfg, factoryapi.InvocationRequest{Args: &args})
	if err != nil {
		t.Fatalf("remoteDurableRequestFromRunConfig second retry: %v", err)
	}
	if first.RequestId == "" || first.RequestId != second.RequestId {
		t.Fatalf("derived request IDs = %q/%q, want stable retry identity", first.RequestId, second.RequestId)
	}
}

func TestRemoteInvocationFailureDoesNotLeakRequestOrRetryLocally(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"code":"UNAVAILABLE","message":"remote service unavailable","detail":"sensitive request"}`)
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	secret := "do not echo this payload"
	err = RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments(secret),
		Output:                  io.Discard,
	}, server.URL, NewRemoteInvocation(transport))
	if err == nil {
		t.Fatal("RunRemoteInvocation error = nil, want remote failure")
	}
	if !strings.Contains(err.Error(), server.URL) || !strings.Contains(err.Error(), "remote service unavailable") {
		t.Fatalf("error = %q, want selected endpoint and stable remote message", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "sensitive request") {
		t.Fatalf("error leaked sensitive request data: %q", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("remote failure unexpectedly mapped to cancellation: %v", err)
	}
}

func TestRemoteInvocationClientRejectsInvalidInputsAndResponses(t *testing.T) {
	t.Run("context is required", testRemoteInvocationRequiresContext)
	t.Run("protocol is required", testRemoteInvocationRequiresProtocol)
	t.Run("endpoint must be valid", testRemoteInvocationRejectsInvalidEndpoint)
	t.Run("request must be JSON encodable", testRemoteInvocationRejectsUnencodableRequest)
	t.Run("nil HTTP response is rejected", testRemoteInvocationRejectsMissingResponse)
	t.Run("non API error status remains actionable", testRemoteInvocationReportsNonAPIError)
	t.Run("successful response requires a durable identity", testRemoteInvocationRejectsEmptySuccessBody)
}

func TestRemoteExistingSessionInvocationPreservesContextOutcome(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus factoryapi.InvocationTerminalStatus
		wantCode   factoryapi.InvocationResponseErrorCode
	}{
		{
			name:       "deadline",
			err:        fmt.Errorf("execute request: %w", context.DeadlineExceeded),
			wantStatus: factoryapi.InvocationTerminalStatusTimedOut,
			wantCode:   factoryapi.INVOCATIONTIMEDOUT,
		},
		{
			name:       "cancellation",
			err:        fmt.Errorf("execute request: %w", context.Canceled),
			wantStatus: factoryapi.InvocationTerminalStatusCanceled,
			wantCode:   factoryapi.INVOCATIONCANCELED,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestID := "request-context-outcome"
			response, err := (remoteInvocationClient{transport: &remoteProtocolStub{err: tt.err}}).InvokeFactorySession(
				context.Background(),
				RemoteExistingSessionInvocationRequest{
					Server:    "http://selected.test",
					SessionID: "session-context-outcome",
					Request:   factoryapi.InvocationRequest{RequestId: &requestID},
				},
			)
			if err != nil {
				t.Fatalf("InvokeFactorySession: %v", err)
			}
			if response.Status != tt.wantStatus || response.ErrorCode == nil || *response.ErrorCode != tt.wantCode {
				t.Fatalf("response = %#v, want status %q and code %q", response, tt.wantStatus, tt.wantCode)
			}
			if response.SessionId == nil || *response.SessionId != "session-context-outcome" ||
				response.RequestId != requestID {
				t.Fatalf("response identity = %#v, want session and request identity preserved", response)
			}
		})
	}
}

func testRemoteInvocationRequiresContext(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.StartFactorySession(nil, RemoteInvocationRequest{})
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("error = %v, want required-context error", err)
	}
}

func testRemoteInvocationRequiresProtocol(t *testing.T) {
	_, err := remoteInvocationClient{}.StartFactorySession(context.Background(), RemoteInvocationRequest{})
	if err == nil || !strings.Contains(err.Error(), "CLI HTTP protocol is required") {
		t.Fatalf("error = %v, want required-protocol error", err)
	}
}

func testRemoteInvocationRejectsInvalidEndpoint(t *testing.T) {
	stub := &remoteProtocolStub{}
	_, err := remoteInvocationClient{transport: stub}.StartFactorySession(context.Background(), RemoteInvocationRequest{
		Server: "http://[::1",
	})
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("error = %v, want invalid-endpoint error", err)
	}
	if stub.called {
		t.Fatal("invalid endpoint called the HTTP protocol")
	}
}

func testRemoteInvocationRejectsUnencodableRequest(t *testing.T) {
	bad := map[string]any{"function": func() {}}
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.StartFactorySession(context.Background(), RemoteInvocationRequest{
		Server:  "http://selected.test",
		Request: factoryapi.FactorySessionExecutionRequest{Args: &bad},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal remote durable start request") {
		t.Fatalf("error = %v, want marshal error", err)
	}
}

func testRemoteInvocationRejectsMissingResponse(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{
		response: clihttp.Response{},
	}}.StartFactorySession(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
	if err == nil || !strings.Contains(err.Error(), "HTTP response is unavailable") {
		t.Fatalf("error = %v, want missing-response error", err)
	}
}

func testRemoteInvocationReportsNonAPIError(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{
		response: clihttp.Response{HTTP: &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("not an API error")),
		}},
	}}.StartFactorySession(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
	if err == nil || !strings.Contains(err.Error(), "(502)") {
		t.Fatalf("error = %v, want HTTP status error", err)
	}
}

func testRemoteInvocationRejectsEmptySuccessBody(t *testing.T) {
	stub := &remoteProtocolStub{response: clihttp.Response{HTTP: &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader("")),
	}}}
	response, err := remoteInvocationClient{transport: stub}.StartFactorySession(context.Background(), RemoteInvocationRequest{
		Server: "https://selected.test",
	})
	if err == nil {
		t.Fatalf("StartFactorySession: nil error, want malformed response")
	}
	if response.SessionId != "" || !strings.Contains(err.Error(), RemoteDurableResponseInvalidCode) {
		t.Fatalf("response/error = %#v/%v, want malformed durable response", response, err)
	}
	if !strings.HasSuffix(stub.url, "/factory-sessions/async") {
		t.Fatalf("request URL = %q, want durable start endpoint", stub.url)
	}
}

func TestRunRemoteInvocationReportsDurableInputAndResponseErrors(t *testing.T) {
	t.Run("nil operation is rejected", func(t *testing.T) {
		err := RunRemoteInvocation(context.Background(), RunConfig{}, "", nil)
		if err == nil || !strings.Contains(err.Error(), "operation is required") {
			t.Fatalf("error = %v, want required-operation error", err)
		}
	})

	t.Run("missing invocation input has stable code", func(t *testing.T) {
		err := RunRemoteInvocation(context.Background(), RunConfig{Dir: "factory"}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
			t.Fatal("remote operation called without invocation input")
			return factoryapi.FactorySessionExecutionResponse{}, nil
		}))
		var invocationErr *InvocationError
		if !errors.As(err, &invocationErr) || invocationErr.Code != RemoteInvocationInputRequiredCode {
			t.Fatalf("error = %v, want %s", err, RemoteInvocationInputRequiredCode)
		}

	})

	t.Run("compatibility content is rejected", func(t *testing.T) {
		text := "remote compatibility content"
		err := RunRemoteInvocation(context.Background(), RunConfig{
			Dir: "factory", NamedFactoryName: "@you/research", InvocationPositionalText: &text,
		}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
			return factoryapi.FactorySessionExecutionResponse{}, nil
		}))
		var invocationErr *InvocationError
		if !errors.As(err, &invocationErr) || invocationErr.Code != RemoteDurableRequestInvalidCode {
			t.Fatalf("error = %v, want %s", err, RemoteDurableRequestInvalidCode)
		}
	})

	t.Run("missing durable identity is rejected", func(t *testing.T) {
		text := "remote normalized input"
		err := RunRemoteInvocation(context.Background(), RunConfig{
			Dir: "factory", NamedFactoryName: "@you/research", PreparedInvocationInput: preparedRemoteArguments(text),
		}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
			return factoryapi.FactorySessionExecutionResponse{}, nil
		}))
		var invocationErr *InvocationError
		if !errors.As(err, &invocationErr) || invocationErr.Code != RemoteDurableResponseInvalidCode {
			t.Fatalf("error = %v, want %s", err, RemoteDurableResponseInvalidCode)
		}
	})
}
