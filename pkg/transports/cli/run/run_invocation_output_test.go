package run

import (
	"bytes"
	"context"
	"encoding/json"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
					TraceID:   "trace-123",
					Status:    interfaces.InvocationTerminalStatusCompleted,
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
