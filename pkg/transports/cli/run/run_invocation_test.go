// backendsizecheck:ignore-file consolidated run invocation tests remain together until dedicated CLI invocation test seams split.
// pkgmaintcheck:ignore-file-lines consolidated run invocation tests remain together until dedicated CLI invocation test seams split.
package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/packages/subagent"
	"github.com/portpowered/infinite-you/pkg/factory/packages/tts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type stubInvocationService struct {
	run    func(context.Context) error
	invoke func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error)
	close  func(context.Context, string) error
}

func TestBuildApplication_ConstructsInvocationBootstrapOnceBeforeInitializer(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	buildCalls := 0
	lifecycleStarted := false
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		buildCalls++
		if lifecycleStarted {
			t.Fatal("invocation bootstrap constructed after lifecycle start")
		}
		return stubInvocationService{
			run: func(ctx context.Context) error {
				lifecycleStarted = true
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	application, err := BuildApplication(context.Background(), RunConfig{
		Dir:                      t.TempDir(),
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		DisableDefaultRecording:  true,
	}, nil, buildInvocationBootstrap)
	if err != nil {
		t.Fatalf("BuildApplication() error = %v", err)
	}
	if buildCalls != 1 || lifecycleStarted {
		t.Fatalf("after construction: build calls = %d, lifecycle started = %t; want 1, false", buildCalls, lifecycleStarted)
	}

	err = initializer.RunProcess(context.Background(), &initializer.ProcessGraph{
		Policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}},
		Run:    application,
	})
	if err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
	if buildCalls != 1 || !lifecycleStarted {
		t.Fatalf("after initialization: build calls = %d, lifecycle started = %t; want 1, true", buildCalls, lifecycleStarted)
	}
}

func (s stubInvocationService) Run(ctx context.Context) error {
	return s.run(ctx)
}

func (s stubInvocationService) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{Name: "portable"}, nil
}

func (s stubInvocationService) InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return s.invoke(ctx, sessionID, request)
}

func (s stubInvocationService) CloseFactorySession(ctx context.Context, sessionID string) error {
	if s.close != nil {
		return s.close(ctx, sessionID)
	}
	return nil
}

func TestBuildInvocationRunServiceConfig_ForcesNoServerBootstrapShape(t *testing.T) {
	t.Parallel()

	cfg := buildInvocationRunServiceConfig(RunConfig{
		Dir:  "/tmp/factory",
		Port: 7437,
	}, zap.NewNop(), nil)
	if cfg.Port != 0 {
		t.Fatalf("Port = %d, want 0", cfg.Port)
	}
	if cfg.APIServerStarter != nil {
		t.Fatal("APIServerStarter = non-nil, want nil")
	}
	if cfg.APIServerReady != nil {
		t.Fatal("APIServerReady = non-nil, want nil")
	}
	if cfg.SimpleDashboardRenderer != nil {
		t.Fatal("SimpleDashboardRenderer = non-nil, want nil")
	}
	if cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("RuntimeMode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if cfg.WorkFile != "" {
		t.Fatalf("WorkFile = %q, want empty", cfg.WorkFile)
	}
}

func TestRun_FactoryInvocationUsesNoServerBootstrapConfig(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var captured *service.FactoryServiceConfig
	buildInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		cloned := *cfg
		captured = &cloned
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured == nil {
		t.Fatal("expected factory invocation bootstrap config capture")
	}
	if captured.Port != 0 {
		t.Fatalf("captured Port = %d, want 0", captured.Port)
	}
	if captured.APIServerStarter != nil {
		t.Fatal("captured APIServerStarter = non-nil, want nil")
	}
}

func TestRun_FactoryInvocationReleasesSessionThroughFactoryServiceOwnership(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var closedSessionID string
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
			close: func(_ context.Context, sessionID string) error {
				closedSessionID = sessionID
				return nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if closedSessionID == "" {
		t.Fatal("expected CloseFactorySession through bootstrap ownership path")
	}
}

func TestResolveFactoryInvocationRequest_NamedFactoryDirPositionalText(t *testing.T) {
	text := "hi there"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for named factory positional text")
	}
	if got := extractInvocationText(t, request); got != text {
		t.Fatalf("invocation text = %q, want %q", got, text)
	}
}

