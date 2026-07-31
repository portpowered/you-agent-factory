package cli

import (
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolveOperatorDefaultsDelegatesExactObservedLayers(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "you"}
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
	got, err := resolveOperatorDefaults(command, &cliOperatorDefaultsOptions{providerOverride: "codex"}, factory, "/home/customer")
	if err != nil {
		t.Fatal(err)
	}
	if !called || got != want {
		t.Fatalf("called = %t, result = %#v", called, got)
	}
}

func TestRootGlobalResolutionIsAvailableToAttachedCommandFamilies(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ModelsCLI: rootModelsCLI,
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{},
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

func TestRunParsingResolvesRunScopedProvider(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ModelsCLI: rootModelsCLI,
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{},
	)
	run, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	if run.Flags().Lookup("provider") == nil {
		t.Fatal("run is missing --provider")
	}
	if root.PersistentFlags().Lookup("default-worker-model-provider") != nil {
		t.Fatal("root still exposes --default-worker-model-provider")
	}
}
