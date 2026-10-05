package runtimebuild_test

import (
	"context"
	"errors"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"go.uber.org/zap"
)

func testRuntimeID() string { return "runtime-test-id" }

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

func TestPrepareSpecRetainsMutationRecorderAcrossRepeatedCandidates(t *testing.T) {
	t.Parallel()
	owner := runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop())
	wantErr := errors.New("persist mutation")
	calls := 0
	selected := runtimebuild.SessionBuildSpec{PetriMutationRecorder: func(id string, mutations []factorydefinitions.TokenMutationRecord) error {
		calls++
		if id != "session" || len(mutations) != 1 || mutations[0].TransitionID != "done" {
			t.Fatal("recorder inputs changed")
		}
		return wantErr
	}}
	for range 2 {
		candidate := &runtimeBuildLoadedSource{config: &factorydefinitions.FactoryConfig{}}
		spec, err := owner.PrepareSpec(t.Context(), runtimebuild.BuildDefaults{}, runtimebuild.SessionBuildValues{SessionID: "session", LoadedFactoryCfg: candidate}, selected)
		if err != nil {
			t.Fatal(err)
		}
		if spec.PetriMutationRecorder == nil {
			t.Fatal("selected recorder was dropped")
		}
		if err := spec.PetriMutationRecorder("session", []factorydefinitions.TokenMutationRecord{{TransitionID: "done"}}); !errors.Is(err, wantErr) {
			t.Fatalf("recorder error = %v", err)
		}
	}
	if calls != 2 || selected.SessionID != "" || selected.LoadedFactoryCfg != nil {
		t.Fatalf("recorder calls = %d or caller selections changed", calls)
	}
}