func TestResolveFactoryInvocationRequest_PositionalText(t *testing.T) {
	text := "Fix the lint issues"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for positional text")
	}
	if got := extractInvocationText(t, request); got != text {
		t.Fatalf("invocation text = %q, want %q", got, text)
	}
}

func TestResolveFactoryInvocationRequest_StdinText(t *testing.T) {
	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath: "/tmp/factory.json",
		Stdin:             strings.NewReader("from stdin"),
		StdinIsTTY:        func() bool { return false },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for stdin text")
	}
	if got := extractInvocationText(t, request); got != "from stdin" {
		t.Fatalf("invocation text = %q, want stdin text", got)
	}
}

func TestResolveFactoryInvocationRequest_NamedFactoryStdinText(t *testing.T) {
	stdinText := "hi from stdin"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir:                 "/tmp/builtin-tts",
		NamedFactoryName:    "@you/tts",
		InvocationStdinText: &stdinText,
		StdinIsTTY:          func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for named factory stdin text")
	}
	if got := extractInvocationText(t, request); got != stdinText {
		t.Fatalf("invocation text = %q, want %q", got, stdinText)
	}
}

func TestResolveFactoryInvocationRequest_UsesNormalizedSignatureArgs(t *testing.T) {
	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir: "/tmp/signature-factory",
		InvocationNormalizedArguments: &invocations.NormalizedArguments{
			Arguments: map[string]invocations.NormalizedArgument{
				"input": {Values: []string{"draft"}},
				"mode":  {Values: []string{"fast", "review"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for normalized signature args")
	}
	if request == nil || request.Args == nil {
		t.Fatalf("request = %#v, want args request", request)
	}
	if got := (*request.Args)["input"]; got != "draft" {
		t.Fatalf("args[input] = %#v, want %q", got, "draft")
	}
	values, ok := (*request.Args)["mode"].([]string)
	if !ok {
		t.Fatalf("args[mode] = %#v, want []string", (*request.Args)["mode"])
	}
	if len(values) != 2 || values[0] != "fast" || values[1] != "review" {
		t.Fatalf("args[mode] = %#v, want [fast review]", values)
	}
}

func TestResolveFactoryInvocationRequest_NamedFactoryRejectsConflictingSources(t *testing.T) {
	text := "from args"

	_, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		Stdin:                    strings.NewReader("from stdin"),
		StdinIsTTY:               func() bool { return false },
	})
	if !invocationMode {
		t.Fatal("expected invocation mode when both sources are present for named factory")
	}
	if err == nil {
		t.Fatal("expected conflicting invocation sources to fail for named factory")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("error = %q, want stable conflict code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_RejectsWhitespaceOnlyPositional(t *testing.T) {
	text := "   "

	_, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
	})
	if !invocationMode {
		t.Fatal("expected invocation mode for whitespace-only positional text")
	}
	if err == nil {
		t.Fatal("expected whitespace-only positional rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_RejectsConflictingSources(t *testing.T) {
	text := "from args"

	_, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		Stdin:                    strings.NewReader("from stdin"),
		StdinIsTTY:               func() bool { return false },
	})
	if !invocationMode {
		t.Fatal("expected invocation mode when both sources are present")
	}
	if err == nil {
		t.Fatal("expected conflicting invocation sources to fail")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("error = %q, want stable conflict code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_ConflictLogsAndCountsSourceConflict(t *testing.T) {
	text := "from args"
	core, observedLogs := observer.New(zap.InfoLevel)
	recorder := &capturingInvocationMetricsRecorder{}

	_, _, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:         "/tmp/factory.json",
		InvocationPositionalText:  &text,
		Stdin:                     strings.NewReader("from stdin"),
		StdinIsTTY:                func() bool { return false },
		Logger:                    zap.New(core),
		InvocationMetricsRecorder: recorder,
	})
	if err == nil {
		t.Fatal("expected conflicting invocation sources to fail")
	}

	entries := observedLogs.FilterMessage("factory invocation input resolution failed").All()
	if len(entries) != 1 {
		t.Fatalf("conflict log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields["failure_class"]; got != "source_conflict" {
		t.Fatalf("failure_class = %#v, want source_conflict", got)
	}
	if got := fields["error_code"]; got != "INVOCATION_INPUT_SOURCE_CONFLICT" {
		t.Fatalf("error_code = %#v, want INVOCATION_INPUT_SOURCE_CONFLICT", got)
	}
	if _, ok := fields["conflicting_sources"]; !ok {
		t.Fatal("expected conflicting_sources field in conflict log")
	}

	recorder.assertContainsMetricNames(t, "invocation.failure", "invocation.source_conflict")
}

func TestRun_NamedFactoryModelNotReadyKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "hi there"
	var output bytes.Buffer
	core, observedLogs := observer.New(zap.InfoLevel)

	buildInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
					ErrorCode: tts.InvocationErrorCodeModelNotReady,
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
	if !strings.Contains(err.Error(), tts.InvocationErrorCodeModelNotReady) {
		t.Fatalf("error = %q, want %s", err.Error(), tts.InvocationErrorCodeModelNotReady)
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

	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
					ErrorCode: tts.InvocationErrorCodeGenerationFailed,
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
	if !strings.Contains(err.Error(), tts.InvocationErrorCodeGenerationFailed) {
		t.Fatalf("error = %q, want %s", err.Error(), tts.InvocationErrorCodeGenerationFailed)
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

	buildInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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

	var metadata tts.InvocationMetadata
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
	var captured *service.FactoryServiceConfig

	buildInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		captured = cfg
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
	if captured.SimpleDashboardRenderer != nil {
		t.Fatal("expected invocation mode to suppress dashboard rendering")
	}
}

func TestRun_FactoryInvocationFailureKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "Fix the lint issues"
	var output bytes.Buffer

	buildInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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

const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"

func TestResolveFactoryInvocationRequest_NamedGoalInputSourcesMatchSharedResolver(t *testing.T) {
	planSprint := "Plan the sprint"
	stdinText := "Ship the feature from stdin"

	tests := []struct {
		name       string
		cfg        RunConfig
		wantSource invocations.InputSourceLabel
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
			wantSource: invocations.InputSourcePositionalText,
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
			wantSource: invocations.InputSourceStdinText,
			wantText:   stdinText,
		},
		{
			name: "piped non-tty stdin",
			cfg: RunConfig{
				Dir:              "/tmp/builtin-goal",
				NamedFactoryName: goal.PackagedFactoryName,
				Stdin:            strings.NewReader("Ship from pipe\n"),
				StdinIsTTY:       func() bool { return false },
			},
			wantSource: invocations.InputSourceStdinText,
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
		wantSource invocations.InputSourceLabel
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
			wantSource: invocations.InputSourcePositionalText,
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
			wantSource: invocations.InputSourceStdinText,
			wantText:   "Ship the feature from explicit stdin",
			wantOutput: "goal stdin completed",
		},
		{
			name: "piped stdin",
			cfg: RunConfig{
				Dir:              "/tmp/builtin-goal",
				NamedFactoryName: goal.PackagedFactoryName,
				Stdin:            strings.NewReader("Ship from pipe\n"),
				StdinIsTTY:       func() bool { return false },
			},
			wantSource: invocations.InputSourceStdinText,
			wantText:   "Ship from pipe\n",
			wantOutput: "goal pipe completed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			preserveRunGlobals(t)

			var output bytes.Buffer
			buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
	text := "Plan from args"

	tests := []struct {
		name string
		cfg  RunConfig
	}{
		{
			name: "positional text with piped non-tty stdin",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: &text,
				Stdin:                    strings.NewReader("Plan from stdin\n"),
				StdinIsTTY:               func() bool { return false },
			},
		},
		{
			name: "positional text with explicit stdin text",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: &text,
				InvocationStdinText:      stringPtr("Plan from explicit stdin"),
				StdinIsTTY:               func() bool { return true },
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, invocationMode, err := resolveFactoryInvocationRequest(tc.cfg)
			if !invocationMode {
				t.Fatal("expected invocation mode when both sources are present for named goal")
			}
			assertStableSourceConflictError(t, err)
		})
	}
}

func TestRun_NamedGoalConflictingSourcesFailsBeforeInvocation(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan from args"
	var output bytes.Buffer
	invokeCalled := false

	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				invokeCalled = true
				t.Fatal("expected conflicting goal invocation sources to fail before InvokeFactorySession")
				return apisurface.FactoryInvocationResult{}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: &text,
		Stdin:                    strings.NewReader("Plan from stdin\n"),
		StdinIsTTY:               func() bool { return false },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected conflicting goal invocation sources to fail")
	}
	assertStableSourceConflictError(t, err)
	if invokeCalled {
		t.Fatal("expected InvokeFactorySession to stay uncalled for conflicting goal sources")
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on conflicting-source failure", output.String())
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
				Dir:              "/tmp/builtin-goal",
				NamedFactoryName: goal.PackagedFactoryName,
				Stdin:            strings.NewReader(stdinText),
				StdinIsTTY:       func() bool { return false },
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

	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
			InvocationNormalizedArguments: &invocations.NormalizedArguments{Arguments: map[string]invocations.NormalizedArgument{
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
		cliArgs, err := invocations.NamedArgumentInputsFromAnyMap(*cliRequest.Args)
		if err != nil {
			t.Fatalf("normalize CLI args: %v", err)
		}
		apiArgs, err := invocations.NamedArgumentInputsFromAnyMap(*apiRequest.Args)
		if err != nil {
			t.Fatalf("normalize API args: %v", err)
		}
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

	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
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
	text := "Plan from args"
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	cliCfg := RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: &text,
		Stdin:                    strings.NewReader("Plan from stdin\n"),
		StdinIsTTY:               func() bool { return false },
	}
	_, _, cliErr := resolveFactoryInvocationRequest(cliCfg)
	assertStableSourceConflictError(t, cliErr)
	assertStableInvocationSourceConflictMessage(t, cliErr.Error(), conflictMessage)

	apiErr := &invocations.InputError{
		Code:    invocations.InputErrorCodeSourceConflict,
		Message: conflictMessage,
	}
	assertStableInvocationSourceConflictMessage(t, apiErr.Error(), conflictMessage)
}

func extractInvocationText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.Content == nil {
		t.Fatal("content = nil, want one text part")
	}
	parts := *request.Content
	if len(parts) != 1 {
		t.Fatalf("content parts = %d, want 1", len(parts))
	}
	part, err := parts[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("AsWorkTextContentPart: %v", err)
	}
	return part.Text
}

type capturingInvocationMetricsRecorder struct {
	metrics []service.InvocationMetric
}

func (r *capturingInvocationMetricsRecorder) RecordInvocationMetric(metric service.InvocationMetric) {
	r.metrics = append(r.metrics, metric)
}

type capturingBootstrapRunner struct {
	inner       sessionInvocationRunner
	lastRequest *factoryapi.InvocationRequest
	lastResult  *apisurface.FactoryInvocationResult
}

func (c *capturingBootstrapRunner) Run(ctx context.Context) error {
	return c.inner.Run(ctx)
}

func (c *capturingBootstrapRunner) GetCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factoryapi.Factory, error) {
	return c.inner.GetCurrentFactoryForSession(ctx, sessionID)
}

func (c *capturingBootstrapRunner) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	c.lastRequest = cloneInvocationRequestForCapture(request)
	result, err := c.inner.InvokeFactorySession(ctx, sessionID, request)
	if err == nil {
		captured := result
		c.lastResult = &captured
	}
	return result, err
}

