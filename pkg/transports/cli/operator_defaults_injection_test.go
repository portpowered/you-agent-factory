package cli

import (
	"os"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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

func TestRootDoesNotSelectOperatorSettingsImplementation(t *testing.T) {
	t.Parallel()
	payload, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"operatorconfig.ResolveFromHomeWithEnvironment", "operatorconfig.ResolveFromHome("} {
		if strings.Contains(string(payload), forbidden) {
			t.Errorf("root.go contains owner implementation call %q", forbidden)
		}
	}
}
