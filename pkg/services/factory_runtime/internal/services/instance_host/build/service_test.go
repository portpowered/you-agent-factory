package runtimebuild_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"go.uber.org/zap"
)

func testRuntimeID() string { return "runtime-test-id" }

func newRuntimeBuildService(
	clock factory.Clock,
	logger *zap.Logger,
	build runtimebuild.BundleBuilder,
	recorder factory.PetriMutationRecorder,
) (*runtimebuild.CompatibilityBuild, error) {
	return runtimebuild.BindCompatibility(runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, errors.New("unused test loader")
	}, testRuntimeID, logger), runtimebuild.BuildDefaults{},
		nil,
		nil,
		nil,
		nil,
		nil,
		clock,
		logger,
		build,
		recorder)
}

func TestService_BuildReplacementAndBuildShareBuilder(t *testing.T) {
	t.Parallel()

	buildCalls := 0
	build := func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		buildCalls++
		return &factoryhost.Bundle{}, nil
	}
	svc, err := newRuntimeBuildService(platformclock.Real{}, zap.NewNop(), build, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.Build(context.Background(), runtimebuild.SessionBuildSpec{
		Dir:        "/tmp/alpha",
		FolderPath: "/tmp",
		SessionID:  "~default",
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if buildCalls != 1 {
		t.Fatalf("build calls after startup path = %d, want 1", buildCalls)
	}
}

func TestService_WithPetriMutationRecorderInstallsRecorderOnEveryBuild(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist mutation")
	recorder := func(string, []factorydefinitions.TokenMutationRecord) error { return wantErr }
	buildCalls := 0
	build := func(_ context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		buildCalls++
		if spec.PetriMutationRecorder == nil {
			t.Fatal("PetriMutationRecorder = nil")
		}
		if err := spec.PetriMutationRecorder("session-1", []factorydefinitions.TokenMutationRecord{{TransitionID: "done"}}); !errors.Is(err, wantErr) {
			t.Fatalf("PetriMutationRecorder error = %v, want %v", err, wantErr)
		}
		return &factoryhost.Bundle{}, nil
	}
	svc, err := newRuntimeBuildService(platformclock.Real{}, zap.NewNop(), build, recorder)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.Build(context.Background(), runtimebuild.SessionBuildSpec{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := svc.Build(context.Background(), runtimebuild.SessionBuildSpec{SessionID: "session-1"}); err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if buildCalls != 2 {
		t.Fatalf("build calls = %d, want 2", buildCalls)
	}
}

func TestNewRejectsMissingConstructionDependencies(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
		return &factoryhost.Bundle{}, nil
	}
	tests := []struct {
		name        string
		clock       factory.Clock
		newID       factory.IDGenerator
		logger      *zap.Logger
		build       runtimebuild.BundleBuilder
		loadFactory factory.LoadedFactoryLoader
		want        string
	}{
		{name: "clock", logger: zap.NewNop(), build: build, want: "clock is required"},
		{name: "id generator", clock: platformclock.Real{}, logger: zap.NewNop(), build: build, want: "ID generator is required"},
		{name: "logger", clock: platformclock.Real{}, newID: testRuntimeID, build: build, want: "logger is required"},
		{name: "builder", clock: platformclock.Real{}, newID: testRuntimeID, logger: zap.NewNop(), want: "runtime builder is required"},
		{name: "factory loader", clock: platformclock.Real{}, newID: testRuntimeID, logger: zap.NewNop(), build: build, want: "Factory Definition loader is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := runtimebuild.BindCompatibility(runtimebuild.New(nil, test.loadFactory, test.newID, test.logger), runtimebuild.BuildDefaults{},
				nil,
				nil,
				nil,
				nil,
				nil,
				test.clock,
				test.logger,
				test.build,
				nil)
			if service != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = (%v, %v), want nil error containing %q", service, err, test.want)
			}
		})
	}
}

func TestSessionScopedRecordPath_PreservesDefaultAndSuffixesNonDefaultSessions(t *testing.T) {
	t.Parallel()

	if got := runtimebuild.SessionScopedRecordPath("/tmp/recording.json", "~default"); got != "/tmp/recording.json" {
		t.Fatalf("default path = %q", got)
	}
	if got := runtimebuild.SessionScopedRecordPath("/tmp/recording.json", "session-b"); got != "/tmp/recording.session-b.json" {
		t.Fatalf("suffix path = %q", got)
	}
}

