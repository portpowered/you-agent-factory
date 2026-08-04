package root

import (
	"context"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestBuildProcessClosesDeterministicallyAndIdempotently proves canonical
// application construction (root.BuildProcess) composes the Events service's
// shutdown into the same single application-process lifecycle Providers
// already used, exactly as cmd/factory/main.go invokes it after
// Process.Execute: Close succeeds, and a repeated Close remains a safe
// no-op. It does not itself assert on Events' internal outcome (that is
// pkg/wire's own focused shared-identity coverage); this proves the real,
// end-to-end BuildProcess-composed shutdown path returns cleanly.
func TestBuildProcessClosesDeterministicallyAndIdempotently(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if process == nil {
		t.Fatal("BuildProcess() returned nil process")
	}

	ctx := context.Background()
	if err := process.Close(ctx); err != nil {
		t.Fatalf("Process.Close() error = %v", err)
	}
	if err := process.Close(ctx); err != nil {
		t.Fatalf("second Process.Close() error = %v, want idempotent no-op", err)
	}
}
