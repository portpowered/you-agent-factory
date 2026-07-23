package cli

import (
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolveOperatorDefaultsDelegatesExactObservedLayers(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "you"}
	root.PersistentFlags().String("default-worker-model-provider", "", "")
	root.PersistentFlags().String("default-worker-model", "", "")
	if err := root.PersistentFlags().Set("default-worker-model-provider", "codex"); err != nil {
		t.Fatal(err)
	}
	command := &cobra.Command{Use: "run"}
	root.AddCommand(command)

	want := operatorsettings.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-test"}
	called := false
	factory := CommandFactory{
		lookupEnv: func(name string) (string, bool) {
			if name == operatorsettings.EnvDefaultWorkerModel {
				return "gpt-test", true
			}
			return "", false
		},
		resolveOperatorDefaults: func(home string, environment operatorsettings.Defaults, flags operatorsettings.FlagOverrides) (operatorsettings.ResolvedDefaults, error) {
			called = true
			if home != "/home/customer" {
				t.Fatalf("home = %q", home)
			}
			if environment.WorkerModel != "gpt-test" || environment.WorkerModelProvider != "" {
				t.Fatalf("environment = %#v", environment)
			}
			if flags.WorkerModelProvider != "codex" || flags.WorkerModel != "" {
				t.Fatalf("flags = %#v", flags)
			}
			return want, nil
		},
	}
	got, err := resolveOperatorDefaults(command, &cliOperatorDefaultsOptions{defaultWorkerModelProvider: "codex"}, factory, "/home/customer")
	if err != nil {
		t.Fatal(err)
	}
	if !called || got != want {
		t.Fatalf("called = %t, result = %#v", called, got)
	}
}

func TestRootGlobalResolutionIsAvailableToAttachedCommandFamilies(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ModelsCLI: legacyModelsCLIService{},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	models, _, err := root.Find([]string{"models"})
	if err != nil {
		t.Fatalf("Find(models) error = %v", err)
	}
	var received resolvedinput.Inputs
	models.RunE = func(cmd *cobra.Command, _ []string) error {
		var resolveErr error
		received, resolveErr = climanifestcobra.ResolvedPersistentInputs(cmd)
		return resolveErr
	}
	root.SetArgs([]string{"--verbose", "models"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	value, err := received.Bool("you.flag.verbose")
	if err != nil || !value {
		t.Fatalf("resolved verbose = (%t, %v), want true", value, err)
	}
	state, ok := received.State("you.flag.verbose")
	if !ok || state.Provenance != resolvedinput.SourceCLIFlag || !state.Changed || state.Default {
		t.Fatalf("resolved verbose state = (%#v, %t), want changed CLI provenance", state, ok)
	}
}

func TestRunCompatibilityParsingRefreshesResolvedGlobals(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ModelsCLI: legacyModelsCLIService{},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	run, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	var received resolvedinput.Inputs
	run.RunE = func(cmd *cobra.Command, args []string) error {
		if _, parseErr := parseRunCommandArgs(cmd, args); parseErr != nil {
			return parseErr
		}
		if refreshErr := climanifestcobra.RefreshResolvedPersistentInputs(cmd); refreshErr != nil {
			return refreshErr
		}
		var resolveErr error
		received, resolveErr = climanifestcobra.ResolvedPersistentInputs(cmd)
		return resolveErr
	}
	root.SetArgs([]string{"run", "--default-worker-model-provider", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	value, err := received.String("you.flag.default-worker-model-provider")
	if err != nil || value != "codex" {
		t.Fatalf("resolved provider = (%q, %v), want codex", value, err)
	}
	state, ok := received.State("you.flag.default-worker-model-provider")
	if !ok || state.Provenance != resolvedinput.SourceCLIFlag || !state.Changed {
		t.Fatalf("resolved provider state = (%#v, %t), want changed CLI provenance", state, ok)
	}
}
