package run

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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

func extractInvocationText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	parts := request.Content
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
