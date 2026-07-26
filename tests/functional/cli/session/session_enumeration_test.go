package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionEnumeration(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	support.SetWorkingDirectory(t, dir)

	process, server, daemon, env := startSessionProcess(t, dir)
	defer daemon.Stop(t)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "list", "--server", server.WaitForURL(t),
	})
	inputs.Input.Env = env
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session list) error = %v; stderr=%s", err, inputs.Stderr())
	}
	if got := inputs.Stdout(); !contains(got, dir) {
		t.Errorf("session list output = %q, want copied fixture directory %q", got, dir)
	}
}

func TestSessionEnumerationJSON(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	support.SetWorkingDirectory(t, dir)

	process, server, daemon, env := startSessionProcess(t, dir)
	defer daemon.Stop(t)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "list", "--json", "--server", server.WaitForURL(t),
	})
	inputs.Input.Env = env
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session list --json) error = %v; stderr=%s", err, inputs.Stderr())
	}

	var sessions factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &sessions); err != nil {
		t.Fatalf("unmarshal session output %q: %v", inputs.Stdout(), err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(sessions.Sessions))
	}
	if sessions.Sessions[0].Id == "" || sessions.Sessions[0].Runtime == nil {
		t.Fatalf("session missing id or runtime: %#v", sessions.Sessions[0])
	}
	if sessions.Sessions[0].FolderPath != dir {
		t.Fatalf("session folder path = %q, want %q", sessions.Sessions[0].FolderPath, dir)
	}

	createInputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "create", "--json", "--dir", dir, "--port", "1",
	})
	createInputs.Input.Env = env
	err := process.Execute(createInputs.Input)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("Process.Execute(session create --json) error = %v, want accepted inherited flag and unreachable endpoint", err)
	}
}

// TestSessionCreateInitializesNewFactoryThroughSupportedAPI proves the supported session API creates a new Factory.
func TestSessionCreateInitializesNewFactoryThroughSupportedAPI(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	support.SetWorkingDirectory(t, dir)

	process, server, daemon, env := startSessionProcess(t, dir)
	defer daemon.Stop(t)

	newFactoryDir := filepath.Join(t.TempDir(), "new-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create empty Factory directory: %v", err)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "create",
		"--json",
		"--dir", newFactoryDir,
		"--init-new-factory",
		"--server", server.WaitForURL(t),
	})
	inputs.Input.Env = env
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(session create --init-new-factory) error = %v; stderr=%s",
			err,
			inputs.Stderr(),
		)
	}

	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &opened); err != nil {
		t.Fatalf("unmarshal session create output %q: %v", inputs.Stdout(), err)
	}
	if opened.Session.Id == "" || opened.Session.FolderPath != newFactoryDir {
		t.Fatalf("opened session = %#v, want new Factory at %q", opened.Session, newFactoryDir)
	}
	scaffoldDir := filepath.Join(newFactoryDir, "factory")
	for _, relativePath := range []string{
		"factory.json",
		filepath.Join("workers", "processor", "AGENTS.md"),
		filepath.Join("workstations", "process", "AGENTS.md"),
		filepath.Join("inputs", "task", "default"),
	} {
		if _, err := os.Stat(filepath.Join(scaffoldDir, relativePath)); err != nil {
			t.Fatalf("initialized Factory path %q missing: %v", relativePath, err)
		}
	}
}

func startSessionProcess(
	t *testing.T,
	dir string,
) (support.Process, *support.ProcessAPIServer, *support.ProcessCommand, []string) {
	t.Helper()
	homeDir := t.TempDir()
	env := append(
		os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
	)
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      server.Start,
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("primary result COMPLETE"),
	})
	daemonInputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	daemonInputs.Input.Env = env
	daemon := support.StartProcessCommand(t, process, daemonInputs.Input)
	return process, server, daemon, env
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}
