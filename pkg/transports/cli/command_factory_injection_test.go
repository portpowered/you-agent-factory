package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"go.uber.org/zap"
)

func TestPackagedInitCommandCompositionUsesDefinitionsOwnedAdapter(t *testing.T) {
	t.Parallel()

	called := false
	install := func(
		context.Context,
		interfaces.InstallPackagedFactoryRequest,
	) (interfaces.InstallPackagedFactoryResult, error) {
		called = true
		return interfaces.InstallPackagedFactoryResult{
			Definition: interfaces.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
			},
			Outcome: interfaces.PackagedFactoryInstallCreated,
			Format:  interfaces.PackagedFactoryFormatJSON,
		}, nil
	}
	factory := NewCommandFactory(CommandOperations{
		InstallPackagedFactory: factorydefinitionscli.BindInstallPackagedFactory(install),
	})
	if factory.InstallPackagedFactory == nil {
		t.Fatal("InstallPackagedFactory operation is missing from composed factory")
	}

	root := factory.NewCommand(
		func() (string, error) { return "/home/operator", nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/workspace/fleet"))
	root.SetArgs([]string{
		"init", "--package", "@you/goal", "--dir", "alternate-factories",
		"--format", "yaml", "--replace=true",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute packaged init: %v", err)
	}
	if !called {
		t.Fatal("Definitions-owned packaged init adapter was not invoked through production composition")
	}
}

func TestPackagedInitPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json",
		"init", "--package", "@you/goal", "--dir", "alternate-factories",
		"--format", "yaml", "--replace=true",
	}
	runPackagedInitCompositionCases(t, args, errors.New("packaged factory install failed"), func(result error) CommandOperations {
		install := func(
			_ context.Context,
			request interfaces.InstallPackagedFactoryRequest,
		) (interfaces.InstallPackagedFactoryResult, error) {
			if request.Name != "@you/goal" ||
				request.Format != interfaces.PackagedFactoryFormatYAML ||
				!request.Replace {
				return interfaces.InstallPackagedFactoryResult{}, fmt.Errorf(
					"unexpected packaged install request: %#v",
					request,
				)
			}
			if result != nil {
				return interfaces.InstallPackagedFactoryResult{}, result
			}
			return interfaces.InstallPackagedFactoryResult{
				Definition: interfaces.DistributedFactoryDefinitionFacts{
					Name:       "@you/goal",
					FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
				},
				Outcome: interfaces.PackagedFactoryInstallCreated,
				Format:  interfaces.PackagedFactoryFormatYAML,
			}, nil
		}
		return CommandOperations{
			InstallPackagedFactory: factorydefinitionscli.BindInstallPackagedFactory(install),
		}
	})
}

func runPackagedInitCompositionCases(
	t *testing.T,
	args []string,
	wantError error,
	operations func(error) CommandOperations,
) {
	t.Helper()
	t.Run("success", func(t *testing.T) {
		stdout, stderr, err := executePackagedInitComposition(t, operations(nil), args)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.Contains(stdout, `"name":"@you/goal"`) ||
			!strings.Contains(stdout, `"outcome":"created"`) {
			t.Fatalf("stdout = %q, want packaged init JSON success", stdout)
		}
		if !strings.Contains(stderr, "init packaged factory request name=@you/goal") {
			t.Fatalf("stderr = %q, want verbose packaged init diagnostics", stderr)
		}
	})
	t.Run("failure", func(t *testing.T) {
		stdout, stderr, err := executePackagedInitComposition(t, operations(wantError), args)
		if err == nil || !strings.Contains(err.Error(), wantError.Error()) {
			t.Fatalf("Execute() error = %v, want error containing %v", err, wantError)
		}
		if stdout != "" || !strings.Contains(stderr, wantError.Error()) {
			t.Fatalf("failure stdout = %q, stderr = %q", stdout, stderr)
		}
	})
}

