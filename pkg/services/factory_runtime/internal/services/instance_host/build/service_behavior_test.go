package runtimebuild_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	runtimestate "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestService_BuildSpecCarriesSessionInputsAndAppliesOperatorDefaults(t *testing.T) {
	t.Parallel()

	loaded := &runtimeBuildLoadedSource{
		factoryDir: "/factories/selected",
		config: &factorydefinitions.FactoryConfig{
			Name: "selected",
			Workers: []factorydefinitions.FactoryWorkerConfig{
				{Name: "empty-model", Type: factorydefinitions.WorkerTypeModel},
				{
					Name:          "invocation-model",
					Type:          factorydefinitions.WorkerTypeAgent,
					ModelProvider: "${provider}",
					Model:         "${model}",
				},
				{
					Name:          "explicit-model",
					Type:          factorydefinitions.WorkerTypeInference,
					ModelProvider: "configured-provider",
					Model:         "configured-model",
				},
				{Name: "script-worker", Type: "SCRIPT"},
			},
		},
	}
	configuredProvider := &testutil.NativeProvider{}
	replayProvider := &testutil.NativeProvider{}
	var providerRunner platformprocess.CommandRunner = &behaviorCommandRunner{}
	var scriptRunner platformprocess.CommandRunner = &behaviorCommandRunner{}
	var replayRunner platformprocess.CommandRunner = &behaviorCommandRunner{}
	hook := buildSubmissionHook{name: "selected-factory-hook"}
	planner := &buildCompletionPlanner{}
	recorder := factory.PetriMutationRecorder(func(string, []factorydefinitions.TokenMutationRecord) error {
		return nil
	})
	var buildCalled bool
	var builtSpec runtimebuild.SessionBuildSpec
	svc := mustNewBehaviorService(
		t,
		" CODEX ",
		" gpt-5 ",
		true,
		"/recordings/factory-__factory_session_id__.json",
		"workflow-selected",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("loader should not be called when source is supplied")
		},
		configuredProvider,
		providerRunner,
		scriptRunner,
		func(_ context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			buildCalled = true
			builtSpec = spec
			return &factoryhost.Bundle{}, nil
		},
		recorder,
	)

	spec, err := svc.BuildSpec(
		context.Background(),
		"/factories/selected",
		"/workspace/project",
		"session-selected",
		"/runtime/session-selected",
		loaded,
		"  ",
		replayProvider,
		replayRunner,
		[]factory.SubmissionHook{hook},
		planner,
		true,
	)
	if err != nil {
		t.Fatalf("BuildSpec: %v", err)
	}
	if buildCalled {
		t.Fatal("BuildSpec invoked the bundle builder")
	}
	if _, err := svc.Build(context.Background(), spec); err != nil {
		t.Fatalf("Build(derived spec): %v", err)
	}
	if !buildCalled || builtSpec.Dir != spec.Dir || builtSpec.SessionID != spec.SessionID || builtSpec.LoadedFactoryCfg != spec.LoadedFactoryCfg {
		t.Fatalf("builder did not receive derived session spec = %#v", builtSpec)
	}
	if spec.Dir != "/factories/selected" || spec.FolderPath != "/workspace/project" || spec.SessionID != "session-selected" {
		t.Fatalf("session locations = %#v", spec)
	}
	if spec.ExecutionBaseDir != "/runtime/session-selected" {
		t.Fatalf("ExecutionBaseDir = %q", spec.ExecutionBaseDir)
	}
	if spec.RuntimeInstanceID != testRuntimeID() {
		t.Fatalf("RuntimeInstanceID = %q, want generated test identity", spec.RuntimeInstanceID)
	}
	if spec.RecordPath != "/recordings/factory-~default.json" {
		t.Fatalf("compatibility RecordPath = %q", spec.RecordPath)
	}
	if spec.WorkflowID != "workflow-selected" || spec.LoadedFactoryCfg != loaded {
		t.Fatalf("definition identity fields = %#v", spec)
	}
	if spec.ProviderOverride != configuredProvider {
		t.Fatalf("ProviderOverride = %T, want configured provider", spec.ProviderOverride)
	}
	if spec.ProviderCommandRunner != nil {
		t.Fatalf("ProviderCommandRunner = %T, want nil without mock-worker mode", spec.ProviderCommandRunner)
	}
	if spec.CommandRunnerOverride != scriptRunner {
		t.Fatalf("CommandRunnerOverride = %T, want configured script runner", spec.CommandRunnerOverride)
	}
	if spec.ReplayCommandRunner != replayRunner {
		t.Fatalf("ReplayCommandRunner = %T, want replay runner", spec.ReplayCommandRunner)
	}
	if len(spec.SubmissionHooks) != 1 || spec.SubmissionHooks[0] != hook {
		t.Fatalf("SubmissionHooks = %#v, want selected hook", spec.SubmissionHooks)
	}
	if spec.CompletionPlanner != planner || spec.PetriMutationRecorder == nil {
		t.Fatalf("runtime collaborators = planner %T, recorder nil=%t", spec.CompletionPlanner, spec.PetriMutationRecorder == nil)
	}
	if loaded.runtimeBaseDir != "/runtime/session-selected" {
		t.Fatalf("RuntimeBaseDir = %q, want execution base", loaded.runtimeBaseDir)
	}

	workersByName := make(map[string]factorydefinitions.FactoryWorkerConfig, len(loaded.config.Workers))
	for _, worker := range loaded.config.Workers {
		workersByName[worker.Name] = worker
	}
	if worker := workersByName["empty-model"]; worker.ModelProvider != "codex" || worker.Model != "gpt-5" {
		t.Fatalf("empty worker defaults = %#v, want codex/gpt-5", worker)
	}
	if worker := workersByName["invocation-model"]; worker.ModelProvider != "${provider}" || worker.Model != "${model}" || worker.RuntimeDefaultModelProvider != "codex" || worker.RuntimeDefaultModel != "gpt-5" {
		t.Fatalf("invocation worker defaults = %#v, want placeholders plus fallbacks", worker)
	}
	if worker := workersByName["explicit-model"]; worker.ModelProvider != "configured-provider" || worker.Model != "configured-model" || worker.RuntimeDefaultModelProvider != "" || worker.RuntimeDefaultModel != "" {
		t.Fatalf("explicit worker values = %#v, want caller values unchanged", worker)
	}
	if worker := workersByName["script-worker"]; worker.ModelProvider != "" || worker.Model != "" {
		t.Fatalf("non-model worker was defaulted = %#v", worker)
	}
}

