package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRun_RedirectedHumanResponseStreamConsumesOnlyCanonicalTypedEvents(t *testing.T) {
	preserveRunGlobals(t)

	const canary = "SECRET_PROVIDER_PAYLOAD_7f8a"
	const answer = "authoritative answer"
	var output strings.Builder
	stub := &stubInvocationService{
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		events: canonicalJavaScriptFactoryEvents(),
	}
	stub.invoke = func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		return apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: answer}},
		}, nil
	}
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stub, nil
	}

	text := "prompt"
	err := Run(context.Background(), RunConfig{
		FactoryConfigPath: "/tmp/factory.json", InvocationPositionalText: &text,
		InvocationOutputMode: InvocationOutputResponseStream, StdinIsTTY: func() bool { return true },
		OutputIsTTY: false, Output: &output,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "[1] factory started\n" +
		"[2] workflow phase synthesize: ACTIVE\n" +
		"[3] workflow checkpoint written: draft-ready (RESUMABLE)\n\n" +
		responseStreamPrimaryResultHeader + "\n" + answer
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), canary) {
		t.Fatalf("human output used unsafe provider data: %q", output.String())
	}
}

func TestRun_TerminalOnlyModesIgnoreFactoryEventsForLiveAndReplay(t *testing.T) {
	preserveRunGlobals(t)

	const providerCanary = "SECRET_PROVIDER_CHUNK_3c19"
	const finalResult = "authoritative terminal result"
	providerResponse := providerCanary
	events := append(canonicalJavaScriptFactoryEvents(), canonicalFactoryEventWithPayload(
		4,
		interfaces.FactoryEventTypeInferenceResponse,
		workers.InferenceResponseEventPayload{Response: &providerResponse},
	))

	for _, source := range []struct {
		name       string
		replayPath string
	}{
		{name: "live"},
		{name: "replay", replayPath: "/tmp/terminal-only-replay.json"},
	} {
		for _, mode := range []struct {
			name       string
			jsonOutput bool
		}{
			{name: "quiet"},
			{name: "single JSON", jsonOutput: true},
		} {
			t.Run(source.name+"/"+mode.name, func(t *testing.T) {
				var output strings.Builder
				var openedReplayPath string
				stub := &stubInvocationService{
					run: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					events: events,
					invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
						return apisurface.FactoryInvocationResult{
							RequestID: "request-terminal-only",
							Status:    interfaces.InvocationTerminalStatusCompleted,
							PrimaryResult: []work.WorkContentPart{{
								Type: work.WorkContentPartTypeText,
								Text: finalResult,
							}},
						}, nil
					},
				}
				openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
					openedReplayPath = cfg.ReplayPath
					return stub, nil
				}

				prompt := "terminal-only prompt"
				err := Run(context.Background(), RunConfig{
					FactoryConfigPath:        "/tmp/factory.json",
					InvocationPositionalText: &prompt,
					StdinIsTTY:               func() bool { return true },
					ReplayPath:               source.replayPath,
					TerminalPolicy: terminalpolicy.Resolve(terminalpolicy.Options{
						Quiet: true,
					}),
					JSONOutput: mode.jsonOutput,
					Output:     &output,
				})
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				if openedReplayPath != source.replayPath {
					t.Fatalf("opened replay path = %q, want %q", openedReplayPath, source.replayPath)
				}
				if strings.Contains(output.String(), providerCanary) || strings.Contains(output.String(), "factory_event") {
					t.Fatalf("terminal-only output exposed lifecycle data: %q", output.String())
				}

				if !mode.jsonOutput {
					if got := output.String(); got != finalResult {
						t.Fatalf("quiet stdout = %q, want raw result %q", got, finalResult)
					}
					return
				}

				var response factoryapi.InvocationResponse
				if err := json.Unmarshal([]byte(output.String()), &response); err != nil {
					t.Fatalf("decode single JSON stdout: %v\n%s", err, output.String())
				}
				if response.Status != factoryapi.InvocationTerminalStatusCompleted ||
					response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
					t.Fatalf("single JSON response = %#v, want one completed InvocationResponse", response)
				}
			})
		}
	}
}

func TestRun_BootstrapErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Bootstrap: true,
		ResolveCurrentFactoryDir: func(string) (string, error) {
			return "", interfaces.ErrFactoryLayoutNotFound
		},
		FactoryScaffoldInitializer: func(interfaces.ScaffoldConfig) error {
			return errors.New("bootstrap failed")
		},
	})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("error = %q, want bootstrap failure", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not run when bootstrap fails")
	}
}

func TestRun_VerbosePassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedVerbose bool
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedVerbose = cfg.Verbose
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Verbose: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !capturedVerbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestRun_DoesNotDiscoverMissingExecutionBaseDir(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedBaseDir string
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", DisableDefaultRecording: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedBaseDir != "" {
		t.Fatalf("execution base dir = %q, want the missing Process input to remain missing", capturedBaseDir)
	}
}

func TestRun_PreservesExplicitExecutionBaseDir(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	overrideDir := t.TempDir()

	var capturedBaseDir string
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", ExecutionBaseDir: overrideDir, DisableDefaultRecording: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(overrideDir) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, overrideDir)
	}
}

func TestRun_RuntimeLogConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeLogConfig := logging.RuntimeLogConfig{
		MaxSize:    12,
		MaxBackups: 6,
		MaxAge:     21,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeLogDir:    "runtime-logs",
		RuntimeLogConfig: runtimeLogConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeLogDir != "runtime-logs" {
		t.Fatalf("runtime log dir = %q, want runtime-logs", capturedConfig.RuntimeLogDir)
	}
	if capturedConfig.RuntimeLogConfig != runtimeLogConfig {
		t.Fatalf("runtime log config = %#v, want %#v", capturedConfig.RuntimeLogConfig, runtimeLogConfig)
	}
}