func executePackagedInitComposition(
	t *testing.T,
	operations CommandOperations,
	args []string,
) (string, string, error) {
	t.Helper()
	factory := NewCommandFactory(operations)
	if factory.InstallPackagedFactory == nil {
		t.Fatal("InstallPackagedFactory operation is missing from production composition")
	}
	root := factory.NewCommand(
		func() (string, error) { return "/home/operator", nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/workspace/fleet"))
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestSessionCommandCompositionUsesTypedSessionsCLIAdapter(t *testing.T) {
	t.Parallel()

	called := false
	factory := NewCommandFactory(CommandOperations{
		SessionsCLI: session.Bind(session.Operations{
			Show: func(cfg session.ShowConfig) error {
				called = true
				if cfg.SessionID != "session-beta" {
					t.Fatalf("SessionID = %q, want session-beta", cfg.SessionID)
				}
				return nil
			},
		}),
	})
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from composed factory")
	}

	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-beta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute session show: %v", err)
	}
	if !called {
		t.Fatal("typed Sessions adapter was not invoked through production composition")
	}
}

func TestSessionCreatePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "create", "--dir", "/workspace/fleet",
		"--validate-only", "--target-kind", "named", "--target-name", "alpha",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Create: func(cfg session.CreateConfig) error {
				if cfg.Server != "https://factory.example" || cfg.Dir != "/workspace/fleet" ||
					!cfg.ValidateOnly || cfg.TargetKind != "named" || cfg.TargetName != "alpha" ||
					!cfg.JSON || !cfg.Verbose {
					t.Fatalf("create config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionDeletePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{"--verbose", "--json", "session", "delete", "session-beta", "--port", "9444"}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Delete: func(cfg session.DeleteConfig) error {
				if cfg.SessionID != "session-beta" || cfg.Port != 9444 || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("delete config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionListPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "list", "--scope", "live",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			List: func(cfg session.ListConfig) error {
				if cfg.Context == nil || cfg.Server != "https://factory.example" ||
					cfg.Scope != "live" || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("list config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionShowPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "show", "session-beta",
	}
	runSessionCompositionCases(t, args, context.Canceled, func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Show: func(cfg session.ShowConfig) error {
				if cfg.Context == nil || cfg.Server != "https://factory.example" ||
					cfg.SessionID != "session-beta" || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("show config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionDispatchesPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "dispatches", "dur-sess-review-001",
		"--phase", "review", "--status", "COMPLETED",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			ListDispatches: func(cfg session.DispatchesConfig) error {
				if cfg.Context == nil || cfg.Server != "https://factory.example" ||
					cfg.SessionID != "dur-sess-review-001" || cfg.Phase != "review" ||
					cfg.Status != "COMPLETED" || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("dispatches config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionPausePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "pause",
	}
	runSessionCompositionCases(t, args, errors.New("session lifecycle operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Pause: func(cfg session.LifecycleControlConfig) error {
				if cfg.Context == nil || cfg.Server != "https://factory.example" ||
					cfg.SessionID != "" || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("pause config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func TestSessionResumePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "resume", "session-beta",
	}
	runSessionCompositionCases(t, args, context.Canceled, func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Resume: func(cfg session.LifecycleControlConfig) error {
				if cfg.Context == nil || cfg.Server != "https://factory.example" ||
					cfg.SessionID != "session-beta" || !cfg.JSON || !cfg.Verbose {
					t.Fatalf("resume config = %#v", cfg)
				}
				return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
			},
		})}
	})
}

func runSessionCompositionCases(
	t *testing.T,
	args []string,
	wantError error,
	operations func(error) CommandOperations,
) {
	t.Helper()
	t.Run("success", func(t *testing.T) {
		stdout, stderr, err := executeSessionComposition(t, operations(nil), args)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if stdout != "session-ok\n" || stderr != "session-diagnostic\n" {
			t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
		}
	})
	t.Run("failure", func(t *testing.T) {
		stdout, stderr, err := executeSessionComposition(t, operations(wantError), args)
		if !errors.Is(err, wantError) {
			t.Fatalf("Execute() error = %v, want %v", err, wantError)
		}
		if stdout != "" || stderr != fmt.Sprintf("Error: %v\n", wantError) {
			t.Fatalf("failure stdout = %q, stderr = %q", stdout, stderr)
		}
	})
}

func executeSessionComposition(
	t *testing.T,
	operations CommandOperations,
	args []string,
) (string, string, error) {
	t.Helper()
	factory := NewCommandFactory(operations)
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from production composition")
	}
	root := factory.NewCommand(nil, nil, nil)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func writeSessionCompositionOutput(output, diagnostics io.Writer, result error) error {
	if result != nil {
		return result
	}
	if _, err := fmt.Fprintln(output, "session-ok"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(diagnostics, "session-diagnostic")
	return err
}

type injectedModelsCLIService struct {
	list func(modelscli.ListConfig) error
}

func (service injectedModelsCLIService) List(cfg modelscli.ListConfig) error {
	if service.list == nil {
		return fmt.Errorf("models list service is required")
	}
	return service.list(cfg)
}
func (injectedModelsCLIService) Inspect(modelscli.InspectConfig) error { return nil }
func (injectedModelsCLIService) Invoke(modelscli.InvokeConfig) error   { return nil }
func (injectedModelsCLIService) Pull(modelscli.PullConfig) error       { return nil }

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestNewCommandFactoryDoesNotInstallTransportDefaults(t *testing.T) {
	t.Parallel()

	factory := NewCommandFactory(CommandOperations{})
	if factory.SubmitWork != nil ||
		factory.SessionsCLI != nil ||
		factory.ModelsCLI != nil ||
		factory.FlattenFactoryConfig != nil ||
		factory.InitFactory != nil ||
		factory.QueryFactory != nil ||
		factory.ListWork != nil ||
		factory.resolveNamedFactoryRoots != nil ||
		factory.resolveNamedFactoryCandidatePaths != nil ||
		factory.resolveCurrentFactoryDir != nil ||
		factory.resolveFactoryConfigRoot != nil ||
		factory.loadFactoryConfigFile != nil ||
		factory.workRequestFileLoader != nil ||
		factory.prepareInvocationInput != nil ||
		factory.openRunSelection != nil ||
		factory.buildTerminalLogger != nil ||
		factory.batchInputFileSystem != nil ||
		factory.runDirectoryCreator != nil ||
		factory.browserOpener != nil ||
		!reflect.DeepEqual(factory.runDefaults, runcli.RunConfig{}) {
		t.Fatalf("factory = %#v, want missing operations to remain missing", factory)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestNewCommandFactoryPreservesInjectedRuntimeCLIAdapter(t *testing.T) {
	t.Parallel()

	adapter := factoryruntimecli.BindService(factoryruntimecli.Config{})
	factory := NewCommandFactory(CommandOperations{
		RunDefaults: runcli.RunConfig{RuntimeCLI: adapter},
	})
	if factory.runDefaults.RuntimeCLI == nil {
		t.Fatal("injected Runtime CLI adapter is missing from composed run defaults")
	}
}

func TestNewCommandFactoryPreservesInjectedOperations(t *testing.T) {
	t.Parallel()

	modelCalls := 0
	sessionCalls := 0
	resolver := interfaces.CurrentFactoryDirectoryResolver(func(rootDir string) (string, error) {
		return rootDir + "/current", nil
	})
	namedRoots := NamedFactoryRootsResolver(func(homeDir, workingDir string) (interfaces.NamedFactoryRoots, error) {
		return interfaces.NamedFactoryRoots{Project: workingDir + "/factory", Global: homeDir + "/factories"}, nil
	})
	namedCandidates := interfaces.NamedFactoryCandidatePathsResolver(func(projectRoot, globalRoot, name string) (interfaces.NamedFactoryCandidatePaths, error) {
		return interfaces.NamedFactoryCandidatePaths{Project: projectRoot + "/candidate", Global: globalRoot + "/candidate"}, nil
	})
	configRootResolver := interfaces.FactoryConfigRootResolver(func(path string) (string, error) {
		return path + "/root", nil
	})
	configLoader := interfaces.FactoryConfigFileLoader(func(string) (*interfaces.FactoryConfig, error) {
		return &interfaces.FactoryConfig{Name: "injected"}, nil
	})
	workLoader := work.RequestFileLoader(func(string) (work.WorkRequest, error) {
		return work.WorkRequest{Type: work.WorkRequestTypeFactoryRequestBatch}, nil
	})
	invocationPreparation := rootInvocationInputScript{}
	openRunSelection := runcli.SelectionFactory(func(runcli.RunConfig) startupcli.RunSelection { return nil })
	loggerBuilder := terminalpolicy.LoggerBuilder(func(terminalpolicy.Mode, bool) (*zap.Logger, error) {
		return zap.NewNop(), nil
	})
	batchFiles := batchInputFileSystemFakeForFactoryTest{}
	directories := runDirectoryCreatorFakeForFactoryTest{}
	browser := func(context.Context, string) error { return nil }
	factory := NewCommandFactory(CommandOperations{
		ResolveNamedFactoryRoots:          namedRoots,
		ResolveNamedFactoryCandidatePaths: namedCandidates,
		ResolveCurrentFactoryDir:          resolver,
		ResolveFactoryConfigRoot:          configRootResolver,
		LoadFactoryConfigFile:             configLoader,
		WorkRequestFileLoader:             workLoader,
		PrepareInvocationInput:            invocationPreparation,
		OpenRunSelection:                  openRunSelection,
		BuildTerminalLogger:               loggerBuilder,
		RunDefaults:                       runcli.RunConfig{Port: 9123},
		BatchInputFileSystem:              batchFiles,
		RunDirectoryCreator:               directories,
		BrowserOpener:                     browser,
		ModelsCLI: injectedModelsCLIService{list: func(modelscli.ListConfig) error {
			modelCalls++
			return nil
		}},
		SessionsCLI: session.Bind(session.Operations{
			Show: func(cfg session.ShowConfig) error {
				sessionCalls++
				if cfg.SessionID != "session-alpha" {
					t.Fatalf("SessionID = %q, want session-alpha", cfg.SessionID)
				}
				return nil
			},
		}),
	})
	if roots, err := factory.resolveNamedFactoryRoots("home", "repo"); err != nil || roots.Project != "repo/factory" || roots.Global != "home/factories" {
		t.Fatalf("named Factory roots = %#v, %v", roots, err)
	}
	if candidates, err := factory.resolveNamedFactoryCandidatePaths("project", "global", "alpha"); err != nil || candidates.Project != "project/candidate" || candidates.Global != "global/candidate" {
		t.Fatalf("named Factory candidates = %#v, %v", candidates, err)
	}
	if factory.ModelsCLI == nil {
		t.Fatal("injected Models CLI service is missing")
	}
	if factory.SessionsCLI == nil {
		t.Fatal("injected Sessions CLI service is missing")
	}
	if err := factory.ModelsCLI.List(modelscli.ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if err := factory.SessionsCLI.Show(session.ShowConfig{SessionID: "session-alpha"}); err != nil {
		t.Fatalf("ShowSession: %v", err)
	}
	if modelCalls != 1 {
		t.Fatalf("model calls = %d, want 1", modelCalls)
	}
	if sessionCalls != 1 {
		t.Fatalf("session calls = %d, want 1", sessionCalls)
	}
	resolved, err := factory.resolveCurrentFactoryDir("factory")
	if err != nil || resolved != "factory/current" {
		t.Fatalf("current Factory resolution = %q, %v", resolved, err)
	}
	if logger, err := factory.buildTerminalLogger(terminalpolicy.ModeQuiet, false); err != nil || logger == nil {
		t.Fatalf("terminal logger = %#v, %v", logger, err)
	}
	if resolved, err := factory.resolveFactoryConfigRoot("factory.json"); err != nil || resolved != "factory.json/root" {
		t.Fatalf("config root = %q, %v", resolved, err)
	}
	if loaded, err := factory.loadFactoryConfigFile("factory.json"); err != nil || loaded.Name != "injected" {
		t.Fatalf("config = %#v, %v", loaded, err)
	}
	if loaded, err := factory.workRequestFileLoader("work.json"); err != nil || loaded.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request = %#v, %v", loaded, err)
	}
	if factory.prepareInvocationInput == nil {
		t.Fatal("injected Work invocation-input preparation is missing")
	}
	if factory.openRunSelection == nil {
		t.Fatal("injected run selection operation is missing")
	}
	if factory.runDefaults.Port != 9123 {
		t.Fatalf("run defaults port = %d, want 9123", factory.runDefaults.Port)
	}
	if factory.batchInputFileSystem == nil {
		t.Fatal("injected batch input file system is missing")
	}
	if factory.runDirectoryCreator == nil {
		t.Fatal("injected run directory creator is missing")
	}
	if factory.browserOpener == nil {
		t.Fatal("injected browser opener is missing")
	}
}

type batchInputFileSystemFakeForFactoryTest struct{}

type runDirectoryCreatorFakeForFactoryTest struct{}

func (runDirectoryCreatorFakeForFactoryTest) MkdirAll(string, fs.FileMode) error { return nil }

func (batchInputFileSystemFakeForFactoryTest) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (batchInputFileSystemFakeForFactoryTest) ReadFile(string) ([]byte, error) {
	return nil, fs.ErrNotExist
}

func TestMissingCommandOperationFailsExecutionWithRequiredEdgeError(t *testing.T) {
	t.Parallel()

	root := NewCommandFactory(CommandOperations{ModelsCLI: injectedModelsCLIService{}}).NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "models list service is required") {
		t.Fatalf("error = %v, want explicit required-edge failure", err)
	}
}
