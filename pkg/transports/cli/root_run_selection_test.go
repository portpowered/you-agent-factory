package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

				sessionResolvedHandlers: testSessionHandlers(nil, nil),
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

		sessionResolvedHandlers: testSessionHandlers(nil, nil),
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

// TestRunExplicitDirectoryStartupLayoutFailureRendersActionableNotFoundDiagnostic
// covers the proven public failure: an explicit --dir run against an
// authored-only directory reports a wrapped layout sentinel that the Current
// Factory classifier intentionally skips.
func TestRunExplicitDirectoryStartupLayoutFailureRendersActionableNotFoundDiagnostic(t *testing.T) {
	t.Parallel()

	const privateCause = "PRIVATE_LAYOUT_PAYLOAD_DO_NOT_LEAK"
	authoredDir := seedAuthoredOnlyFactoryDirectory(t)
	workFile := writeTestRunWorkFile(t, "batch.json")
	startupFailure := fmt.Errorf(
		"open Factory from %s: %w", privateCause, interfaces.ErrFactoryLayoutNotFound,
	)

	diagnostic, err := executeRunStartupFailure(t, startupFailure, []string{
		"--json", "run", "--dir", authoredDir, "--work", workFile,
	})

	if diagnostic.Code != runcli.CurrentFactoryNotFoundCode {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code, runcli.CurrentFactoryNotFoundCode)
	}
	if diagnostic.Family != string(factoryapi.ErrorFamilyNotFound) {
		t.Fatalf("diagnostic family = %q, want %q", diagnostic.Family, factoryapi.ErrorFamilyNotFound)
	}
	if diagnostic.Message != explicitFactoryDirectoryLayoutMessage {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message, explicitFactoryDirectoryLayoutMessage)
	}
	if !errors.Is(err, interfaces.ErrFactoryLayoutNotFound) {
		t.Fatalf("ExecuteCommand() error = %v, want preserved layout sentinel", err)
	}
	for _, forbidden := range []string{privateCause, authoredDir, "CLI_COMMAND_FAILED", "command failed"} {
		if strings.Contains(diagnostic.Output, forbidden) {
			t.Fatalf("run diagnostic disclosed %q:\n%s", forbidden, diagnostic.Output)
		}
	}
}

// TestRunExplicitDirectoryStartupLayoutFailureBoundaries keeps the new
// classification narrow: joined sentinels classify, unrelated startup failures
// stay on the generic fallback, and an authored --factory selection is not
// mapped by error text.
func TestRunExplicitDirectoryStartupLayoutFailureBoundaries(t *testing.T) {
	t.Parallel()

	const (
		privateCause = "PRIVATE_JOINED_PAYLOAD_DO_NOT_LEAK"
		factoryPath  = "authored-factory.json"
	)
	tests := []struct {
		name           string
		startupFailure error
		wantCode       string
	}{
		{
			name: "joined layout sentinel",
			startupFailure: errors.Join(
				errors.New(privateCause),
				interfaces.ErrFactoryLayoutNotFound,
			),
			wantCode: runcli.CurrentFactoryNotFoundCode,
		},
		{
			name:           "unrelated startup failure",
			startupFailure: fmt.Errorf("initialize %s: %w", privateCause, errors.New("host unavailable")),
			wantCode:       clidiag.DefaultFailureCode,
		},
		{
			name:           "layout sentinel text without identity",
			startupFailure: errors.New("factory layout not found"),
			wantCode:       clidiag.DefaultFailureCode,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			authoredDir := seedAuthoredOnlyFactoryDirectory(t)
			diagnostic, _ := executeRunStartupFailure(t, test.startupFailure, []string{
				"--json", "run", "--dir", authoredDir, "--work", writeTestRunWorkFile(t, "batch.json"),
			})
			if diagnostic.Code != test.wantCode {
				t.Fatalf("diagnostic code = %q, want %q (output=%s)", diagnostic.Code, test.wantCode, diagnostic.Output)
			}
			if strings.Contains(diagnostic.Output, privateCause) {
				t.Fatalf("diagnostic disclosed a private cause:\n%s", diagnostic.Output)
			}
		})
	}

	t.Run("authored --factory selection stays unmapped", func(t *testing.T) {
		t.Parallel()

		authoredFile := filepath.Join(t.TempDir(), factoryPath)
		if err := os.WriteFile(authoredFile, portableFactoryPayloadWithDefaultHandling(), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", authoredFile, err)
		}
		diagnostic, _ := executeRunStartupFailure(t, interfaces.ErrFactoryLayoutNotFound, []string{
			"--json", "run", "--factory", authoredFile, "--work", writeTestRunWorkFile(t, "batch.json"),
		})
		if diagnostic.Code != clidiag.DefaultFailureCode {
			t.Fatalf("--factory diagnostic code = %q, want %q (output=%s)", diagnostic.Code, clidiag.DefaultFailureCode, diagnostic.Output)
		}
		if strings.Contains(diagnostic.Output, explicitFactoryDirectoryLayoutMessage) {
			t.Fatalf("--factory selection used the --dir diagnostic:\n%s", diagnostic.Output)
		}
	})
}

type runStartupDiagnostic struct {
	Code    string `json:"code"`
	Family  string `json:"family"`
	Message string `json:"message"`
	Output  string
}

// executeRunStartupFailure runs one real composed `you run` invocation whose
// injected initializer reports startupFailure, then decodes the single
// diagnostic envelope owned by the central renderer.
func executeRunStartupFailure(t *testing.T, startupFailure error, args []string) (runStartupDiagnostic, error) {
	t.Helper()

	selectionOpened := false
	factory := withTestInjectedPlatformRoles(CommandFactory{
		sessionResolvedHandlers: testSessionHandlers(nil, nil),
	})
	workDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	executionErr := factory.ExecuteCommand(startupcli.CommandInvocation{
		Arguments: args,
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
		Context:   startupcli.WithWorkingDirectory(context.Background(), workDir),
		HomeDir:   func() (string, error) { return workDir, nil },
		LookupEnv: func(string) (string, bool) { return "", false },
		Initializer: startupcli.Functions{
			RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				selectionOpened = true
				return startupFailure
			},
		},
	})
	if !selectionOpened {
		t.Fatalf("run command never reached the injected initializer for %v", args)
	}
	return decodeRunStartupDiagnostic(t, stdout.String()+stderr.String()), executionErr
}

func decodeRunStartupDiagnostic(t *testing.T, output string) runStartupDiagnostic {
	t.Helper()

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatal("run failure rendered no diagnostic output")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 1 {
		t.Fatalf("run diagnostic output = %q, want exactly one envelope", output)
	}
	var diagnostic runStartupDiagnostic
	if err := json.Unmarshal([]byte(lines[0]), &diagnostic); err != nil {
		t.Fatalf("decode run diagnostic: %v; output=%q", err, output)
	}
	diagnostic.Output = output
	return diagnostic
}

func seedAuthoredOnlyFactoryDirectory(t *testing.T) string {
	t.Helper()

	authoredDir := t.TempDir()
	authored := []byte("name: authored-only\nworkTypes: []\n")
	if err := os.WriteFile(filepath.Join(authoredDir, "factory.yaml"), authored, 0o600); err != nil {
		t.Fatalf("WriteFile(authored Factory source): %v", err)
	}
	return authoredDir
}

func writeTestRunWorkFile(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(`{"requests":[]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}