func (c *capturingBootstrapRunner) CloseFactorySession(ctx context.Context, sessionID string) error {
	return c.inner.CloseFactorySession(ctx, sessionID)
}

func (c *capturingBootstrapRunner) SubscribeSessionResponseEventsFromLatest(
	sessionID string,
) (*responseeventstore.Subscription, error) {
	attachable, ok := c.inner.(sessionResponseEventAttachable)
	if !ok {
		return nil, fmt.Errorf("captured invocation bootstrap does not expose canonical response events")
	}
	return attachable.SubscribeSessionResponseEventsFromLatest(sessionID)
}

func cloneInvocationRequestForCapture(request factoryapi.InvocationRequest) *factoryapi.InvocationRequest {
	data, err := json.Marshal(request)
	if err != nil {
		panic(fmt.Sprintf("marshal invocation request for capture: %v", err))
	}
	var cloned factoryapi.InvocationRequest
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic(fmt.Sprintf("unmarshal invocation request for capture: %v", err))
	}
	return &cloned
}

func installCapturingRealInvocationBootstrap(t *testing.T) *capturingBootstrapRunner {
	t.Helper()

	capture := &capturingBootstrapRunner{}
	buildInvocationBootstrap = func(ctx context.Context, cfg *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		svc, err := service.BuildFactoryService(ctx, service.NormalizeInvocationBootstrapConfig(cfg))
		if err != nil {
			return nil, err
		}
		inner, err := service.NewInvocationBootstrap(svc)
		if err != nil {
			return nil, err
		}
		capture.inner = inner
		return capture, nil
	}
	return capture
}

