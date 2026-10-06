package service

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestDurableOpeningCanonicalizesAndSnapshotsRequestFacts(t *testing.T) {
	t.Parallel()
	configured := operatorconfig.Config{
		Defaults:      operatorconfig.Defaults{WorkerModelProvider: "customer"},
		WorkerPresets: []operatorconfig.WorkerPreset{{ID: "review", ModelProvider: "agent", Model: "first"}},
	}
	var facts []durableexecution.ScopeFacts
	clock, logger := openingCoordinatorClock{}, zap.NewNop()
	owner := &portableReplayRuntimeOwner{}
	opening := NewDurableOpening(
		func(string) (operatorconfig.Config, error) { return configured, nil }, owner,
		func(_ context.Context, selected durableexecution.ScopeFacts, selectedClock factoryruntime.Clock, selectedLogger *zap.Logger) (func(context.Context) error, error) {
			if selectedClock != clock || selectedLogger != logger {
				t.Fatal("selected clock/logger lost")
			}
			facts = append(facts, selected)
			return func(context.Context) error { return nil }, nil
		},
		func(identity string) (string, error) {
			switch identity {
			case "CODEX":
				return "codex", nil
			case "customer":
				return "customer.provider", nil
			case "agent":
				return "cursor", nil
			default:
				return "", errors.New("unexpected provider")
			}
		}, false,
	)
	open := func(id, root, model string) DurableExecution {
		t.Helper()
		result, err := opening.Open(t.Context(), id, factorydefinitions.RuntimeSelection{Directory: root},
			factorysessions.PersistencePolicyDisabled, "/operator", "", operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModel: model},
			RuntimeRoot{FactoryRootDir: root, RuntimeInstanceID: "runtime-" + id, BaseLogger: logger}, clock, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := open("first", "/first", "first-default")
	configured.WorkerPresets[0].Model = "second"
	second := open("second", "/second", "second-default")
	if first.Service != owner || first.Release == nil || second.Release == nil {
		t.Fatal("fixed owner or scoped release lost")
	}
	if facts[0].FactorySessionID != "first" || facts[1].RuntimeID != "runtime-second" || facts[0].ProjectRoot != "/first" || facts[1].ProjectRoot != "/second" {
		t.Fatalf("facts = %#v", facts)
	}
	if first.WorkerSettings.DefaultModelProvider != "codex" || first.WorkerSettings.DefaultModel != "first-default" || first.WorkerSettings.Presets["review"].ModelProvider != "cursor" {
		t.Fatalf("settings = %#v", first.WorkerSettings)
	}
	second.WorkerSettings.Presets["review"] = factoryruntime.JavaScriptWorkerPreset{Model: "mutated"}
	facts[0].WorkerSettings.Presets["review"] = factoryruntime.JavaScriptWorkerPreset{Model: "mutated"}
	if first.WorkerSettings.Presets["review"].Model != "first" {
		t.Fatal("opening settings alias acquisition or peer")
	}
}

func TestDurableOpeningRetainsOnlyReleaseOnAcquisitionFailure(t *testing.T) {
	t.Parallel()
	failure, cleanupFailure := errors.New("acquisition failed"), errors.New("release failed")
	calls := 0
	opening := NewDurableOpening(
		func(string) (operatorconfig.Config, error) { return operatorconfig.Config{}, nil }, &portableReplayRuntimeOwner{},
		func(context.Context, durableexecution.ScopeFacts, factoryruntime.Clock, *zap.Logger) (func(context.Context) error, error) {
			return func(context.Context) error {
				calls++
				if calls == 1 {
					return cleanupFailure
				}
				return nil
			}, failure
		}, func(id string) (string, error) { return id, nil }, false,
	)
	opened, err := opening.Open(t.Context(), "candidate", factorydefinitions.RuntimeSelection{Directory: "/selected"},
		factorysessions.PersistencePolicyDisabled, "/operator", "", operatorconfig.ResolvedDefaults{}, RuntimeRoot{}, nil, nil, nil)
	if !errors.Is(err, failure) || opened.Service != nil || opened.Release == nil || opened.WorkerSettings != nil {
		t.Fatalf("failed candidate = %#v, %v", opened, err)
	}
	if err := errors.Join(err, opened.Release(t.Context())); !errors.Is(err, failure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("joined cleanup = %v", err)
	}
	if err := opened.Release(t.Context()); err != nil || calls != 2 {
		t.Fatalf("release retry = %v, calls %d", err, calls)
	}
}

func TestDurableOpeningPreservesChildModeSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		reachable bool
		provider  providers.Service
		mocks     *workers.MockWorkersConfig
		want      string
	}{
		{name: "no provider", want: factorysessions.ChildExecutorModeFake},
		{name: "reachable", reachable: true, want: factorysessions.ChildExecutorModeLive},
		{name: "mock", reachable: true, mocks: workers.NewEmptyMockWorkersConfig(), want: factorysessions.ChildExecutorModeFake},
		{name: "passthrough", reachable: true, mocks: &workers.MockWorkersConfig{UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough}, want: factorysessions.ChildExecutorModeLive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opening := NewDurableOpening(func(string) (operatorconfig.Config, error) { return operatorconfig.Config{}, nil }, nil,
				func(_ context.Context, facts durableexecution.ScopeFacts, _ factoryruntime.Clock, _ *zap.Logger) (func(context.Context) error, error) {
					if facts.ChildExecutorMode != tc.want {
						t.Fatalf("mode = %s, want %s", facts.ChildExecutorMode, tc.want)
					}
					return nil, nil
				}, func(id string) (string, error) { return id, nil }, tc.reachable)
			_, err := opening.Open(t.Context(), tc.name, factorydefinitions.RuntimeSelection{Directory: "/selected"},
				factorysessions.PersistencePolicyDisabled, "/operator", "", operatorconfig.ResolvedDefaults{}, RuntimeRoot{}, nil, tc.provider, tc.mocks)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