func TestRun_RuntimeMetricsConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeMetricsConfig := platformmetrics.RuntimeMetricsConfig{
		MaxSize:    14,
		MaxBackups: 7,
		MaxAge:     28,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeMetricsDir:    "runtime-metrics",
		RuntimeMetricsConfig: runtimeMetricsConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeMetricsDir != "runtime-metrics" {
		t.Fatalf("runtime metrics dir = %q, want runtime-metrics", capturedConfig.RuntimeMetricsDir)
	}
	if capturedConfig.RuntimeMetricsConfig != runtimeMetricsConfig {
		t.Fatalf("runtime metrics config = %#v, want %#v", capturedConfig.RuntimeMetricsConfig, runtimeMetricsConfig)
	}
}

func TestRun_ModelCacheDirPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{ModelCacheDir: "managed-model-cache"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.ModelCacheDir != "managed-model-cache" {
		t.Fatalf("model cache dir = %q, want managed-model-cache", capturedConfig.ModelCacheDir)
	}
}

func TestRun_WithMockWorkersWithoutPathPassesDefaultConfigToService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{MockWorkersEnabled: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected default mock workers config to be passed to service")
	}
	if len(capturedConfig.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(capturedConfig.MockWorkersConfig.MockWorkers))
	}
}

func TestRun_WithMockWorkersConfigPathLoadsConfigBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	mockWorkersPath := filepath.Join(t.TempDir(), "mock-workers.json")
	exitCode := 42
	wantConfig := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		ID: "reviewer-rejects", WorkerName: "reviewer", RunType: workers.MockWorkerRunTypeReject,
		RejectConfig: &workers.MockWorkerRejectConfig{Stderr: "needs changes", ExitCode: &exitCode},
	}}}
	var loadedPath string
	load := workers.MockWorkersConfigLoader(func(path string) (*workers.MockWorkersConfig, error) {
		loadedPath = path
		return wantConfig, nil
	})

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	}, load)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected loaded mock workers config to be passed to service")
	}
	if loadedPath != mockWorkersPath {
		t.Fatalf("loader path = %q, want exact CLI path %q", loadedPath, mockWorkersPath)
	}
	got := capturedConfig.MockWorkersConfig.MockWorkers
	if len(got) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(got))
	}
	if got[0].ID != "reviewer-rejects" || got[0].WorkerName != "reviewer" {
		t.Fatalf("loaded mock worker = %#v, want reviewer target", got[0])
	}
	if got[0].RejectConfig == nil || got[0].RejectConfig.ExitCode == nil || *got[0].RejectConfig.ExitCode != 42 {
		t.Fatalf("reject config = %#v, want exit code 42", got[0].RejectConfig)
	}
}

func TestRun_WithMockWorkersInvalidPathFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	wantReadErr := errors.New("read mock workers config: missing")
	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	}, func(string) (*workers.MockWorkersConfig, error) { return nil, wantReadErr })
	if err == nil {
		t.Fatal("expected missing mock workers config path to fail")
	}
	if !strings.Contains(err.Error(), "read mock workers config") {
		t.Fatalf("error = %q, want read mock workers config context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config loading fails")
	}
}

func TestRun_WithMockWorkersInvalidJSONFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{"mockWorkers":[{"runType":"bogus"}]}`)

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	wantParseErr := errors.New("runType must be one of accept, script, or reject")
	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	}, func(string) (*workers.MockWorkersConfig, error) { return nil, wantParseErr })
	if err == nil {
		t.Fatal("expected invalid mock workers config to fail")
	}
	if !strings.Contains(err.Error(), "runType must be one of") {
		t.Fatalf("error = %q, want runType validation context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config validation fails")
	}
}

func TestRun_WithSkipPermissionsPassesInvocationOverrideToService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	override := true
	err := Run(context.Background(), RunConfig{
		InvocationSkipPermissionsOverride: &override,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.InvocationSkipPermissionsOverride == nil {
		t.Fatal("expected invocation skip-permissions override to be passed to service")
	}
	if !*capturedConfig.InvocationSkipPermissionsOverride {
		t.Fatal("expected invocation skip-permissions override to be true")
	}
}

func TestRun_WithoutSkipPermissionsOmitsInvocationOverrideFromService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service config to be captured")
	}
	if capturedConfig.InvocationSkipPermissionsOverride != nil {
		t.Fatalf("invocation skip-permissions override = %#v, want nil when flag omitted", capturedConfig.InvocationSkipPermissionsOverride)
	}
}

func TestRun_WithSkipPermissionsDoesNotMutatePersistedFactoryWorkerConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	dir := t.TempDir()
	factoryJSON := filepath.Join(dir, "factory.json")
	writeFile(t, factoryJSON, `{
  "factory": {
    "workers": [
      {
        "name": "agent",
        "type": "MODEL_WORKER",
        "modelProvider": "CLAUDE",
        "skipPermissions": false
      }
    ]
  }
}`)

	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	override := true
	err := Run(context.Background(), RunConfig{
		Dir:                               dir,
		InvocationSkipPermissionsOverride: &override,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(factoryJSON)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if !strings.Contains(string(got), `"skipPermissions": false`) {
		t.Fatalf("factory.json = %s, want persisted skipPermissions:false unchanged", string(got))
	}
	if strings.Contains(string(got), `"skipPermissions": true`) {
		t.Fatalf("factory.json = %s, want skipPermissions not persisted as true", string(got))
	}
}
