package cli

import (
	"io"
	"strings"
	"testing"
)

// Hermetic S02 failure-baseline fixtures for one-shot run invocation when the
// operator-level DEFAULT worker model provider cannot resolve to a concrete value.

func TestFailureBaseline_AbsentDefault_RunCommandRejectsUnresolvedDefaultProvider(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT", "--no-record"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unresolved DEFAULT provider error")
	}
	if !strings.Contains(err.Error(), "DEFAULT requires a concrete provider") {
		t.Fatalf("error = %q, want unresolved DEFAULT guidance", err.Error())
	}
}