func TestService_BuildPropagatesBuilderOutcomeAndKeepsCallerSpecUnchanged(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("runtime construction failed")
	var captured runtimebuild.SessionBuildSpec
	buildCalls := 0
	svc := mustNewBehaviorService(
		t,
		"",
		"",
		false,
		"",
		"",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("unused loader")
		},
		nil,
		nil,
		nil,
		func(ctx context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			buildCalls++
			captured = spec
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, wantErr
		},
		func(string, []factorydefinitions.TokenMutationRecord) error { return nil },
	)

	callerSpec := runtimebuild.SessionBuildSpec{SessionID: "caller-owned"}
	_, err := svc.Build(context.Background(), callerSpec)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Build() error = %v, want %v", err, wantErr)
	}
	if buildCalls != 1 || captured.SessionID != callerSpec.SessionID {
		t.Fatalf("builder calls/spec = %d/%#v", buildCalls, captured)
	}
	if callerSpec.PetriMutationRecorder != nil {
		t.Fatal("Build mutated caller-owned spec")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.Build(canceled, runtimebuild.SessionBuildSpec{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Build() error = %v, want context.Canceled", err)
	}
	if buildCalls != 2 {
		t.Fatalf("builder calls after cancellation = %d, want 2", buildCalls)
	}
}

func TestService_BuildRejectsNilBundleAndPreservesBuilderErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   runtimebuild.BundleBuilder
		wantErr string
	}{
		{
			name: "nil bundle",
			build: func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
				return nil, nil
			},
			wantErr: "runtime builder returned nil bundle",
		},
		{
			name: "builder error",
			build: func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
				return nil, errors.New("builder exploded")
			},
			wantErr: "builder exploded",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := mustNewBehaviorService(
				t,
				"",
				"",
				false,
				"",
				"",
				func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return nil, errors.New("unused loader")
				},
				nil,
				nil,
				nil,
				test.build,
				nil,
			)
			_, err := svc.Build(context.Background(), runtimebuild.SessionBuildSpec{})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Build() error = %v, want text %q", err, test.wantErr)
			}
		})
	}
}

