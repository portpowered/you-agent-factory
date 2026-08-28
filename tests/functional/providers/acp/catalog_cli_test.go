package acp_test

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func executeACPCommand(t *testing.T, process support.Process, home, working string, args ...string) string {
	t.Helper()
	inputs := support.FakeInputs(context.Background(), append([]string{"you"}, args...))
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = working
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute %v: %v; stderr=%s", args, err, inputs.Stderr())
	}
	return inputs.Stdout()
}