func namedGoalNoServerInvocationRunConfig(t *testing.T, goalText string) RunConfig {
	t.Helper()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, goal.PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}

	return RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           goal.PackagedFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &goalText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      writePackagedGoalNoServerMockWorkersConfig(t),
		DisableDefaultRecording:    true,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	}
}

func runNoServerBootstrapEquivalenceCase(
	t *testing.T,
	goalText string,
	mutate func(*RunConfig),
) (*capturingBootstrapRunner, *bytes.Buffer) {
	t.Helper()

	preserveRunGlobals(t)
	prohibitInvocationAPIServer(t)
	capture := installCapturingRealInvocationBootstrap(t)
	cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
	if mutate != nil {
		mutate(&cfg)
	}
	var output bytes.Buffer
	cfg.Output = &output

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return capture, &output
}

func assertCapturedRequestMatchesLogicalAPIText(t *testing.T, capture *capturingBootstrapRunner, goalText string) {
	t.Helper()

	apiRequest, err := invocationRequestFromLogicalAPIText(goalText)
	if err != nil {
		t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("expected InvokeFactorySession request capture on real no-server bootstrap")
	}
	assertEquivalentInvocationRequests(t, capture.lastRequest, apiRequest)
}

func assertCapturedResultMatchesCLIJSONOutput(t *testing.T, capture *capturingBootstrapRunner, output *bytes.Buffer) {
	t.Helper()

	if capture.lastResult == nil {
		t.Fatal("expected InvokeFactorySession result capture on real no-server bootstrap")
	}
	if capture.lastResult.Status != interfaces.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want %q", capture.lastResult.Status, interfaces.InvocationTerminalStatusCompleted)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\n%s", err, output.String())
	}
	apiResponse := apisurface.InvocationResponseFromResult(*capture.lastResult)
	if !reflect.DeepEqual(cliResponse, apiResponse) {
		t.Fatalf("CLI response = %#v, API projection = %#v", cliResponse, apiResponse)
	}
	assertInvocationResponseMatchesFactoryResult(t, cliResponse, *capture.lastResult)
}

