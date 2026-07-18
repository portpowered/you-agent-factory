package session

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func cliInputWithArgs(t *testing.T, args []string) root.Input {
	t.Helper()

	return root.Input{
		Args:    args,
		Env:     os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: t.Context(),
	}
}

func startSessionListHost(t *testing.T, factoryRoot string) *support.RootRunFunctionalHost {
	t.Helper()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: factoryRoot,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})
	return host
}

func TestSessionEnumeration(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	host := startSessionListHost(t, dir)

	// Act
	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := cliInputWithArgs(t, []string{"you", "session", "list", "--server", host.Endpoint()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert

	if !bytes.Contains(output.Bytes(), []byte(dir)) {
		t.Errorf("expected output to contain copied fixture directory %q, got: %s", dir, output.String())
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestSessionEnumerationJson(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	host := startSessionListHost(t, dir)

	// Act
	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := cliInputWithArgs(t, []string{"you", "session", "list", "--json", "--server", host.Endpoint()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}

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

}
