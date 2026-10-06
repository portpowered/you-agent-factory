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
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	runtimestate "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPrepareExecutionSpecCarriesSessionInputsAndAppliesOperatorDefaults(t *testing.T) {
	t.Parallel()

	fixture := newSelectedBuildFixture(t)
	spec, err := fixture.service.PrepareExecutionSpec(t.Context(), fixture.defaults,
		runtimebuild.SessionBuildValues{Dir: "/factories/selected", FolderPath: "/workspace/project", SessionID: "session-selected",
			ExecutionBaseDir: "/runtime/session-selected", LoadedFactoryCfg: fixture.loaded},
		runtimebuild.SessionBuildSpec{Clock: platformclock.Real{}, BaseLogger: zap.NewNop(), ProviderOverride: fixture.replayProvider,
			ReplayCommandRunner: fixture.replayRunner, SubmissionHooks: []factory.SubmissionHook{fixture.hook}, CompletionPlanner: fixture.planner,
			PetriMutationRecorder: func(string, []factorydefinitions.TokenMutationRecord) error { return nil }},
		nil)
	if err != nil {
		t.Fatalf("PrepareExecutionSpec: %v", err)
	}
	assertSelectedBuildSpec(t, spec, fixture)
	assertSelectedWorkerDefaults(t, fixture.loaded)
}

func TestPrepareExecutionSpecKeepsOpeningEffectsIndependent(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"replay", "selected", "mock"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := &runtimeBuildLoadedSource{factoryDir: "/factory/" + name, config: &factorydefinitions.FactoryConfig{}}
			replayProvider := &testutil.NativeProvider{}
			replayRunner := platformprocess.CommandRunner(&behaviorCommandRunner{selection: name + "-replay"})
			providerRunner := platformprocess.CommandRunner(&behaviorCommandRunner{selection: name + "-provider"})
			var selectedProvider providers.Service
			var scriptRunner platformprocess.CommandRunner
			var mockConfig *workers.MockWorkersConfig
			if name != "replay" {
				selectedProvider = &testutil.NativeProvider{}
				scriptRunner = &behaviorCommandRunner{selection: name + "-script"}
			}
			if name == "mock" {
				mockConfig = &workers.MockWorkersConfig{}
			}
			var wrapped []platformprocess.CommandRunner
			decorate := func(config *workers.MockWorkersConfig, definitions factorydefinitions.RuntimeDefinitionLookup, next platformprocess.CommandRunner) platformprocess.CommandRunner {
				if config != mockConfig || definitions != candidate {
					t.Fatal("mock opening changed selected config or candidate")
				}
				wrapped = append(wrapped, next)
				return next
			}
			preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop(), selectedProvider, providerRunner, scriptRunner, decorate)
			clock := &platformclock.Real{}
			logger := zap.NewNop().With(zap.String("opening", name))
			spec, err := preparation.PrepareExecutionSpec(context.Background(), runtimebuild.BuildDefaults{},
				runtimebuild.SessionBuildValues{SessionID: name, RuntimeInstanceID: "runtime-" + name, LoadedFactoryCfg: candidate},
				runtimebuild.SessionBuildSpec{Clock: clock, BaseLogger: logger, ProviderOverride: replayProvider, ReplayCommandRunner: replayRunner},
				mockConfig)
			if err != nil {
				t.Fatal(err)
			}
			assertPreparedExecutionEffects(t, spec, clock, logger, replayProvider, selectedProvider, replayRunner, scriptRunner)
			if name == "mock" {
				if spec.ProviderCommandRunner != providerRunner || len(wrapped) != 2 || wrapped[0] != providerRunner || wrapped[1] != scriptRunner {
					t.Fatalf("mock execution selections = %#v, wrappers = %#v", spec, wrapped)
				}
			} else if spec.ProviderCommandRunner != nil || len(wrapped) != 0 {
				t.Fatal("ordinary opening acquired mock execution effects")
			}
		})
	}
}

func assertPreparedExecutionEffects(t *testing.T, spec runtimebuild.SessionBuildSpec, clock factory.Clock, logger *zap.Logger,
	replayProvider, selectedProvider providers.Service, replayRunner, scriptRunner platformprocess.CommandRunner,
) {
	t.Helper()
	wantProvider, wantRunner := replayProvider, replayRunner
	if selectedProvider != nil {
		wantProvider = selectedProvider
	}
	if scriptRunner != nil {
		wantRunner = scriptRunner
	}
	if spec.ProviderOverride != wantProvider || spec.CommandRunnerOverride != wantRunner || spec.ReplayCommandRunner != replayRunner ||
		spec.Clock != clock || spec.BaseLogger != logger {
		t.Fatalf("opening changed selected execution effects: %#v", spec)
	}
}

