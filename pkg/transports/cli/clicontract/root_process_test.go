package clicontract

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

func productionCLIInventory(t testing.TB) commandidentity.Inventory {
	return productionCLISnapshot(t).Commands
}

func productionCLIInputs(t testing.TB) cliinputs.Inventory {
	return productionCLISnapshot(t).Inputs
}

func productionCLISnapshot(t testing.TB) cliobservation.Snapshot {
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
	return result.Snapshot
}

func TestProductionRootResolvesOperatorConfigWithManifestPrecedence(t *testing.T) {
	home := t.TempDir()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	err = process.Execute(root.Input{
		Args:             []string{"you", "docs", "agents", "--default-worker-model", "cli-model"},
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed default model flag error = %v, want unknown flag", err)
	}
}
