package cliinputs_test

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
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

func TestProductionCLIObservationReportsResolvedGlobalsByStableManifestID(t *testing.T) {
	defaulted, err := productionCLIObservation(t, "session", "list")
	if err != nil {
		t.Fatalf("observe defaulted globals: %v", err)
	}
	assertResolvedGlobal(t, defaulted, "you.flag.verbose", false, resolvedinput.SourceManifestDefault, false)
	assertResolvedGlobal(t, defaulted, "you.flag.server", "http://localhost:7437", resolvedinput.SourceManifestDefault, false)

	changed, err := productionCLIObservation(t, "session", "-v", "list", "--server", "https://factory.example")
	if err != nil {
		t.Fatalf("observe changed globals: %v", err)
	}
	assertResolvedGlobal(t, changed, "you.flag.verbose", true, resolvedinput.SourceCLIFlag, true)
	assertResolvedGlobal(t, changed, "you.flag.server", "https://factory.example", resolvedinput.SourceCLIFlag, true)
}

func TestProductionCLIObservationResolvesGlobalsAcrossStaticFamilyDepths(t *testing.T) {
	defaulted, err := productionCLIObservation(t, "docs", "run")
	if err != nil {
		t.Fatalf("observe top-level static family: %v", err)
	}
	assertResolvedGlobal(t, defaulted, "you.flag.server", "http://localhost:7437", resolvedinput.SourceManifestDefault, false)

	changed, err := productionCLIObservation(
		t,
		"factory", "list", "--dir", "factory",
		"--server", "https://factory.example",
	)
	if err != nil {
		t.Fatalf("observe deep static family: %v", err)
	}
	assertResolvedGlobal(t, changed, "you.flag.server", "https://factory.example", resolvedinput.SourceCLIFlag, true)
}

func TestProductionCLIObservationRejectsRetiredRootGlobalWithoutResolvedSnapshot(t *testing.T) {
	observation, err := productionCLIObservation(t, "--api-port", "9000", "session", "list")
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --api-port") {
		t.Fatalf("retired root global error = %v, want standard unknown-flag diagnostic", err)
	}
	if len(observation.ResolvedInputs) != 0 {
		t.Fatalf("resolved inputs after parser rejection = %#v, want none", observation.ResolvedInputs)
	}
}

func assertResolvedGlobal(
	t *testing.T,
	observation cliobservation.Result,
	inputID string,
	wantValue any,
	wantSource resolvedinput.Source,
	wantChanged bool,
) {
	t.Helper()
	for _, input := range observation.ResolvedInputs {
		if input.InputID != inputID {
			continue
		}
		if !reflect.DeepEqual(input.Value, wantValue) ||
			input.Provenance != wantSource ||
			input.Changed != wantChanged ||
			input.Default != (wantSource == resolvedinput.SourceManifestDefault) {
			t.Fatalf(
				"resolved input %q = %#v, want value=%#v source=%q changed=%t",
				inputID,
				input,
				wantValue,
				wantSource,
				wantChanged,
			)
		}
		return
	}
	t.Fatalf("resolved input %q missing from %#v", inputID, observation.ResolvedInputs)
}
