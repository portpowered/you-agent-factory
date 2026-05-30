package runtimebuild_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
)

func TestService_BuildReplacementAndBuildFromLoadedConfigShareBuilder(t *testing.T) {
	t.Parallel()

	buildCalls := 0
	build := func(context.Context, runtimebuild.BuildInput) (any, error) {
		buildCalls++
		return "bundle", nil
	}
	svc := runtimebuild.New(runtimebuild.Config{}, factory.EnsureClock(nil), nil, build)

	if _, err := svc.BuildFromLoadedConfig(context.Background(), runtimebuild.BuildInput{
		Dir:        "/tmp/alpha",
		FolderPath: "/tmp",
		SessionID:  "~default",
	}); err != nil {
		t.Fatalf("BuildFromLoadedConfig: %v", err)
	}
	if buildCalls != 1 {
		t.Fatalf("build calls after startup path = %d, want 1", buildCalls)
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
