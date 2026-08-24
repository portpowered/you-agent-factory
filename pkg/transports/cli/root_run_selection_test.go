package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func TestResolveRunNamedFactorySelectionForwardsFailureToInjectedCandidatePaths(t *testing.T) {
	t.Parallel()

	const (
		homeDir     = "customer-home"
		workingDir  = "customer-repo"
		projectRoot = "detached-project-root"
		globalRoot  = "detached-global-root"
		name        = "@you/goal"
		projectPath = "detached-project-candidate"
		globalPath  = "detached-global-candidate"
	)
	blocking := interfaces.NewBlockingFactoryLoadError(interfaces.ValidationResult{
		Targets: []interfaces.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	catalog := rootNamedFactoryCatalogFake{resolve: func(gotProject, gotGlobal, gotName string) (*interfaces.NamedFactoryResolution, error) {
		if gotProject != projectRoot || gotGlobal != globalRoot || gotName != name {
			t.Fatalf("catalog request = (%q, %q, %q), want (%q, %q, %q)", gotProject, gotGlobal, gotName, projectRoot, globalRoot, name)
		}
		return nil, blocking
	}}
	candidateCalls := 0
	resolveCandidates := interfaces.NamedFactoryCandidatePathsResolver(func(gotProject, gotGlobal, gotName string) (interfaces.NamedFactoryCandidatePaths, error) {
		candidateCalls++
		if gotProject != projectRoot || gotGlobal != globalRoot || gotName != name {
			t.Fatalf("candidate request = (%q, %q, %q), want (%q, %q, %q)", gotProject, gotGlobal, gotName, projectRoot, globalRoot, name)
		}
		return interfaces.NamedFactoryCandidatePaths{Project: projectPath, Global: globalPath}, nil
	})
	ctx := startupcli.WithWorkingDirectory(context.Background(), workingDir)
	cfg := &runcli.RunConfig{NamedFactoryName: name}

	err := resolveRunNamedFactorySelection(
		ctx,
		cfg,
		homeDir,
		catalog,
		func(gotHome, gotWorking string) (interfaces.NamedFactoryRoots, error) {
			if gotHome != homeDir || gotWorking != workingDir {
				t.Fatalf("root request = (%q, %q), want (%q, %q)", gotHome, gotWorking, homeDir, workingDir)
			}
			return interfaces.NamedFactoryRoots{Project: projectRoot, Global: globalRoot}, nil
		},
		resolveCandidates,
	)
	operatorErr, ok := factoryload.AsOperatorError(err)
	if !ok {
		t.Fatalf("selection error = %T %v, want OperatorError", err, err)
	}
	if operatorErr.FactoryPath != projectPath {
		t.Fatalf("operator FactoryPath = %q, want detached project candidate %q", operatorErr.FactoryPath, projectPath)
	}
	if candidateCalls != 1 {
		t.Fatalf("candidate resolver calls = %d, want 1", candidateCalls)
	}
}

func TestRunScopedServerIntentIncludesInvocationAndSiteDashboard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cfg           runcli.RunConfig
		wantDashboard bool
	}{
		{
			name: "positional invocation with API",
			cfg: runcli.RunConfig{
				WithServer:               true,
				Port:                     7437,
				InvocationPositionalText: new(string),
			},
		},
		{
			name: "stdin invocation with site",
			cfg: runcli.RunConfig{
				WithServer:          true,
				WithSite:            true,
				Port:                7437,
				InvocationStdinText: new(string),
			},
			wantDashboard: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var got startupcli.RunIntent
			options := CommandFactory{
				initializer: startupcli.Functions{
					RunFunc: func(_ context.Context, intent startupcli.RunIntent, _ startupcli.RunSelection) error {
						got = intent
						return nil
					},
				},
				openRunSelection: func(runcli.RunConfig) startupcli.RunSelection {
					return testRunSelection{}
				},
			}
			if err := delegateRunInitialization(t.Context(), test.cfg, false, options); err != nil {
				t.Fatalf("delegateRunInitialization: %v", err)
			}
			if !got.APIEnabled || got.DashboardEnabled != test.wantDashboard {
				t.Fatalf("RunIntent = %#v, want API=true dashboard=%t", got, test.wantDashboard)
			}
		})
	}
}

func TestDelegateRunInitializationCarriesInvocationCancellation(t *testing.T) {
	t.Parallel()

	want := &rootInvocationCancellationStub{}
	var got startupcli.RunIntent
	options := CommandFactory{
		cancellation: want,
		initializer: startupcli.Functions{
			RunFunc: func(_ context.Context, intent startupcli.RunIntent, _ startupcli.RunSelection) error {
				got = intent
				return nil
			},
		},
		openRunSelection: func(runcli.RunConfig) startupcli.RunSelection {
			return testRunSelection{}
		},
	}
	if err := delegateRunInitialization(
		t.Context(),
		runcli.RunConfig{Continuously: true, WithServer: true, Port: 7437},
		false,
		options,
	); err != nil {
		t.Fatalf("delegateRunInitialization: %v", err)
	}
	if got.Cancellation != want {
		t.Fatalf("RunIntent cancellation = %p, want %p", got.Cancellation, want)
	}
}

