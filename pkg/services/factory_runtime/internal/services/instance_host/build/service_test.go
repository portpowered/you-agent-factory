package runtimebuild_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
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
) (*runtimebuild.Service, error) {
	return runtimebuild.New(
		"",
		"",
		false,
		"",
		"",
		nil,
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, errors.New("unused test loader")
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		clock,
		testRuntimeID,
		logger,
		build,
		recorder,
	)
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
			service, err := runtimebuild.New(
				"",
				"",
				false,
				"",
				"",
				nil,
				test.loadFactory,
				nil,
				nil,
				nil,
				nil,
				nil,
				test.clock,
				test.newID,
				test.logger,
				test.build,
				nil,
			)
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
