package baseline_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

func productionCLIObservation(t testing.TB, arguments ...string) (cliobservation.Result, error) {
	t.Helper()
	var result cliobservation.Result
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{CLIObserver: cliobservation.Capture(&result)})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	err = process.Execute(root.Input{
		Args: append([]string{"you"}, arguments...), Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	return result, err
}

func executeProductionCLI(t testing.TB, arguments ...string) (string, error) {
	t.Helper()
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	var stdout bytes.Buffer
	err = process.Execute(root.Input{
		Args: append([]string{"you"}, arguments...), Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: &stdout, Stderr: io.Discard,
	})
	return stdout.String(), err
}