type rootInvocationCancellationStub struct{}

func (*rootInvocationCancellationStub) Cancel() {}

func TestResolveRunNamedFactorySelectionHonorsCancellationWithoutSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cancelCatalog bool
	}{
		{name: "before lookup"},
		{name: "during catalog lookup", cancelCatalog: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(startupcli.WithWorkingDirectory(context.Background(), "customer-repo"))
			catalogCalls := 0
			catalog := rootNamedFactoryCatalogFake{resolve: func(string, string, string) (*interfaces.NamedFactoryResolution, error) {
				catalogCalls++
				if test.cancelCatalog {
					cancel()
				}
				return &interfaces.NamedFactoryResolution{FactoryDir: "selected-factory"}, nil
			}}
			if !test.cancelCatalog {
				cancel()
			}
			cfg := &runcli.RunConfig{NamedFactoryName: "alpha"}

			err := resolveRunNamedFactorySelection(
				ctx,
				cfg,
				"customer-home",
				catalog,
				func(string, string) (interfaces.NamedFactoryRoots, error) {
					return interfaces.NamedFactoryRoots{Project: "project", Global: "global"}, nil
				},
				func(string, string, string) (interfaces.NamedFactoryCandidatePaths, error) {
					t.Fatal("candidate lookup must not run for a canceled successful catalog lookup")
					return interfaces.NamedFactoryCandidatePaths{}, nil
				},
			)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
			if cfg.Dir != "" || cfg.NamedFactoryResolution != nil {
				t.Fatalf("canceled lookup returned partial selection: %#v", cfg)
			}
			wantCalls := 0
			if test.cancelCatalog {
				wantCalls = 1
			}
			if catalogCalls != wantCalls {
				t.Fatalf("catalog calls = %d, want %d", catalogCalls, wantCalls)
			}
		})
	}
}

func TestRunCommand_SelectedFactoryHelpRejectsReservedInvocationFlagBeforeRendering(t *testing.T) {
	for _, selection := range []string{"named", "factory"} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			err, stdout, stderr := executeSelectedFactoryHelpFixture(t, selection, "model")
			if err == nil {
				t.Fatal("reserved Factory flag help error = nil, want non-zero failure")
			}
			diagnostic := err.Error() + "\n" + stdout + "\n" + stderr
			for _, want := range []string{
				"cli.composition.long-name-collision",
				"model",
				"you.run.flag.model",
				"child-model",
				"worker-provider",
				"research-model",
			} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("help diagnostic missing %q:\n%s", want, diagnostic)
				}
			}
			if strings.Contains(stdout, "Factory invocation help") || strings.Contains(stdout, "--model") {
				t.Fatalf("colliding Factory help rendered unusable signature:\n%s", stdout)
			}
		})
	}
}

func TestRunCommand_SelectedFactoryHelpAcceptsPrefixedInvocationFlag(t *testing.T) {
	for _, selection := range []string{"named", "factory"} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			err, stdout, stderr := executeSelectedFactoryHelpFixture(t, selection, "child-model")
			if err != nil {
				t.Fatalf("prefixed Factory help error = %v\nstderr:\n%s", err, stderr)
			}
			for _, want := range []string{"Factory invocation help", "--child-model"} {
				if !strings.Contains(stdout, want) {
					t.Fatalf("prefixed Factory help missing %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func executeSelectedFactoryHelpFixture(t *testing.T, selection, externalName string) (error, string, string) {
	t.Helper()
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Errorf("restore working directory: %v", chdirErr)
		}
	})
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	factoryDir, err := factorydefinitionfixtures.SeedNamedFactory(
		filepath.Join(workingDirectory, "factory", "collision"),
		reservedInvocationHelpPayload(externalName),
	)
	if err != nil {
		t.Fatalf("SeedNamedFactory(collision): %v", err)
	}
	root := newLegacyTestRootCommandWithCatalog(transportNamedFactoryCatalog{"collision": factoryDir})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	args := []string{"run"}
	if selection == "named" {
		args = append(args, "--named", "collision")
	} else {
		args = append(args, "--factory", filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	}
	root.SetArgs(append(args, "--help"))

	err = root.Execute()
	return err, stdout.String(), stderr.String()
}

func reservedInvocationHelpPayload(externalName string) []byte {
	return bytes.Replace(
		portableFactoryPayloadWithInvocationSignature(),
		[]byte(`"name": "mode",`),
		[]byte(`"name": "mode", "externalName": "`+externalName+`",`),
		1,
	)
}
