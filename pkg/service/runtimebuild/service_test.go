package runtimebuild_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
)

func TestService_BuildReplacementAndBuildShareBuilder(t *testing.T) {
	t.Parallel()

	buildCalls := 0
	build := func(context.Context, runtimebuild.SessionBuildSpec) (any, error) {
		buildCalls++
		return "bundle", nil
	}
	svc := runtimebuild.New(runtimebuild.Config{}, factory.EnsureClock(nil), nil, build)

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
	buildCalls := 0
	build := func(_ context.Context, spec runtimebuild.SessionBuildSpec) (any, error) {
		buildCalls++
		cfg := &factory.FactoryConfig{}
		for _, option := range spec.AdditionalFactoryOpts {
			option(cfg)
		}
		if cfg.PetriMutationRecorder == nil {
			t.Fatal("PetriMutationRecorder = nil")
		}
		if err := cfg.PetriMutationRecorder("session-1", []interfaces.TokenMutationRecord{{TransitionID: "done"}}); !errors.Is(err, wantErr) {
			t.Fatalf("PetriMutationRecorder error = %v, want %v", err, wantErr)
		}
		return "bundle", nil
	}
	svc := runtimebuild.New(runtimebuild.Config{}, factory.EnsureClock(nil), nil, build).
		WithPetriMutationRecorder(func(string, []interfaces.TokenMutationRecord) error { return wantErr })

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

func TestSessionScopedRecordPath_ReplacesTokenAndSuffixesNonDefaultSessions(t *testing.T) {
	t.Parallel()

	tokenPath := "/recordings/factory-session-__factory_session_id__-1.json"
	if got := runtimebuild.SessionScopedRecordPath(tokenPath, "session-a"); got != "/recordings/factory-session-session-a-1.json" {
		t.Fatalf("token path = %q", got)
	}
	if got := runtimebuild.SessionScopedRecordPath("/tmp/recording.json", "~default"); got != "/tmp/recording.json" {
		t.Fatalf("default path = %q", got)
	}
	if got := runtimebuild.SessionScopedRecordPath("/tmp/recording.json", "session-b"); got != "/tmp/recording.session-b.json" {
		t.Fatalf("suffix path = %q", got)
	}
}
