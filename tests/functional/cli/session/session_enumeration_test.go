package session

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	"go.uber.org/zap"
)

func BasicCliInputWithArgs(t *testing.T, args []string) root.Input {
	return root.Input{
		Args:    args,
		Env:     os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: t.Context(),
	}
}

func basicServer(t *testing.T, dir string) *support.FunctionalAPIServer {
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(captured *service.FactoryService) {
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
			cfg.Logger = zap.NewNop()
		},
	})
}

func TestSessionEnumeration(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	support.SetWorkingDirectory(t, dir)

	// Act

	// Instantiate the server
	server := basicServer(t, dir)

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "session", "list", "--server", server.URL()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert

	if !bytes.Contains(output.Bytes(), []byte(dir)) {
		t.Errorf("expected output to contain copied fixture directory %q, got: %s", dir, output.String())
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestSessionEnumerationJson(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	support.SetWorkingDirectory(t, dir)

	// Act

	// Instantiate the server
	server := basicServer(t, dir)

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "session", "list", "--json", "--server", server.URL()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert

	var session factoryapi.ListFactorySessionsResponse
	err := json.Unmarshal(output.Bytes(), &session)
	if err != nil {
		t.Fatalf("failed to unmarshal session output: %v", err)
	}
	if len(session.Sessions) != 1 {
		t.Fatalf("expected at least one session, got 0")
	}

	if (session.Sessions[0].Id == "") || (session.Sessions[0].Runtime == nil) {
		t.Fatalf("expected session to have id and runtime, got: %#v", session.Sessions[0])
	}

	if session.Sessions[0].FolderPath != dir {
		t.Fatalf("expected session folder path to be %q, got: %q", dir, session.Sessions[0].FolderPath)
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}
