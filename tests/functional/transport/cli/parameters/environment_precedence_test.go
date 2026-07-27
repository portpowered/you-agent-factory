package parameters_test

import (
	"os"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIExplicitFlagOverridesEnvironmentDefault proves an explicit CLI flag
// wins over a conflicting environment default for the same documented operator
// setting, with the flag-resolved value observable at the CLI resolution edge
// and the environment value not selected.
func TestCLIExplicitFlagOverridesEnvironmentDefault(t *testing.T) {
	home := t.TempDir()
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--default-worker-model-provider", "gemini",
		"--default-worker-model", "flag-gemini-model",
		"docs", "agents",
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		operatorsettings.EnvDefaultWorkerModelProvider+"=codex",
		operatorsettings.EnvDefaultWorkerModel+"=environment-model",
	)
	inputs.Input.WorkingDirectory = home

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(docs agents with operator defaults) error = %v", err)
	}

	assertResolvedOperatorDefault(
		t,
		observation.ResolvedInputs,
		"you.flag.default-worker-model-provider",
		"gemini",
		resolvedinput.SourceCLIFlag,
	)
	assertResolvedOperatorDefault(
		t,
		observation.ResolvedInputs,
		"you.flag.default-worker-model",
		"flag-gemini-model",
		resolvedinput.SourceCLIFlag,
	)
	assertOperatorDefaultNotFromSource(
		t,
		observation.ResolvedInputs,
		"you.flag.default-worker-model-provider",
		resolvedinput.SourceEnvironment,
		"codex",
	)
	assertOperatorDefaultNotFromSource(
		t,
		observation.ResolvedInputs,
		"you.flag.default-worker-model",
		resolvedinput.SourceEnvironment,
		"environment-model",
	)
}

func assertResolvedOperatorDefault(
	t *testing.T,
	observations []resolvedinput.Observation,
	inputID string,
	wantValue string,
	wantSource resolvedinput.Source,
) {
	t.Helper()
	observation, found := resolvedOperatorDefaultObservation(observations, inputID)
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

func assertOperatorDefaultNotFromSource(
	t *testing.T,
	observations []resolvedinput.Observation,
	inputID string,
	rejectedSource resolvedinput.Source,
	rejectedValue string,
) {
	t.Helper()
	observation, found := resolvedOperatorDefaultObservation(observations, inputID)
	if !found {
		t.Fatalf("%s observation is missing", inputID)
	}
	if observation.Provenance == rejectedSource && observation.Value == rejectedValue {
		t.Fatalf(
			"%s observation = %#v, want source other than %q with value other than %q",
			inputID,
			observation,
			rejectedSource,
			rejectedValue,
		)
	}
}

func resolvedOperatorDefaultObservation(
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
