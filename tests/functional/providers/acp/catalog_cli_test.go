package acp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestCustomersCanAddListAndDeleteACPWorkers(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	home := t.TempDir()
	working := t.TempDir()

	added := executeACPCommand(t, process, home, working,
		"workers", "acp", "add",
		"--name", "custom-acp",
		"--transport", "stdio",
		"--argument", `custom-agent --profile "team alpha" acp`,
	)
	if !strings.Contains(added, "install succeeded: custom-acp") {
		t.Fatalf("add output = %q", added)
	}

	listed := executeACPCommand(t, process, home, working, "workers", "list")
	if !strings.Contains(listed, "custom-acp") {
		t.Fatalf("workers list omitted custom-acp: %q", listed)
	}

	deleted := executeACPCommand(t, process, home, working,
		"workers", "acp", "delete", "--name", "custom-acp",
	)
	if !strings.Contains(deleted, "deleted ACP provider custom-acp") {
		t.Fatalf("delete output = %q", deleted)
	}
	listed = executeACPCommand(t, process, home, working, "workers", "list")
	if strings.Contains(listed, "custom-acp") {
		t.Fatalf("workers list retained deleted provider: %q", listed)
	}

	functionalevidence.Covers(t,
		"cli/you.workers.acp.add",
		"cli/you.workers.acp.delete",
		"cli/you.workers.list",
	)
}

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
