package session

import (
	"encoding/json"
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

	process, server, daemon := startSessionProcess(t, dir)
	defer daemon.Stop(t)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "list", "--server", server.WaitForURL(t),
	})
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

	process, server, daemon := startSessionProcess(t, dir)
	defer daemon.Stop(t)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "list", "--json", "--server", server.WaitForURL(t),
	})
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
	err := process.Execute(createInputs.Input)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("Process.Execute(session create --json) error = %v, want accepted inherited flag and unreachable endpoint", err)
	}
}

func startSessionProcess(
	t *testing.T,
	dir string,
) (support.Process, *support.ProcessAPIServer, *support.ProcessCommand) {
	t.Helper()
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
	daemon := support.StartProcessCommand(t, process, daemonInputs.Input)
	return process, server, daemon
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}
