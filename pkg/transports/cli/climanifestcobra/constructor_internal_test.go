package climanifestcobra

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestLocalBindingTargetParsesIntDefaults(t *testing.T) {
	target, err := localBindingTarget(climanifest.Flag{
		Long:      "count",
		ValueType: "int",
		Default:   "3",
	})
	if err != nil {
		t.Fatalf("localBindingTarget() error = %v", err)
	}
	if target.intValue == nil || *target.intValue != 3 {
		t.Fatalf("int binding = %#v, want 3", target.intValue)
	}
}

func TestSessionLocalBindingTargetsRejectInheritedJSON(t *testing.T) {
	flag := climanifest.Flag{Long: "json"}
	tests := []struct {
		name   string
		target func() error
	}{
		{
			name: "create",
			target: func() error {
				_, err := sessionCreateFlagTarget(flag, &sessioncli.CreateConfig{})
				return err
			},
		},
		{
			name: "list",
			target: func() error {
				_, err := sessionListFlagTarget(flag, &sessioncli.ListConfig{})
				return err
			},
		},
		{
			name: "delete",
			target: func() error {
				_, err := sessionDeleteFlagTarget(flag, &sessioncli.DeleteConfig{})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.target(); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("local JSON target error = %v, want unsupported inherited flag", err)
			}
		})
	}
}

func TestRegisterFlagSupportsBoolStringAndInt(t *testing.T) {
	t.Run("bool shorthand", func(t *testing.T) {
		var value bool
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "verbose",
			Shorthand: "v",
			ValueType: "bool",
			Default:   "true",
		}, flagTarget{boolValue: &value}, "verbose help"); err != nil {
			t.Fatalf("registerFlag(bool) error = %v", err)
		}
		if err := flags.Set("verbose", "true"); err != nil {
			t.Fatalf("Set(verbose) error = %v", err)
		}
		if !value {
			t.Fatal("bool flag did not bind")
		}
	})

	t.Run("string", func(t *testing.T) {
		var value string
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "server",
			ValueType: "string",
			Default:   "http://localhost:7437",
		}, flagTarget{stringValue: &value}, "server help"); err != nil {
			t.Fatalf("registerFlag(string) error = %v", err)
		}
		if value != "http://localhost:7437" {
			t.Fatalf("string default = %q", value)
		}
	})

	t.Run("int", func(t *testing.T) {
		var value int
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "retries",
			ValueType: "int",
			Default:   "2",
		}, flagTarget{intValue: &value}, "retries help"); err != nil {
			t.Fatalf("registerFlag(int) error = %v", err)
		}
		if value != 2 {
			t.Fatalf("int default = %d, want 2", value)
		}
	})
}

func TestPositionalArgsFromManifestCoversCardinalityModes(t *testing.T) {
	exact := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, MaxCardinality: 1},
		},
	})
	cmd := &cobra.Command{Use: "exact", Args: exact}
	if err := cmd.Args(cmd, []string{"one"}); err != nil {
		t.Fatalf("exact args error = %v", err)
	}

	variadic := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, Variadic: true},
		},
	})
	cmd = &cobra.Command{Use: "variadic", Args: variadic}
	if err := cmd.Args(cmd, []string{"one", "two"}); err != nil {
		t.Fatalf("variadic args error = %v", err)
	}

	maxOnly := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MaxCardinality: 2},
		},
	})
	cmd = &cobra.Command{Use: "max", Args: maxOnly}
	if err := cmd.Args(cmd, []string{"one", "two"}); err != nil {
		t.Fatalf("max args error = %v", err)
	}

	twoRequired := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, MaxCardinality: 1},
			"arg1": {Position: 1, MinCardinality: 1, MaxCardinality: 1},
		},
	})
	cmd = &cobra.Command{Use: "move", Args: twoRequired}
	if err := cmd.Args(cmd, []string{"work-1", "ready"}); err != nil {
		t.Fatalf("two required args error = %v", err)
	}
	if err := cmd.Args(cmd, []string{"work-1"}); err == nil {
		t.Fatal("two required args accepted one positional, want rejection")
	}
}

func TestRegisterManifestLocalFlagsRejectsUnsupportedValueType(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	err := registerManifestLocalFlags(cmd, climanifest.Command{
		ID: "you.models.list",
		Flags: map[string]climanifest.Flag{
			"you.models.list.flag.foo": {
				Long:      "foo",
				Scope:     "local",
				ValueType: "duration",
			},
		},
	})
	if err == nil {
		t.Fatal("registerManifestLocalFlags() unsupported type = nil, want error")
	}
}

