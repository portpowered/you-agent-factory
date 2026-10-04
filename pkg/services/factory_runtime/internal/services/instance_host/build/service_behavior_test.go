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
	"go.uber.org/zap/zaptest/observer"
)

func TestService_BuildSpecCarriesSessionInputsAndAppliesOperatorDefaults(t *testing.T) {
	t.Parallel()

	fixture := newSelectedBuildFixture(t)
	spec, err := fixture.service.BuildSpec(
		context.Background(),
		"/factories/selected",
		"/workspace/project",
		"session-selected",
		"/runtime/session-selected",
		fixture.loaded,
		"  ",
		fixture.replayProvider,
		fixture.replayRunner,
		[]factory.SubmissionHook{fixture.hook},
		fixture.planner,
		true,
	)
	if err != nil {
		t.Fatalf("BuildSpec: %v", err)
	}
	assertSelectedBuildSpec(t, spec, fixture)
	assertSelectedWorkerDefaults(t, fixture.loaded)
}

type selectedBuildFixture struct {
	service            *runtimebuild.CompatibilityBuild
	loaded             *runtimeBuildLoadedSource
	configuredProvider *testutil.NativeProvider
	replayProvider     *testutil.NativeProvider
	scriptRunner       platformprocess.CommandRunner
	replayRunner       platformprocess.CommandRunner
	hook               buildSubmissionHook
	planner            *buildCompletionPlanner
}

func newSelectedBuildFixture(t *testing.T) selectedBuildFixture {
	t.Helper()
	fixture := selectedBuildFixture{
		loaded: &runtimeBuildLoadedSource{
			factoryDir: "/factories/selected",
			config: &factorydefinitions.FactoryConfig{
				Name: "selected",
				Workers: []factorydefinitions.FactoryWorkerConfig{
					{Name: "empty-model", Type: factorydefinitions.WorkerTypeModel},
					{Name: "invocation-model", Type: factorydefinitions.WorkerTypeAgent, ModelProvider: "${provider}", Model: "${model}"},
					{Name: "explicit-model", Type: factorydefinitions.WorkerTypeInference, ModelProvider: "configured-provider", Model: "configured-model"},
					{Name: "script-worker", Type: "SCRIPT"},
				},
			},
		},
		configuredProvider: &testutil.NativeProvider{},
		replayProvider:     &testutil.NativeProvider{},
		scriptRunner:       &behaviorCommandRunner{},
		replayRunner:       &behaviorCommandRunner{},
		hook:               buildSubmissionHook{name: "selected-factory-hook"},
		planner:            &buildCompletionPlanner{},
	}
	providerRunner := platformprocess.CommandRunner(&behaviorCommandRunner{})
	recorder := factory.PetriMutationRecorder(func(string, []factorydefinitions.TokenMutationRecord) error {
		return nil
	})
	fixture.service = mustNewBehaviorService(
		t,
		" CODEX ",
		" gpt-5 ",
		true,
		"/recordings/factory-__factory_session_id__.json",
		"workflow-selected",
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("loader should not be called when source is supplied")
		},
		fixture.configuredProvider,
		providerRunner,
		fixture.scriptRunner,
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			return &factoryhost.Bundle{}, nil
		},
		recorder,
	)
	return fixture
}

func assertSelectedBuildSpec(t *testing.T, spec runtimebuild.SessionBuildSpec, fixture selectedBuildFixture) {
	t.Helper()
	assertSelectedBuildIdentity(t, spec, fixture)
	assertSelectedBuildCollaborators(t, spec, fixture)
}

func assertSelectedBuildIdentity(t *testing.T, spec runtimebuild.SessionBuildSpec, fixture selectedBuildFixture) {
	t.Helper()
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
	if spec.WorkflowID != "workflow-selected" || spec.LoadedFactoryCfg != fixture.loaded {
		t.Fatalf("definition identity fields = %#v", spec)
	}
}