func TestPrepareSharedServiceKeepsParallelSessionValuesIndependent(t *testing.T) {
	t.Parallel()
	preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop())
	for _, id := range []string{"session-a", "session-b"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			candidate := &runtimeBuildLoadedSource{factoryDir: "/factory/" + id, config: &factorydefinitions.FactoryConfig{}}
			result, err := preparation.Prepare(context.Background(), runtimebuild.BuildDefaults{RecordPath: "/recording.json", WorkflowID: "workflow-" + id},
				runtimebuild.SessionBuildValues{SessionID: id, FolderPath: "/folder/" + id, ExecutionBaseDir: "/runtime/" + id,
					RuntimeInstanceID: " runtime-" + id + " ", LoadedFactoryCfg: candidate})
			if err != nil {
				t.Fatal(err)
			}
			if result.SessionID != id || result.RuntimeInstanceID != "runtime-"+id || result.FolderPath != "/folder/"+id ||
				result.ExecutionBaseDir != "/runtime/"+id || result.RecordPath != "/recording."+id+".json" ||
				result.WorkflowID != "workflow-"+id || result.LoadedFactoryCfg != candidate || candidate.RuntimeBaseDir() != "/runtime/"+id {
				t.Fatalf("session values = %#v", result)
			}
		})
	}
}

func TestPrepareSpecKeepsSelectedEffectsAndParallelCandidatesIndependent(t *testing.T) {
	t.Parallel()
	preparation := runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop())
	for _, id := range []string{"session-a", "session-b"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			clock := &platformclock.Real{}
			logger := zap.NewNop().With(zap.String("selected", id))
			runner := platformprocess.CommandRunner(&behaviorCommandRunner{})
			hook := buildSubmissionHook{name: id}
			hooks := []factory.SubmissionHook{hook}
			candidate := &runtimeBuildLoadedSource{factoryDir: "/factory/" + id, config: &factorydefinitions.FactoryConfig{}}
			spec, err := preparation.PrepareSpec(context.Background(),
				runtimebuild.BuildDefaults{RecordPath: "/recording.json", WorkflowID: "workflow-" + id},
				runtimebuild.SessionBuildValues{Dir: candidate.FactoryDir(), SessionID: id, FolderPath: "/folder/" + id,
					ExecutionBaseDir: "/runtime/" + id, RuntimeInstanceID: " runtime-" + id + " ", LoadedFactoryCfg: candidate},
				runtimebuild.SessionBuildSpec{Clock: clock, BaseLogger: logger, ProviderCommandRunner: runner,
					CommandRunnerOverride: runner, ReplayCommandRunner: runner, SubmissionHooks: hooks, MetricsSessionID: "canonical-" + id})
			if err != nil {
				t.Fatal(err)
			}
			assertPreparedSpecEffects(t, spec, clock, logger, runner, "canonical-"+id)
			if spec.SessionID != id || spec.RuntimeInstanceID != "runtime-"+id || spec.LoadedFactoryCfg != candidate ||
				spec.ExecutionBaseDir != "/runtime/"+id || spec.RecordPath != "/recording."+id+".json" || spec.WorkflowID != "workflow-"+id {
				t.Fatalf("prepared candidate changed: %#v", spec)
			}
			hooks[0] = nil
			if len(spec.SubmissionHooks) != 1 || spec.SubmissionHooks[0] != hook {
				t.Fatal("candidate hooks alias the caller's slice")
			}
		})
	}
}

func assertPreparedSpecEffects(t *testing.T, spec runtimebuild.SessionBuildSpec, clock factory.Clock, logger *zap.Logger, runner platformprocess.CommandRunner, canonicalID string) {
	t.Helper()
	if spec.Clock != clock || spec.BaseLogger != logger || spec.ProviderCommandRunner != runner ||
		spec.CommandRunnerOverride != runner || spec.ReplayCommandRunner != runner || spec.MetricsSessionID != canonicalID {
		t.Fatalf("selected effects changed: %#v", spec)
	}
}

func TestPrepareSpecFailedCandidateLeavesOwnerReusable(t *testing.T) {
	t.Parallel()
	failure := errors.New("candidate load failed")
	preparation := runtimebuild.New(nil, func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, failure
	}, testRuntimeID, zap.NewNop())
	clock := &platformclock.Real{}
	selections := runtimebuild.SessionBuildSpec{Clock: clock}
	values := runtimebuild.SessionBuildValues{SessionID: "same-session", RuntimeInstanceID: "same-runtime", ExecutionBaseDir: "/runtime"}
	if spec, err := preparation.PrepareSpec(context.Background(), runtimebuild.BuildDefaults{}, values, selections); !errors.Is(err, failure) || spec.Clock != nil {
		t.Fatalf("failed preparation = (%#v, %v)", spec, err)
	}
	values.LoadedFactoryCfg = &runtimeBuildLoadedSource{factoryDir: "/factory", config: &factorydefinitions.FactoryConfig{}}
	spec, err := preparation.PrepareSpec(context.Background(), runtimebuild.BuildDefaults{}, values, selections)
	if err != nil || spec.SessionID != values.SessionID || spec.RuntimeInstanceID != values.RuntimeInstanceID || spec.Clock != clock {
		t.Fatalf("same-owner retry = (%#v, %v)", spec, err)
	}
}