type selectedBuildFixture struct {
	service            *runtimebuild.Service
	defaults           runtimebuild.BuildDefaults
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
	fixture.defaults = runtimebuild.BuildDefaults{WorkerModelProvider: " CODEX ", WorkerModel: " gpt-5 ", ApplyOperatorDefaults: true,
		RecordPath: "/recordings/factory-__factory_session_id__.json", WorkflowID: "workflow-selected"}
	fixture.service = runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("supplied candidate unexpectedly loaded")
		return nil, nil
	}, testRuntimeID, zap.NewNop(), fixture.configuredProvider, nil, fixture.scriptRunner, nil)
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
	if spec.RecordPath != "/recordings/factory-session-selected.json" {
		t.Fatalf("session RecordPath = %q", spec.RecordPath)
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

func TestPrepareExecutionSpecReportsFailuresWithoutOpeningEffects(t *testing.T) {
	t.Parallel()
	loadErr, mutationErr := errors.New("factory unavailable"), errors.New("defaults mutation failed")
	for _, test := range []struct {
		name, provider, wantText string
		candidate                *runtimeBuildLoadedSource
		wantErr                  error
	}{
		{name: "load", wantText: "load factory config", wantErr: loadErr},
		{name: "unsupported defaults", provider: "unsupported provider", candidate: &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}}}}, wantText: "unsupported worker model provider"},
		{name: "mutation", provider: "CODEX", candidate: &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel}}}, mutateErr: mutationErr}, wantText: "apply operator defaults", wantErr: mutationErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			preparation := runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				return nil, loadErr
			}, testRuntimeID, zap.NewNop(), nil, nil, nil, func(*workers.MockWorkersConfig, factorydefinitions.RuntimeDefinitionLookup, platformprocess.CommandRunner) platformprocess.CommandRunner {
				t.Fatal("execution effect opened after preparation failure")
				return nil
			})
			var candidate factorydefinitions.MutableLoadedFactorySource
			if test.candidate != nil {
				candidate = test.candidate
			}
			selections := runtimebuild.SessionBuildSpec{Clock: platformclock.Real{}, BaseLogger: zap.NewNop()}
			spec, err := preparation.PrepareExecutionSpec(t.Context(), runtimebuild.BuildDefaults{WorkerModelProvider: test.provider, WorkerModel: "model", ApplyOperatorDefaults: true},
				runtimebuild.SessionBuildValues{Dir: "/factory", SessionID: "session", LoadedFactoryCfg: candidate}, selections,
				&workers.MockWorkersConfig{})
			if err == nil || !strings.Contains(err.Error(), test.wantText) || (test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("preparation error = %v", err)
			}
			if spec.Clock != nil || spec.LoadedFactoryCfg != nil || selections.Clock == nil || selections.BaseLogger == nil {
				t.Fatal("failed candidate published data or mutated caller selections")
			}
		})
	}
}

type runtimeBuildLoadedSource struct {
	factoryDir     string
	runtimeBaseDir string
	config         *factorydefinitions.FactoryConfig
	mutateErr      error
	replacements   []factorydefinitions.PortableBundledFileReplacement
}

type behaviorCommandRunner struct{ selection string }

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
	}, testRuntimeID, zap.NewNop(), nil, nil, nil, nil)
	defaults := runtimebuild.BuildDefaults{WorkerModelProvider: " CODEX ", WorkerModel: " gpt-5 ", ApplyOperatorDefaults: true,
		RecordPath: "/recordings/factory-__factory_session_id__.json", WorkflowID: "workflow-selected"}
	values := runtimebuild.SessionBuildValues{Dir: "/factories/selected", FolderPath: "/workspace/project", SessionID: "session-selected",
		ExecutionBaseDir: "/runtime/session-selected", LoadedFactoryCfg: candidate}
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
	if prepared.SessionID != "session-selected" || prepared.RecordPath != "/recordings/factory-session-selected.json" {
		t.Fatal("prepared values alias caller data")
	}
}

func TestPrepareLoadsSelectedCandidateAndSelectsIdentityAndRecordingPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, runtimeID, sessionID, wantID, wantPath string
	}{
		{"generated default", "  ", "session-a", "generated", "/recording.session-a.json"},
		{"trimmed explicit", " runtime-b ", "session-b", "runtime-b", "/recording.session-b.json"},
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
			}, func() string { ids++; return "generated" }, zap.NewNop(), nil, nil, nil, nil)
			prepared, err := preparation.Prepare(context.Background(), runtimebuild.BuildDefaults{RecordPath: "/recording.json"},
				runtimebuild.SessionBuildValues{Dir: "/selected", SessionID: test.sessionID, RuntimeInstanceID: test.runtimeID,
					ExecutionBaseDir: "/runtime"})
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
			}, testRuntimeID, zap.NewNop(), nil, nil, nil, nil)
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
	preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.New(core), nil, nil, nil, nil)
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

func TestPrepareSpecWarningsUseSelectedOpeningLogger(t *testing.T) {
	t.Parallel()
	processCore, processLogs := observer.New(zap.WarnLevel)
	preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.New(processCore), nil, nil, nil, nil)
	for _, id := range []string{"session-a", "session-b"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			core, logs := observer.New(zap.WarnLevel)
			selectedLogger := zap.New(core).With(zap.String("invocation_id", "invocation-"+id))
			clock := &platformclock.Real{}
			candidate := newSelectedBuildFixture(t).loaded
			candidate.replacements = []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "selected-target"}}
			spec, err := preparation.PrepareSpec(t.Context(), runtimebuild.BuildDefaults{}, runtimebuild.SessionBuildValues{
				SessionID: id, FolderPath: "/folder/" + id, LoadedFactoryCfg: candidate,
			}, runtimebuild.SessionBuildSpec{BaseLogger: selectedLogger, Clock: clock})
			if err != nil {
				t.Fatal(err)
			}
			if spec.BaseLogger != selectedLogger || spec.Clock != clock {
				t.Fatal("preparation changed the opening logger or clock")
			}
			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("selected opening warnings = %d, want 1", len(entries))
			}
			fields := entries[0].ContextMap()
			if fields["session_id"] != id || fields["invocation_id"] != "invocation-"+id || fields["folder_path"] != "/folder/"+id {
				t.Fatalf("warning lost opening attribution: %#v", fields)
			}
			if processLogs.Len() != 0 {
				t.Fatalf("selected opening logged through the process constructor: %#v", processLogs.All())
			}
		})
	}
}