func assertSelectedBuildCollaborators(t *testing.T, spec runtimebuild.SessionBuildSpec, fixture selectedBuildFixture) {
	t.Helper()
	if spec.ProviderOverride != fixture.configuredProvider {
		t.Fatalf("ProviderOverride = %T, want configured provider", spec.ProviderOverride)
	}
	if spec.ProviderCommandRunner != nil {
		t.Fatalf("ProviderCommandRunner = %T, want nil without mock-worker mode", spec.ProviderCommandRunner)
	}
	if spec.CommandRunnerOverride != fixture.scriptRunner {
		t.Fatalf("CommandRunnerOverride = %T, want configured script runner", spec.CommandRunnerOverride)
	}
	if spec.ReplayCommandRunner != fixture.replayRunner {
		t.Fatalf("ReplayCommandRunner = %T, want replay runner", spec.ReplayCommandRunner)
	}
	if len(spec.SubmissionHooks) != 1 || spec.SubmissionHooks[0] != fixture.hook {
		t.Fatalf("SubmissionHooks = %#v, want selected hook", spec.SubmissionHooks)
	}
	if spec.CompletionPlanner != fixture.planner || spec.PetriMutationRecorder == nil {
		t.Fatalf("runtime collaborators = planner %T, recorder nil=%t", spec.CompletionPlanner, spec.PetriMutationRecorder == nil)
	}
	if fixture.loaded.runtimeBaseDir != "/runtime/session-selected" {
		t.Fatalf("RuntimeBaseDir = %q, want execution base", fixture.loaded.runtimeBaseDir)
	}
}

func assertSelectedWorkerDefaults(t *testing.T, loaded *runtimeBuildLoadedSource) {
	t.Helper()
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

func TestService_BuildSpecReportsLoaderFailure(t *testing.T) {
	t.Parallel()

	loadErr := errors.New("factory definition unavailable")
	loader := func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, loadErr
	}
	service := newBuildSpecFailureService(t, loader)
	_, err := service.BuildReplacementSpec(context.Background(), "/folder", "/missing", "session", "/runtime")
	if !errors.Is(err, loadErr) || !strings.Contains(err.Error(), "load factory config") {
		t.Fatalf("loader error = %v, want wrapped %v", err, loadErr)
	}
}

func TestService_BuildSpecRejectsUnsupportedOperatorDefault(t *testing.T) {
	t.Parallel()

	loaded := &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}},
	}}
	service := newDefaultOperatorFailureService(t, "unsupported provider", "model")
	_, err := service.BuildSpec(
		context.Background(),
		"/factory",
		"/folder",
		"session",
		"/runtime",
		loaded,
		"id",
		nil,
		nil,
		nil,
		nil,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported worker model provider") {
		t.Fatalf("invalid default error = %v, want unsupported provider diagnostic", err)
	}
}

func TestService_BuildSpecReportsOperatorDefaultMutationFailure(t *testing.T) {
	t.Parallel()

	mutationErr := errors.New("worker defaults cannot be applied")
	loaded := &runtimeBuildLoadedSource{
		config:    &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}}},
		mutateErr: mutationErr,
	}
	service := mustNewBehaviorService(
		t,
		"CODEX",
		"model",
		true,
		"",
		"",
		unusedFactoryLoader(),
		nil,
		nil,
		nil,
		failIfBuildCalled(t),
		nil,
	)
	_, err := service.BuildSpec(
		context.Background(),
		"/factory",
		"/folder",
		"session",
		"/runtime",
		loaded,
		"id",
		nil,
		nil,
		nil,
		nil,
		false,
	)
	if !errors.Is(err, mutationErr) || !strings.Contains(err.Error(), "apply operator defaults") {
		t.Fatalf("mutation error = %v, want wrapped %v", err, mutationErr)
	}
}

func newBuildSpecFailureService(t *testing.T, loader factorydefinitions.LoadedFactoryLoader) *runtimebuild.CompatibilityBuild {
	t.Helper()
	return mustNewBehaviorService(
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
		failIfBuildCalled(t),
		nil,
	)
}

func newDefaultOperatorFailureService(t *testing.T, provider string, model string) *runtimebuild.CompatibilityBuild {
	t.Helper()
	return mustNewBehaviorService(
		t,
		provider,
		model,
		true,
		"",
		"",
		unusedFactoryLoader(),
		nil,
		nil,
		nil,
		failIfBuildCalled(t),
		nil,
	)
}