func TestRun_NoServerBootstrap_PositionalInputMatchesAPIContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap positional parity prompt"
	capture, _ := runNoServerBootstrapEquivalenceCase(t, goalText, nil)
	assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
}

func TestRun_NoServerBootstrap_StdinInputMatchesAPIContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap stdin parity prompt"
	capture, _ := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
		cfg.InvocationPositionalText = nil
		cfg.Stdin = strings.NewReader(goalText)
		cfg.StdinIsTTY = func() bool { return false }
	})
	assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
}

func TestRun_NoServerBootstrap_SuccessJSONMatchesAPIProjection(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap json parity prompt"
	capture, output := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
		cfg.JSONOutput = true
	})
	assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
}

func TestRun_NoServerBootstrap_ResponseStreamJSONEmitsCanonicalEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for canonical response events through real no-server bootstrap")
	}

	goalText := "no-server bootstrap canonical response-event prompt"
	_, output := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
		cfg.JSONOutput = true
		cfg.InvocationOutputMode = InvocationOutputResponseStream
	})

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("NDJSON lines = %d, want at least one response event and one invocation result:\n%s", len(lines), output.String())
	}
	for index, line := range lines[:len(lines)-1] {
		var record responseStreamJSONResponseEventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode response event line %d: %v", index, err)
		}
		if record.RecordType != responseStreamJSONRecordResponseEvent {
			t.Fatalf("record type at line %d = %q, want %q", index, record.RecordType, responseStreamJSONRecordResponseEvent)
		}
	}
	var finalRecord responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
		t.Fatalf("decode final invocation result: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordInvocationResult {
		t.Fatalf("final record type = %q, want %q", finalRecord.RecordType, responseStreamJSONRecordInvocationResult)
	}
}

