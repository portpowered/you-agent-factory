package clicontract

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

func productionCLIInventory(t testing.TB) commandidentity.Inventory {
	t.Helper()
	var result cliobservation.Result
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{CLIObserver: cliobservation.Capture(&result)})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	err = process.Execute(root.Input{
		Args: []string{"you"}, Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	if err != nil {
		t.Fatalf("Process.Execute(observe CLI): %v", err)
	}
	return result.Snapshot.Commands
}