func unusedFactoryLoader() factorydefinitions.LoadedFactoryLoader {
	return func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, errors.New("unused loader")
	}
}

func failIfBuildCalled(t *testing.T) runtimebuild.BundleBuilder {
	t.Helper()
	return func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		t.Fatal("builder called after BuildSpec failure")
		return nil, nil
	}
}

func TestNewRejectsMissingFactoryLoader(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		return &factoryhost.Bundle{}, nil
	}
	service, err := runtimebuild.BindCompatibility(runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop()), runtimebuild.BuildDefaults{},
		nil,
		nil,
		nil,
		nil,
		nil,
		platformclock.Real{},
		zap.NewNop(),
		build,
		nil)
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
) *runtimebuild.CompatibilityBuild {
	t.Helper()
	service, err := runtimebuild.BindCompatibility(runtimebuild.New(nil, loadFactory, testRuntimeID, zap.NewNop()), runtimebuild.BuildDefaults{WorkerModelProvider: defaultProvider, WorkerModel: defaultModel, ApplyOperatorDefaults: applyDefaults, RecordPath: recordPath, WorkflowID: workflowID},
		provider,
		providerRunner,
		scriptRunner,
		nil,
		nil,
		platformclock.Real{},
		zap.NewNop(),
		build,
		recorder)
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
	replacements   []factorydefinitions.PortableBundledFileReplacement
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

func (source *runtimeBuildLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return source.replacements
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

func TestPrepareSuppliedCandidateKeepsPriorGenerationAndDetachedValues(t *testing.T) {
	t.Parallel()
	prior := newSelectedBuildFixture(t).loaded
	prior.SetRuntimeBaseDir("/active")
	candidate := newSelectedBuildFixture(t).loaded
	preparation := runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("supplied candidate must skip loading")
		return nil, nil
	}, testRuntimeID, zap.NewNop())
	defaults := runtimebuild.BuildDefaults{WorkerModelProvider: " CODEX ", WorkerModel: " gpt-5 ", ApplyOperatorDefaults: true,
		RecordPath: "/recordings/factory-__factory_session_id__.json", WorkflowID: "workflow-selected"}
	values := runtimebuild.SessionBuildValues{Dir: "/factories/selected", FolderPath: "/workspace/project", SessionID: "session-selected",
		ExecutionBaseDir: "/runtime/session-selected", LoadedFactoryCfg: candidate, PreserveCompatibilityDefaultRecordPath: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prepared, err := preparation.Prepare(ctx, defaults, values)
	if err != nil {
		t.Fatalf("Prepare canceled context: %v", err)
	}
	assertSelectedBuildIdentity(t, runtimebuild.SessionBuildSpec{Dir: prepared.Dir, FolderPath: prepared.FolderPath, SessionID: prepared.SessionID,
		ExecutionBaseDir: prepared.ExecutionBaseDir, RuntimeInstanceID: prepared.RuntimeInstanceID, RecordPath: prepared.RecordPath,
		WorkflowID: prepared.WorkflowID, LoadedFactoryCfg: prepared.LoadedFactoryCfg}, selectedBuildFixture{loaded: candidate})
	assertSelectedWorkerDefaults(t, candidate)
	if prior.RuntimeBaseDir() != "/active" || prior.config.Workers[0].Model != "" {
		t.Fatalf("prior generation mutated: %#v", prior)
	}
	if values.RuntimeInstanceID != "" || values.ExecutionBaseDir != "/runtime/session-selected" || defaults.WorkerModel != " gpt-5 " {
		t.Fatal("caller values changed")
	}
	defaults.RecordPath = "/changed"
	values.SessionID = "changed"
	if prepared.SessionID != "session-selected" || prepared.RecordPath != "/recordings/factory-~default.json" {
		t.Fatal("prepared values alias caller data")
	}
}

func TestPrepareLoadsSelectedCandidateAndSelectsIdentityAndRecordingPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, runtimeID, sessionID, wantID, wantPath string
		compatibility                                bool
	}{
		{"generated default", "  ", "session-a", "generated", "/recording.json", true},
		{"trimmed explicit", " runtime-b ", "session-b", "runtime-b", "/recording.session-b.json", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := newSelectedBuildFixture(t).loaded
			selectedLoader := &preparationWorkstationLoader{}
			calls, ids := 0, 0
			preparation := runtimebuild.New(selectedLoader, func(dir string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				calls++
				if dir != "/selected" || loader != selectedLoader {
					t.Fatalf("load selection = %q, %T", dir, loader)
				}
				return candidate, nil
			}, func() string { ids++; return "generated" }, zap.NewNop())
			prepared, err := preparation.Prepare(context.Background(), runtimebuild.BuildDefaults{RecordPath: "/recording.json"},
				runtimebuild.SessionBuildValues{Dir: "/selected", SessionID: test.sessionID, RuntimeInstanceID: test.runtimeID,
					ExecutionBaseDir: "/runtime", PreserveCompatibilityDefaultRecordPath: test.compatibility})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || prepared.LoadedFactoryCfg != candidate || prepared.RuntimeInstanceID != test.wantID || prepared.RecordPath != test.wantPath || candidate.RuntimeBaseDir() != "/runtime" {
				t.Fatalf("prepared = %#v, loads = %d", prepared, calls)
			}
			wantIDs := 0
			if test.wantID == "generated" {
				wantIDs = 1
			}
			if ids != wantIDs {
				t.Fatalf("generated IDs = %d, want %d", ids, wantIDs)
			}
		})
	}
}