func TestRun_NoServerBootstrap_TextPrimaryResultFollowsInvocationReturn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap primary-result prompt"
	_, output := runNoServerBootstrapEquivalenceCase(t, goalText, nil)
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want packaged goal invocationReturn primary result", got)
	}
	if got := output.String(); got == goalText {
		t.Fatalf("stdout echoed submitted goal text instead of invocationReturn primary result")
	}
}

func TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named goal no-server invocation")
	}

	preserveRunGlobals(t)
	prohibitInvocationAPIServer(t)

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, goal.PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}

	mockWorkersPath := writePackagedGoalNoServerMockWorkersConfig(t)
	goalText := "hermetic no-server named goal prompt"

	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	err = Run(ctx, RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           goal.PackagedFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &goalText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      mockWorkersPath,
		DisableDefaultRecording:    true,
		Output:                     &output,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary result mock worker accepted", got)
	}
}

func TestRun_NamedGoalRepositoryEditReturnsFinalResponseAsPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic packaged goal repository edit")
	}

	preserveRunGlobals(t)
	prohibitInvocationAPIServer(t)

	repositoryDir := t.TempDir()
	readmePath := filepath.Join(repositoryDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write repository fixture: %v", err)
	}

	goalText := "Update README.md in the repository fixture"
	cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
	cfg.MockWorkersConfigPath = writePackagedGoalRepositoryEditMockWorkersConfig(t, repositoryDir)
	var output bytes.Buffer
	cfg.Output = &output

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	const finalResponse = "Updated README.md with the completed repository edit."
	if got := output.String(); got != finalResponse {
		t.Fatalf("stdout = %q, want final agent response %q", got, finalResponse)
	}
	edited, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read edited repository fixture: %v", err)
	}
	if got := string(edited); got != "after\n" {
		t.Fatalf("README.md = %q, want repository edit", got)
	}
}

func TestRun_NamedGoalRepositoryEditWorkerProcess(t *testing.T) {
	repositoryDir := os.Getenv("YOU_TEST_GOAL_REPOSITORY_DIR")
	if repositoryDir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(repositoryDir, "README.md"), []byte("after\n"), 0o644); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "edit repository fixture: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Updated README.md with the completed repository edit.")
	_, _ = fmt.Fprintln(os.Stdout, "<COMPLETE>")
	os.Exit(0)
}

func writePackagedGoalNoServerMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      "goal-executor",
				WorkstationName: goal.PackagedExecuteWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
		},
	}
	return writeMockWorkersConfig(t, cfg, "mock-workers-packaged-goal-no-server.json")
}

func writePackagedGoalRepositoryEditMockWorkersConfig(t *testing.T, repositoryDir string) string {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cfg := factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: goal.PackagedExecuteWorkstationName,
			RunType:         factoryconfig.MockWorkerRunTypeScript,
			ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
				Command:          executable,
				Args:             []string{"-test.run=^TestRun_NamedGoalRepositoryEditWorkerProcess$"},
				Env:              map[string]string{"YOU_TEST_GOAL_REPOSITORY_DIR": repositoryDir},
				WorkingDirectory: repositoryDir,
				Timeout:          "10s",
			},
		}},
	}
	return writeMockWorkersConfig(t, cfg, "mock-workers-packaged-goal-repository-edit.json")
}

func writeMockWorkersConfig(t *testing.T, cfg factoryconfig.MockWorkersConfig, name string) string {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

const noServerInvocationTestPort = 38317

func prohibitInvocationAPIServer(t *testing.T) {
	t.Helper()

	startAPIServer = func(
		context.Context,
		apisurface.APISurface,
		int,
		*zap.Logger,
		func(),
	) error {
		return errors.New("invocation attempted to start an API server")
	}
	serveFactoryAPIServer = func(
		context.Context,
		apisurface.APISurface,
		int,
		*zap.Logger,
		net.Listener,
	) error {
		return errors.New("invocation attempted to serve a reserved API listener")
	}
}

func (r *capturingInvocationMetricsRecorder) assertContainsMetricNames(t *testing.T, want ...string) {
	t.Helper()

	if len(r.metrics) != len(want) {
		t.Fatalf("metric count = %d, want %d (%#v)", len(r.metrics), len(want), r.metrics)
	}
	got := make(map[string]int, len(r.metrics))
	for _, metric := range r.metrics {
		got[metric.Name]++
	}
	for _, name := range want {
		if got[name] == 0 {
			t.Fatalf("metrics = %#v, want to include %q", r.metrics, name)
		}
	}
}

// TestNoServerNamedInvocationIntegrationAndEquivalenceProof is the consolidated
// package integration and invocation-equivalence proof for hermetic named
// one-shot invocation on the shared no-server bootstrap path. It fails if named
// runs regress to requiring a listening HTTP server or drift from shared CLI/API
// input-resolution and primary-result contracts.
func TestNoServerNamedInvocationIntegrationAndEquivalenceProof(t *testing.T) {
	if testing.Short() {
		t.Skip("consolidated package integration and invocation-equivalence proof for no-server named invocation")
	}

	t.Run("hermetic named success without listener", func(t *testing.T) {
		preserveRunGlobals(t)
		prohibitInvocationAPIServer(t)

		goalText := "consolidated no-server named integration proof"
		cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
		cfg.AutoPort = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := output.String(); got != "mock worker accepted" {
			t.Fatalf("stdout = %q, want invocationReturn primary result mock worker accepted", got)
		}
	})

	t.Run("shared input resolution and primary-result equivalence", func(t *testing.T) {
		goalText := "consolidated no-server equivalence proof"
		capture, output := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
			cfg.JSONOutput = true
		})
		assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
		assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	})
}

func namedSubagentNoServerInvocationRunConfig(t *testing.T, requestText string) RunConfig {
	t.Helper()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, subagent.PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}

	return RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           subagent.PackagedFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &requestText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      writePackagedSubagentNoServerMockWorkersConfig(t),
		DisableDefaultRecording:    true,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	}
}

