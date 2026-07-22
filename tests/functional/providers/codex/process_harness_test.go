package codex

import (
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootBuiltProcessExecutesThroughSharedSupport(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Run and manage") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want root command help", inputs.Stdout())
	}
}
