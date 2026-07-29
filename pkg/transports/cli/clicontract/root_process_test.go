package clicontract

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
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

type operatorConfigResolutionCase struct {
	name        string
	args        []string
	environment []string
	wantValue   string
	wantSource  resolvedinput.Source
}

type operatorConfigFileSystem struct {
	payload []byte
}

func (files operatorConfigFileSystem) ReadFile(string) ([]byte, error) {
	return append([]byte(nil), files.payload...), nil
}

func (operatorConfigFileSystem) MkdirAll(string, os.FileMode) error {
	return fmt.Errorf("unexpected operator config directory creation")
}

func (operatorConfigFileSystem) Remove(string) error {
	return fmt.Errorf("unexpected operator config removal")
}

func (operatorConfigFileSystem) Chmod(string, os.FileMode) error {
	return fmt.Errorf("unexpected operator config permission change")
}

func (operatorConfigFileSystem) Rename(string, string) error {
	return fmt.Errorf("unexpected operator config rename")
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

func assertOperatorConfigResolution(t *testing.T, test operatorConfigResolutionCase) {
	t.Helper()
	home := t.TempDir()
	var result cliobservation.Result
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&result),
		OperatorSettingsFileSystem: operatorConfigFileSystem{payload: []byte(
			`{"defaults":{"workerModelProvider":"codex","workerModel":"file-model"}}`,
		)},
	})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	environment := append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		operatorsettings.EnvDefaultWorkerModelProvider+"=",
		operatorsettings.EnvDefaultWorkerModel+"=",
	)
	environment = append(environment, test.environment...)
	if err := process.Execute(root.Input{
		Args: test.args, Env: environment, WorkingDirectory: home,
		Stdout: io.Discard, Stderr: io.Discard,
	}); err != nil {
		t.Fatalf("Process.Execute(observe CLI): %v", err)
	}

	assertResolvedObservation(
		t,
		result.ResolvedInputs,
		"you.flag.default-worker-model",
		test.wantValue,
		test.wantSource,
	)
	assertResolvedObservation(
		t,
		result.ResolvedInputs,
		"you.flag.default-worker-model-provider",
		"codex",
		resolvedinput.SourceOperatorConfig,
	)
}

func assertResolvedObservation(
	t *testing.T,
	observations []resolvedinput.Observation,
	inputID string,
	wantValue string,
	wantSource resolvedinput.Source,
) {
	t.Helper()
	observation, found := resolvedObservation(observations, inputID)
	if !found {
		t.Fatalf("%s observation is missing", inputID)
	}
	if observation.Value != wantValue ||
		observation.Provenance != wantSource ||
		!observation.Changed ||
		observation.Default {
		t.Fatalf(
			"%s observation = %#v, want value %q changed non-default source %q",
			inputID,
			observation,
			wantValue,
			wantSource,
		)
	}
}

func resolvedObservation(
	observations []resolvedinput.Observation,
	inputID string,
) (resolvedinput.Observation, bool) {
	for _, observation := range observations {
		if observation.InputID == inputID {
			return observation, true
		}
	}
	return resolvedinput.Observation{}, false
}