func TestBuildCommandFromRecordAppliesHiddenVisibility(t *testing.T) {
	cmd, err := buildCommandFromRecord(climanifest.Command{
		ID:         "you.session.show",
		Usage:      climanifest.Usage{Line: "show"},
		Visibility: "hidden",
		Documentation: climanifest.Documentation{
			Documentation: climanifest.DocumentationCopy{
				Title:       climanifest.DocumentationField{CanonicalEnglish: "title"},
				Description: climanifest.DocumentationField{CanonicalEnglish: "description"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildCommandFromRecord() error = %v", err)
	}
	if !cmd.Hidden {
		t.Fatal("hidden command must set cmd.Hidden")
	}
}

func TestApplyFlagContractSetsHiddenAndNoOptDefault(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "json")
	if err := applyFlagContract(flags.Lookup("json"), climanifest.Flag{
		Long:            "json",
		Visibility:      "hidden",
		NoOptionDefault: "true",
	}); err != nil {
		t.Fatalf("applyFlagContract() error = %v", err)
	}
	flag := flags.Lookup("json")
	if flag == nil || !flag.Hidden || flag.NoOptDefVal != "true" {
		t.Fatalf("flag = %#v, want hidden with no-opt default", flag)
	}
}

func TestRegisterLocalFlagsRegistersDeprecatedPort(t *testing.T) {
	cmd := &cobra.Command{Use: "show"}
	if err := registerLocalFlags(cmd, climanifest.Command{
		ID: "you.session.show",
		Flags: map[string]climanifest.Flag{
			"you.session.show.flag.port": {
				Long:       "port",
				Scope:      "local",
				ValueType:  "int",
				Default:    "0",
				Visibility: "hidden",
			},
		},
	}, PersistentFlagBindings{}); err != nil {
		t.Fatalf("registerLocalFlags() error = %v", err)
	}
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil || !portFlag.Hidden {
		t.Fatalf("port flag = %#v, want hidden deprecated local flag", portFlag)
	}
}

func TestRegisterPersistentFlagsRegistersRootBindings(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	bindings := PersistentFlagBindings{
		Verbose:                    boolPtr(false),
		Debug:                      boolPtr(false),
		Server:                     stringPtr("http://localhost:7437"),
		JSON:                       boolPtr(false),
		DefaultWorkerModelProvider: stringPtr(""),
		DefaultWorkerModel:         stringPtr(""),
	}
	if err := registerPersistentFlags(root, climanifest.Command{
		ID: "you",
		Flags: map[string]climanifest.Flag{
			"you.flag.verbose": {Long: "verbose", Shorthand: "v", Scope: "persistent", ValueType: "bool", Default: "false"},
			"you.flag.server":  {Long: "server", Scope: "persistent", ValueType: "string", Default: "http://localhost:7437"},
		},
	}, bindings); err != nil {
		t.Fatalf("registerPersistentFlags() error = %v", err)
	}
	if root.PersistentFlags().Lookup("verbose") == nil || root.PersistentFlags().Lookup("server") == nil {
		t.Fatal("expected root persistent flags to register")
	}
}

func TestRejectDeprecatedPortFlagAllowsUnsetPort(t *testing.T) {
	cmd := &cobra.Command{Use: "show"}
	var port int
	registerDeprecatedPortFlag(cmd, &port)
	if err := rejectDeprecatedPortFlag(cmd, nil); err != nil {
		t.Fatalf("rejectDeprecatedPortFlag() unset port error = %v", err)
	}
}

func TestRepresentativeManifestRecordsRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		Commands: map[string]climanifest.Command{
			"you": {ID: "you"},
		},
	}
	if _, _, _, err := representativeManifestRecords(manifest); err == nil {
		t.Fatal("representativeManifestRecords() missing commands = nil, want error")
	}
}

func boolPtr(value bool) *bool { return &value }

func stringPtr(value string) *string { return &value }

func TestPersistentBindingTargetRejectsUnknownFlag(t *testing.T) {
	if _, err := persistentBindingTarget("unknown-flag", PersistentFlagBindings{}); err == nil {
		t.Fatal("persistentBindingTarget() unknown flag = nil, want error")
	}
}

func TestRegisterFlagRejectsMissingBindings(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := registerFlag(flags, climanifest.Flag{
		Long:      "verbose",
		ValueType: "bool",
		Default:   "false",
	}, flagTarget{}, "help"); err == nil {
		t.Fatal("registerFlag() missing bool binding = nil, want error")
	}
}

func TestNewRepresentativeFamilyComponentsLoadsEmbeddedManifest(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you", func(cmd *cobra.Command, args []string) error { return nil }); err != nil {
		t.Fatalf("Register(you) error = %v", err)
	}
	if err := registry.Register("you.session.show", func(cmd *cobra.Command, args []string) error { return nil }); err != nil {
		t.Fatalf("Register(you.session.show) error = %v", err)
	}
	components, err := NewRepresentativeFamilyComponents(registry, PersistentFlagBindings{
		Verbose:                    boolPtr(false),
		Debug:                      boolPtr(false),
		Server:                     stringPtr("http://localhost:7437"),
		JSON:                       boolPtr(false),
		DefaultWorkerModelProvider: stringPtr(""),
		DefaultWorkerModel:         stringPtr(""),
	})
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyComponents() error = %v", err)
	}
	if components.Show == nil {
		t.Fatal("expected show component from generated manifest")
	}
}

func TestLocalBindingTargetRejectsUnsupportedValueType(t *testing.T) {
	if _, err := localBindingTarget(climanifest.Flag{
		Long:      "name",
		ValueType: "string",
	}); err == nil {
		t.Fatal("localBindingTarget(string) = nil, want error")
	}
}

func TestRegisterFlagRejectsUnsupportedValueType(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := registerFlag(flags, climanifest.Flag{
		Long:      "weird",
		ValueType: "float",
	}, flagTarget{}, "help"); err == nil {
		t.Fatal("registerFlag(float) = nil, want error")
	}
}
