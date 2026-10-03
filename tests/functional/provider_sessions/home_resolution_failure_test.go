package provider_sessions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
)

// A home lookup failure prevents construction, so no detail request can be
// issued. Keep this separate from the reusable successful process: its immutable
// home edge intentionally cannot construct that process.
func TestProviderSessionsHomeResolutionFailureOutcome(t *testing.T) {
	t.Parallel()

	recorder := newProviderSessionEffectRecorder(t)
	edges := recorder.edges()
	want := errors.New("provider-session home lookup failed")
	edges.ProviderSessionResolveHomeDirectory = func() (string, error) {
		recorder.homeCalls.Add(1)
		return "", want
	}

	process, err := root.BuildProcess(context.Background(), edges)
	if process != nil {
		t.Cleanup(func() { _ = process.Close(context.Background()) })
		t.Fatal("BuildProcess returned a process after home resolution failed")
	}
	if !errors.Is(err, want) {
		t.Fatalf("BuildProcess error = %v, want wrapped home lookup sentinel", err)
	}
	if !strings.Contains(err.Error(), "home directory:") {
		t.Fatalf("BuildProcess error = %v, want home directory diagnostic", err)
	}
	if got := recorder.homeCalls.Load(); got != 1 {
		t.Fatalf("home lookup calls = %d, want one failed lookup", got)
	}
	for name, calls := range map[string]int32{
		"candidate stat":  recorder.fileStatCalls.Load(),
		"file open":       recorder.fileOpenCalls(),
		"Codex walk":      recorder.codexWalkCalls(),
		"Codex symlink":   recorder.codexSymlinkCalls(),
		"Cursor walk":     recorder.cursorWalkCalls(),
		"Cursor symlink":  recorder.cursorSymlinkCalls(),
		"Cursor database": recorder.cursorDatabaseCalls(),
	} {
		if calls != 0 {
			t.Errorf("%s calls = %d after failed home lookup, want 0", name, calls)
		}
	}
}
