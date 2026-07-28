package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestRootBuiltACPCommandsAddListDeleteOneSettingsBackedCatalogEntry(t *testing.T) {
	home := t.TempDir()
	working := t.TempDir()
	process := support.BuildProcess(t, serviceedges.Edges{
		OperatorSettingsIDGenerator: func() string { return "integration-1" },
		ProvidersExecutableLocator:  availableExecutableLocator{},
	})

	add := executeACPCommand(t, process, home, working,
		"workers", "acp", "add",
		"--name", "custom-acp", "--transport", "stdio", "--argument", `custom-agent --profile "team alpha" acp`,
	)
	if !strings.Contains(add, "installed ACP provider custom-acp") {
		t.Fatalf("add output = %q", add)
	}

	listed := executeACPCommand(t, process, home, working, "workers", "acp", "list")
	if !strings.Contains(listed, "custom-acp") || !strings.Contains(listed, "ACP") || !strings.Contains(listed, "AVAILABLE") {
		t.Fatalf("list output omitted configured provider facts: %q", listed)
	}

	deleted := executeACPCommand(t, process, home, working, "workers", "acp", "delete", "--name", "custom-acp")
	if !strings.Contains(deleted, "deleted ACP provider custom-acp") {
		t.Fatalf("delete output = %q", deleted)
	}
	listed = executeACPCommand(t, process, home, working, "workers", "acp", "list")
	if strings.Contains(listed, "custom-acp") {
		t.Fatalf("list after delete retained configured provider: %q", listed)
	}

	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted operator config: %v", err)
	}
	if strings.Contains(string(data), "permission") || strings.Contains(string(data), "timeout") {
		t.Fatalf("ACP settings persisted forbidden policy fields: %s", data)
	}
	functionalevidence.Covers(
		t,
		"cli/you.workers.acp.add",
		"cli/you.workers.acp.delete",
		"cli/you.workers.acp.list",
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