type preparationWorkstationLoader struct{}

func (*preparationWorkstationLoader) Load(string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	return nil, nil
}

func TestPreparePreservesFailureIdentityAndContext(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("selected failure")
	for _, test := range []struct {
		name, provider, context    string
		loadFailure, mutateFailure bool
	}{
		{name: "load", context: "load factory config", loadFailure: true},
		{name: "unsupported provider", provider: "unsupported provider", context: "unsupported"},
		{name: "mutation", provider: "codex", context: "apply operator defaults", mutateFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := newSelectedBuildFixture(t).loaded
			if test.mutateFailure {
				candidate.mutateErr = sentinel
			}
			preparation := runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				if test.loadFailure {
					return nil, sentinel
				}
				return candidate, nil
			}, testRuntimeID, zap.NewNop())
			result, err := preparation.Prepare(context.Background(), runtimebuild.BuildDefaults{ApplyOperatorDefaults: true, WorkerModelProvider: test.provider, WorkerModel: "model"}, runtimebuild.SessionBuildValues{})
			if err == nil || !strings.Contains(err.Error(), test.context) {
				t.Fatalf("Prepare error = %v", err)
			}
			if (test.loadFailure || test.mutateFailure) && !errors.Is(err, sentinel) {
				t.Fatalf("lost sentinel: %v", err)
			}
			if result.LoadedFactoryCfg != nil {
				t.Fatal("failed preparation returned candidate")
			}
		})
	}
}

func TestPrepareWarningsPreserveSessionFieldsAndTargetOrder(t *testing.T) {
	t.Parallel()
	core, observed := observer.New(zap.WarnLevel)
	preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.New(core))
	for _, id := range []string{"session-a", "session-b"} {
		candidate := newSelectedBuildFixture(t).loaded
		candidate.replacements = []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "first"}, {TargetPath: "second"}}
		_, err := preparation.Prepare(context.Background(), runtimebuild.BuildDefaults{}, runtimebuild.SessionBuildValues{
			SessionID: id, FolderPath: "/folder/" + id, LoadedFactoryCfg: candidate})
		if err != nil {
			t.Fatal(err)
		}
	}
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("warning count = %d", len(entries))
	}
	for i, id := range []string{"session-a", "session-b"} {
		entry := entries[i]
		fields := entry.ContextMap()
		if entry.Message != "named factory activation replaced portable bundled files" || fields["session_id"] != id || fields["folder_path"] != "/folder/"+id || fields["factory_dir"] != "/factories/selected" {
			t.Fatalf("warning = %#v", entry)
		}
		paths, ok := fields["target_paths"].([]interface{})
		if !ok || len(paths) != 2 || paths[0] != "first" || paths[1] != "second" {
			t.Fatalf("ordered targets = %#v", paths)
		}
	}
}