func runNoServerSubagentBootstrapEquivalenceCase(
	t *testing.T,
	requestText string,
	mutate func(*RunConfig),
) (*capturingBootstrapRunner, *bytes.Buffer) {
	t.Helper()

	preserveRunGlobals(t)
	prohibitInvocationAPIServer(t)
	capture := installCapturingRealInvocationBootstrap(t)
	cfg := namedSubagentNoServerInvocationRunConfig(t, requestText)
	if mutate != nil {
		mutate(&cfg)
	}
	var output bytes.Buffer
	cfg.Output = &output

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return capture, &output
}

func TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named subagent no-server invocation")
	}

	preserveRunGlobals(t)
	prohibitInvocationAPIServer(t)

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, subagent.PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}

	mockWorkersPath := writePackagedSubagentNoServerMockWorkersConfig(t)
	requestText := "hermetic no-server named subagent prompt"

	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	err = Run(ctx, RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           subagent.PackagedFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &requestText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      mockWorkersPath,
		DisableDefaultRecording:    true,
		Output:                     &output,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want agent response mock worker accepted", got)
	}
	if got := output.String(); got == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
}

func TestRun_NamedSubagentNoServerBootstrap_TextPrimaryResultIsAgentResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap subagent primary-result selection")
	}

	requestText := "no-server bootstrap subagent primary-result prompt"
	_, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, nil)
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want agent response mock worker accepted", got)
	}
	if got := output.String(); got == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
}

func TestRun_NamedSubagentNoServerBootstrap_SuccessJSONMatchesAPIProjection(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap subagent CLI/API projection")
	}

	requestText := "no-server bootstrap subagent json parity prompt"
	capture, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, func(cfg *RunConfig) {
		cfg.JSONOutput = true
	})
	assertCapturedRequestMatchesLogicalAPIText(t, capture, requestText)
	assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	if capture.lastResult == nil || len(capture.lastResult.PrimaryResult) == 0 {
		t.Fatalf("capture result = %#v, want primary result", capture.lastResult)
	}
	if got := capture.lastResult.PrimaryResult[0].Text; got != "mock worker accepted" {
		t.Fatalf("primaryResult text = %q, want agent response mock worker accepted", got)
	}
	if got := capture.lastResult.PrimaryResult[0].Text; got == requestText {
		t.Fatalf("primaryResult echoed submitted request text instead of agent response")
	}
	if !strings.Contains(output.String(), `"primaryResult"`) {
		t.Fatalf("json output = %s, want primaryResult field", output.String())
	}
	if strings.Count(output.String(), `"primaryResult"`) != 1 {
		t.Fatalf("json output = %s, want exactly one primaryResult field", output.String())
	}
}

func writePackagedSubagentNoServerMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      subagent.PackagedWorkerName,
				WorkstationName: subagent.PackagedRunWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-subagent-no-server.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

// TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof is the
// consolidated package integration and invocation-equivalence proof for hermetic
// named one-shot @you/subagent invocation on the shared no-server bootstrap path.
func TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof(t *testing.T) {
	if testing.Short() {
		t.Skip("consolidated package integration and invocation-equivalence proof for no-server named subagent invocation")
	}

	t.Run("hermetic named success without listener", func(t *testing.T) {
		preserveRunGlobals(t)
		prohibitInvocationAPIServer(t)

		requestText := "consolidated no-server named subagent integration proof"
		cfg := namedSubagentNoServerInvocationRunConfig(t, requestText)
		cfg.AutoPort = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := output.String(); got != "mock worker accepted" {
			t.Fatalf("stdout = %q, want agent response mock worker accepted", got)
		}
		if got := output.String(); got == requestText {
			t.Fatalf("stdout echoed submitted request text instead of agent response")
		}
	})

	t.Run("shared input resolution and primary-result equivalence", func(t *testing.T) {
		requestText := "consolidated no-server subagent equivalence proof"
		capture, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, func(cfg *RunConfig) {
			cfg.JSONOutput = true
		})
		assertCapturedRequestMatchesLogicalAPIText(t, capture, requestText)
		assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	})
}