func TestService_BuildReplacementLoadsDefinitionAndScopesRecording(t *testing.T) {
	t.Parallel()

	loaded := &runtimeBuildLoadedSource{factoryDir: "/factories/loaded", config: &factorydefinitions.FactoryConfig{Name: "loaded"}}
	var loadedDir string
	var loadedLoader factorydefinitions.WorkstationLoader
	var captured runtimebuild.SessionBuildSpec
	wantBundle := &factoryhost.Bundle{}
	svc := mustNewBehaviorService(
		t,
		"",
		"",
		false,
		"/recordings/runtime.json",
		"workflow-replacement",
		func(dir string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			loadedDir = dir
			loadedLoader = loader
			return loaded, nil
		},
		nil,
		nil,
		nil,
		func(_ context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			captured = spec
			return wantBundle, nil
		},
		nil,
	)

	got, err := svc.BuildReplacement(
		context.Background(),
		"/workspace/folder",
		"/factories/loaded",
		"session-replacement",
		"/runtime/replacement",
	)
	if err != nil {
		t.Fatalf("BuildReplacement: %v", err)
	}
	if got != wantBundle {
		t.Fatalf("BuildReplacement result = %T, want builder bundle", got)
	}
	if loadedDir != "/factories/loaded" || loadedLoader != nil {
		t.Fatalf("loader inputs = %q/%T, want factory path and nil workstation loader", loadedDir, loadedLoader)
	}
	if captured.Dir != "/factories/loaded" || captured.FolderPath != "/workspace/folder" || captured.SessionID != "session-replacement" {
		t.Fatalf("replacement identity = %#v", captured)
	}
	if captured.ExecutionBaseDir != "/runtime/replacement" || captured.RuntimeInstanceID != testRuntimeID() {
		t.Fatalf("replacement runtime inputs = %#v", captured)
	}
	if captured.RecordPath != "/recordings/runtime.session-replacement.json" {
		t.Fatalf("session RecordPath = %q", captured.RecordPath)
	}
	if captured.WorkflowID != "workflow-replacement" || captured.LoadedFactoryCfg != loaded {
		t.Fatalf("replacement collaborators = %#v", captured)
	}
	if loaded.runtimeBaseDir != "/runtime/replacement" {
		t.Fatalf("loaded RuntimeBaseDir = %q", loaded.runtimeBaseDir)
	}
}

func TestService_BuildSpecReportsLoaderAndDefaultFailures(t *testing.T) {
	t.Parallel()

	loadErr := errors.New("factory definition unavailable")
	loader := func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, loadErr
	}
	svc := mustNewBehaviorService(
		t,
		"",
		"",
		false,
		"",
		"",
		loader,
		nil,
		nil,
		nil,
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			t.Fatal("builder called after loader failure")
			return nil, nil
		},
		nil,
	)
	_, err := svc.BuildReplacementSpec(context.Background(), "/folder", "/missing", "session", "/runtime")
	if !errors.Is(err, loadErr) || !strings.Contains(err.Error(), "load factory config") {
		t.Fatalf("loader error = %v, want wrapped %v", err, loadErr)
	}

	nilSourceService := mustNewBehaviorService(
		t,
		"",
		"",
		false,
		"",
		"",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, nil
		},
		nil,
		nil,
		nil,
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			t.Fatal("builder called after nil source")
			return nil, nil
		},
		nil,
	)
	_, err = nilSourceService.BuildReplacementSpec(context.Background(), "/folder", "/missing", "session", "/runtime")
	if err == nil || !strings.Contains(err.Error(), "loader returned nil source") {
		t.Fatalf("nil source error = %v, want explicit loader result failure", err)
	}

	invalidDefaults := &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}},
	}}
	invalidDefaultService := mustNewBehaviorService(
		t,
		"unsupported provider",
		"model",
		true,
		"",
		"",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("unused loader")
		},
		nil,
		nil,
		nil,
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			t.Fatal("builder called after invalid defaults")
			return nil, nil
		},
		nil,
	)
	_, err = invalidDefaultService.BuildSpec(context.Background(), "/factory", "/folder", "session", "/runtime", invalidDefaults, "id", nil, nil, nil, nil, false)
	if err == nil || !strings.Contains(err.Error(), "unsupported worker model provider") {
		t.Fatalf("invalid default error = %v, want unsupported provider diagnostic", err)
	}

	mutationErr := errors.New("worker defaults cannot be applied")
	mutationFailure := &runtimeBuildLoadedSource{
		config:    &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}}},
		mutateErr: mutationErr,
	}
	mutationService := mustNewBehaviorService(
		t,
		"CODEX",
		"model",
		true,
		"",
		"",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("unused loader")
		},
		nil,
		nil,
		nil,
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			t.Fatal("builder called after mutation failure")
			return nil, nil
		},
		nil,
	)
	_, err = mutationService.BuildSpec(context.Background(), "/factory", "/folder", "session", "/runtime", mutationFailure, "id", nil, nil, nil, nil, false)
	if !errors.Is(err, mutationErr) || !strings.Contains(err.Error(), "apply operator defaults") {
		t.Fatalf("mutation error = %v, want wrapped %v", err, mutationErr)
	}
}

