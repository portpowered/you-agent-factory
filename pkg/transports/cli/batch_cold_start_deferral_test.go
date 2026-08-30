package cli

import (
	"context"
	"errors"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

var profileSelectedBatchSystemInitializationCases = []struct {
	name         string
	cfg          runcli.RunConfig
	changedFlag  string
	wantDeferred bool
}{
	{
		name: "finite mock no-record batch",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Port:                    7437,
		},
		wantDeferred: true,
	},
	{
		name: "explicit recording",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: false,
			RecordPath:              "recording.jsonl",
		},
	},
	{
		name: "replay",
		cfg: runcli.RunConfig{
			WorkFile:           "one-work.json",
			MockWorkersEnabled: true,
			ReplayPath:         "recording.jsonl",
		},
	},
	{
		name: "server",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			WithServer:              true,
		},
	},
	{
		name: "continuous",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Continuously:            true,
		},
	},
	{
		name: "real workers",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			DisableDefaultRecording: true,
		},
	},
	{
		name: "bootstrap",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Bootstrap:               true,
		},
	},
	{
		name: "named Factory",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			NamedFactoryName:        "@you/goal",
		},
	},
	{
		name: "listener",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			ListenExplicit:          true,
		},
	},
	{
		name:        "explicit factory path",
		changedFlag: "factory",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			FactoryConfigPath:       "factory.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
		},
	},
	{
		name:        "explicit factory directory",
		changedFlag: "dir",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			Dir:                     "factory",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
		},
	},
}

func TestPrepareRunSystemInitializationDefersOnlyProfileSelectedBatch(t *testing.T) {
	t.Parallel()

	for _, test := range profileSelectedBatchSystemInitializationCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "run"}
			cmd.SetContext(context.Background())
			if test.changedFlag != "" {
				cmd.Flags().String(test.changedFlag, "", "")
				if err := cmd.Flags().Set(test.changedFlag, "selected"); err != nil {
					t.Fatalf("set changed flag %q: %v", test.changedFlag, err)
				}
			}
			options := CommandFactory{
				initializer: startupcli.Functions{
					InitializeSystemFunc: func(context.Context, string) error { return nil },
				},
			}
			allowed, err := prepareRunSystemInitialization(cmd, &test.cfg, options)
			if err != nil {
				t.Fatalf("prepareRunSystemInitialization() error = %v", err)
			}
			if got := !allowed; got != test.wantDeferred {
				t.Fatalf("deferred = %t, want %t", got, test.wantDeferred)
			}
		})
	}
}

func TestDeferredBatchSystemInitializationDoesNotInvokeInitializer(t *testing.T) {
	t.Parallel()

	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{
		initializer: startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				calls++
				return nil
			},
		},
	}
	cfg := runcli.RunConfig{
		WorkFile:                "one-work.json",
		MockWorkersEnabled:      true,
		DisableDefaultRecording: true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepareRunFactoryStartup() error = %v", err)
	}
	if cfg.StartupPreparation == nil {
		t.Fatal("StartupPreparation is nil")
	}
	if err := cfg.StartupPreparation(context.Background(), false, nil); err != nil {
		t.Fatalf("deferred StartupPreparation() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("InitializeSystem calls = %d, want 0 for deferred batch", calls)
	}
}

func TestExactFiniteMockBatchCommandDefersSystemInitialization(t *testing.T) {
	var initialized int
	var got runcli.RunConfig
	workingDirectory := t.TempDir()
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				initialized++
				return nil
			},
			RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				got = testRunConfig(selection)
				return nil
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	root.SetArgs([]string{
		"run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--no-record",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute finite mock batch command: %v", err)
	}
	if initialized != 0 {
		t.Fatalf("InitializeSystem calls = %d, want 0 for exact finite mock batch command", initialized)
	}
	if got.WorkFile != "one-work.json" || !got.MockWorkersEnabled ||
		got.MockWorkersConfigPath != "accept.json" || !got.DisableDefaultRecording {
		t.Fatalf("parsed batch config = %+v, want work, mock worker, config path, and no-record inputs", got)
	}
}

func TestDemandedBatchSystemInitializationPreservesFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("controlled hosted system initialization failure")
	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{
		initializer: startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				calls++
				return wantErr
			},
		},
	}
	cfg := runcli.RunConfig{
		WorkFile:                "one-work.json",
		MockWorkersEnabled:      true,
		DisableDefaultRecording: true,
		WithServer:              true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepareRunFactoryStartup() error = %v", err)
	}
	if cfg.StartupPreparation == nil {
		t.Fatal("StartupPreparation is nil")
	}
	err := cfg.StartupPreparation(context.Background(), false, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("demanded StartupPreparation() error = %v, want sentinel %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("InitializeSystem calls = %d, want one demanded initialization", calls)
	}
}
