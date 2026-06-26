package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type stubInvocationService struct {
	run    func(context.Context) error
	invoke func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error)
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

	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-not-ready",
					TraceID:   "trace-tts-not-ready",
					Status:    factoryapi.InvocationTerminalStatusFailed,
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

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-failed",
					TraceID:   "trace-tts-failed",
					Status:    factoryapi.InvocationTerminalStatusFailed,
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

	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
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

	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
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

	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-123",
					TraceID:   "trace-123",
					Status:    factoryapi.InvocationTerminalStatusFailed,
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
			buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
							Status:    factoryapi.InvocationTerminalStatusCompleted,
							PrimaryResult: []interfaces.WorkContentPart{{
								Type: interfaces.WorkContentPartTypeText,
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

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
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

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_BLOCKED",
		Message:   "goal invocation blocked while work \"Review plan\" is in state goal:blocked",
		SessionID: defaultFactorySessionID,
		WorkID:    "work-review-plan",
		WorkName:  "Review plan",
		WorkState: "goal:blocked",
	}

	var jsonOutput bytes.Buffer
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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

func TestRun_FactoryInvocationPausedFailureIncludesCLIContext(t *testing.T) {
	preserveRunGlobals(t)

	text := "pause the session"
	var output bytes.Buffer

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-paused",
					TraceID:   "trace-paused",
					Status:    factoryapi.InvocationTerminalStatusFailed,
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