func TestNewRejectsMissingFactoryLoader(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		return &factoryhost.Bundle{}, nil
	}
	service, err := runtimebuild.New(
		"",
		"",
		false,
		"",
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		platformclock.Real{},
		testRuntimeID,
		zap.NewNop(),
		build,
		nil,
	)
	if service != nil || err == nil || !strings.Contains(err.Error(), "Factory Definition loader is required") {
		t.Fatalf("New() = (%v, %v), want missing-loader error", service, err)
	}
}

func mustNewBehaviorService(
	t *testing.T,
	defaultProvider string,
	defaultModel string,
	applyDefaults bool,
	recordPath string,
	workflowID string,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	provider providers.Service,
	providerRunner platformprocess.CommandRunner,
	scriptRunner platformprocess.CommandRunner,
	build runtimebuild.BundleBuilder,
	recorder factory.PetriMutationRecorder,
) *runtimebuild.Service {
	t.Helper()
	service, err := runtimebuild.New(
		defaultProvider,
		defaultModel,
		applyDefaults,
		recordPath,
		workflowID,
		nil,
		loadFactory,
		provider,
		providerRunner,
		scriptRunner,
		nil,
		nil,
		platformclock.Real{},
		testRuntimeID,
		zap.NewNop(),
		build,
		recorder,
	)
	if err != nil {
		t.Fatalf("runtimebuild.New: %v", err)
	}
	return service
}

type runtimeBuildLoadedSource struct {
	factoryDir     string
	runtimeBaseDir string
	config         *factorydefinitions.FactoryConfig
	mutateErr      error
}

type behaviorCommandRunner struct{}

func (*behaviorCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

var _ factorydefinitions.MutableLoadedFactorySource = (*runtimeBuildLoadedSource)(nil)

func (source *runtimeBuildLoadedSource) FactoryDir() string {
	return source.factoryDir
}

func (source *runtimeBuildLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}

func (source *runtimeBuildLoadedSource) RuntimeBaseDir() string {
	return source.runtimeBaseDir
}

func (source *runtimeBuildLoadedSource) SetRuntimeBaseDir(dir string) {
	source.runtimeBaseDir = dir
}

func (source *runtimeBuildLoadedSource) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if source == nil || source.config == nil {
		return nil, false
	}
	for index := range source.config.Workers {
		if source.config.Workers[index].Name == name {
			return &source.config.Workers[index], true
		}
	}
	return nil, false
}

func (source *runtimeBuildLoadedSource) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if source == nil || source.config == nil {
		return nil, false
	}
	for index := range source.config.Workstations {
		if source.config.Workstations[index].Name == name {
			return &source.config.Workstations[index], true
		}
	}
	return nil, false
}

func (*runtimeBuildLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}

func (source *runtimeBuildLoadedSource) MutateWorkers(mutate func(*factorydefinitions.FactoryWorkerConfig) error) error {
	if source.mutateErr != nil {
		return source.mutateErr
	}
	if source.config == nil {
		return nil
	}
	for index := range source.config.Workers {
		if err := mutate(&source.config.Workers[index]); err != nil {
			return err
		}
	}
	return nil
}

type buildCompletionPlanner struct{}

func (*buildCompletionPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return 1, true, nil
}

type buildSubmissionHook struct {
	name string
}

func (hook buildSubmissionHook) Name() string {
	return hook.name
}

func (buildSubmissionHook) Priority() int {
	return 1
}

func (buildSubmissionHook) OnTick(
	context.Context,
	factorydefinitions.SubmissionHookContext[factorydefinitions.EngineStateSnapshot[petri.MarkingSnapshot, *runtimestate.Net]],
) (factorydefinitions.SubmissionHookResult, error) {
	return factorydefinitions.SubmissionHookResult{}, nil
}

var _ factory.SubmissionHook = buildSubmissionHook{}
var _ factory.CompletionDeliveryPlanner = (*buildCompletionPlanner)(nil)
