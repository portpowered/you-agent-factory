package acp_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "persistent")
	var starts atomic.Int32
	root, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers: %v", err)
	}
	workingDirectory := t.TempDir()
	lifecycle, ok := root.(providers.Lifecycle)
	if !ok {
		t.Fatal("Providers root omitted its exact lifecycle role")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := lifecycle.Close(ctx); err != nil {
			t.Errorf("close Providers: %v", err)
		}
	})

	for attempt := 1; attempt <= 2; attempt++ {
		result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:           "cursor-acp",
			AttemptID:          "persistent-attempt",
			UserMessage:        "complete one persistent ACP turn",
			WorkingDirectory:   workingDirectory,
			ProcessEnvironment: os.Environ(),
			SkipPermissions:    true,
		})
		if executeErr != nil {
			t.Fatalf("Execute(%d) error = %v", attempt, executeErr)
		}
		if result.SessionRef == nil || result.SessionRef.ID != "acp-session-functional-1-"+strconv.Itoa(attempt) {
			t.Fatalf("Execute(%d) session = %#v", attempt, result.SessionRef)
		}
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want one retained OS process", got)
	}
}

func TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "version")
	var starts atomic.Int32
	root, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers: %v", err)
	}
	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider: "cursor-acp", AttemptID: "version-attempt", UserMessage: "reject version",
		WorkingDirectory: t.TempDir(), ProcessEnvironment: os.Environ(),
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) || failure.Kind != providers.ExecuteFailureKindMisconfigured {
		t.Fatalf("Execute() error = %#v, want misconfigured", executeErr)
	}
}
