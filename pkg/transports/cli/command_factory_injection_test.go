package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workcmd "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
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

func TestWorkCommandCompositionUsesResolvedOwnerAdapter(t *testing.T) {
	t.Parallel()

	var got workcmd.ListConfig
	factory := NewCommandFactory(CommandOperations{
		ListWork: func(cfg workcmd.ListConfig) error {
			got = cfg
			_, err := fmt.Fprintln(cfg.Output, "owner-list")
			return err
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"work", "list", "--name", "review"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list: %v", err)
	}
	if got.Name != "review" {
		t.Fatalf("work list name = %q, want review", got.Name)
	}
	if got.Context == nil || got.Output == nil {
		t.Fatalf("work list owner config = %#v, want CLI context and output boundaries", got)
	}
	if stdout.String() != "owner-list\n" {
		t.Fatalf("stdout = %q, want owner adapter output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
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

	args := []string{"--verbose", "--json", "--server", "https://factory.example:9444", "session", "delete", "session-beta"}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{SessionsCLI: session.Bind(session.Operations{
			Delete: func(cfg session.DeleteConfig) error {
				if cfg.SessionID != "session-beta" || cfg.Server != "https://factory.example:9444" || !cfg.JSON || !cfg.Verbose {
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
	// These unit fixtures provide one adapter for the command family. Production
	// composition supplies distinct local and remote services explicitly.
	if operations.LocalSessionsCLI == nil {
		operations.LocalSessionsCLI = operations.SessionsCLI
	}
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

func TestProductionMetricsCommandUsesInjectedRuntimeMetricsQuery(t *testing.T) {
	called := false
	factory := withTestInjectedPlatformRoles(CommandFactory{
		runtimeMetricsQuery: func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			called = true
			if !strings.HasSuffix(request.MetricsRoot, ".you-agent-factory\\metrics") && !strings.HasSuffix(request.MetricsRoot, ".you-agent-factory/metrics") {
				t.Fatalf("metrics root = %q, want the default metrics directory", request.MetricsRoot)
			}
			return factoryvisualization.RuntimeMetricsQueryResult{
				Providers: []factoryvisualization.RuntimeMetricsBreakdown{{Key: "provider-a"}},
			}, nil
		},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"metrics", "--group-by", "provider"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	if !called {
		t.Fatal("production metrics command did not use the injected query")
	}
	if !strings.Contains(output.String(), "Breakdown by provider: 1 rows") || !strings.Contains(output.String(), "provider-a:") {
		t.Fatalf("output = %q, want provider breakdown", output.String())
	}
}

func TestProductionMetricsCommandResolvesGlobalJSONAndSessionScope(t *testing.T) {
	var gotRequest factoryvisualization.RuntimeMetricsQueryRequest
	factory := withTestInjectedPlatformRoles(CommandFactory{
		runtimeMetricsQuery: func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			gotRequest = request
			return factoryvisualization.RuntimeMetricsQueryResult{
				Providers: []factoryvisualization.RuntimeMetricsBreakdown{{Key: "provider-a"}},
			}, nil
		},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"metrics", "--group-by", "provider", "--session", "session-a", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	if gotRequest.SessionID != "session-a" {
		t.Fatalf("query session ID = %q, want session-a", gotRequest.SessionID)
	}
	var document struct {
		Scope struct {
			Kind             string  `json:"kind"`
			FactorySessionID *string `json:"factory_session_id"`
		} `json:"scope"`
		GroupBy string `json:"group_by"`
		Groups  []struct {
			Key string `json:"key"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode metrics JSON: %v\n%s", err, output.String())
	}
	if document.Scope.Kind != "factory_session" || document.Scope.FactorySessionID == nil || *document.Scope.FactorySessionID != "session-a" {
		t.Fatalf("JSON scope = %#v, want session-a", document.Scope)
	}
	if document.GroupBy != "provider" || len(document.Groups) != 1 || document.Groups[0].Key != "provider-a" {
		t.Fatalf("JSON grouping = %q with groups %#v, want provider/provider-a", document.GroupBy, document.Groups)
	}
}

type productionMetricsFailureCase struct {
	name        string
	args        []string
	home        func() (string, error)
	queryError  error
	wantCode    string
	wantFamily  string
	wantMessage string
	wantCalls   int
	wantCause   error
}

func TestProductionMetricsCommandExecuteCommandPreservesCodedFailures(t *testing.T) {
	t.Parallel()
	queryCause := errors.New("metrics path=/private/credential=do-not-leak")
	queryError := &factoryvisualization.RuntimeMetricsQueryError{
		Kind:    factoryvisualization.RuntimeMetricsQueryReadFailed,
		Message: "query Factory Runtime metrics: read artifacts",
		Cause:   queryCause,
	}
	resolverCause := errors.New("home resolver credential=do-not-leak")
	tests := []productionMetricsFailureCase{
		{
			name:        "invalid group",
			args:        []string{"metrics", "--group-by", "region"},
			home:        func() (string, error) { return "operator-home", nil },
			wantCode:    "METRICS_INVALID_GROUP_BY",
			wantFamily:  "BAD_REQUEST",
			wantMessage: `invalid --group-by "region": choose workstation, worker, or provider`,
		},
		{
			name:        "invalid group in JSON mode",
			args:        []string{"--json", "metrics", "--group-by", "region"},
			home:        func() (string, error) { return "operator-home", nil },
			wantCode:    "METRICS_INVALID_GROUP_BY",
			wantFamily:  "BAD_REQUEST",
			wantMessage: `invalid --group-by "region": choose workstation, worker, or provider`,
		},
		{
			name:        "home resolver failure",
			args:        []string{"metrics"},
			home:        func() (string, error) { return "", resolverCause },
			wantCode:    "METRICS_HOME_DIRECTORY_FAILED",
			wantFamily:  "INTERNAL_SERVER_ERROR",
			wantMessage: "resolve metrics home directory: home directory could not be resolved; set HOME or USERPROFILE",
			wantCause:   resolverCause,
		},
		{
			name:        "empty home path",
			args:        []string{"metrics"},
			home:        func() (string, error) { return "  ", nil },
			wantCode:    "METRICS_HOME_DIRECTORY_FAILED",
			wantFamily:  "INTERNAL_SERVER_ERROR",
			wantMessage: "resolve metrics home directory: resolver returned an empty path; set HOME or USERPROFILE",
		},
		{
			name:        "query read failure",
			args:        []string{"--json", "metrics"},
			home:        func() (string, error) { return "operator-home", nil },
			queryError:  queryError,
			wantCode:    "METRICS_QUERY_FAILED",
			wantFamily:  "INTERNAL_SERVER_ERROR",
			wantMessage: "query Factory Runtime metrics: read artifacts",
			wantCalls:   1,
			wantCause:   queryCause,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runProductionMetricsFailureCase(t, test)
		})
	}
}

func TestExecuteCommandUsageFailuresUseCentralCobraRenderer(t *testing.T) {
	t.Parallel()

	factory := NewCommandFactory(CommandOperations{})
	for _, test := range []struct {
		name         string
		args         []string
		wantError    string
		wantHelpPath string
	}{
		{
			name:         "unknown top-level flag",
			args:         []string{"--definitely-unknown"},
			wantError:    "unknown flag: --definitely-unknown",
			wantHelpPath: "you --help",
		},
		{
			name:         "missing required argument",
			args:         []string{"work", "show"},
			wantError:    "requires at least 1 arg(s), only received 0",
			wantHelpPath: "you work show --help",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := factory.ExecuteCommand(startupcli.CommandInvocation{
				Arguments: test.args,
				Stdin:     strings.NewReader(""),
				Stdout:    &stdout,
				Stderr:    &stderr,
				Context:   context.Background(),
			})
			if err == nil {
				t.Fatal("ExecuteCommand() error = nil, want usage failure")
			}
			if !strings.Contains(stderr.String(), "Error: "+test.wantError) ||
				!strings.Contains(stderr.String(), "Run '"+test.wantHelpPath+"' for usage.") {
				t.Fatalf("stderr = %q, want Cobra error and help hint", stderr.String())
			}
			if strings.Contains(stderr.String(), "CLI_COMMAND_FAILED") ||
				strings.Contains(stderr.String(), "INTERNAL_SERVER_ERROR") {
				t.Fatalf("stderr mislabeled usage failure: %q", stderr.String())
			}
		})
	}
}
func runProductionMetricsFailureCase(t *testing.T, test productionMetricsFailureCase) {
	t.Helper()
	queryCalls := 0
	factory := withTestInjectedPlatformRoles(CommandFactory{
		runtimeMetricsQuery: func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			queryCalls++
			return factoryvisualization.RuntimeMetricsQueryResult{}, test.queryError
		},
	})
	var stdout, stderr bytes.Buffer
	err := factory.ExecuteCommand(startupcli.CommandInvocation{
		Arguments: test.args,
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
		Context:   context.Background(),
		HomeDir:   test.home,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err == nil {
		t.Fatal("ExecuteCommand() error = nil, want metrics failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty failure output", stdout.String())
	}
	assertSingleMetricsDiagnostic(t, stderr.String(), test.wantCode, test.wantFamily, test.wantMessage)
	if queryCalls != test.wantCalls {
		t.Fatalf("metrics query calls = %d, want %d", queryCalls, test.wantCalls)
	}
	if test.wantCause != nil && !errors.Is(err, test.wantCause) {
		t.Fatalf("ExecuteCommand() error = %v, want to preserve cause", err)
	}
	if test.queryError != nil {
		var gotQueryError *factoryvisualization.RuntimeMetricsQueryError
		if !errors.As(err, &gotQueryError) {
			t.Fatalf("ExecuteCommand() error = %v, want query error classification preserved", err)
		}
	}
	if strings.Contains(stderr.String(), "do-not-leak") {
		t.Fatalf("central diagnostic exposed an underlying payload: %q", stderr.String())
	}
}

func assertSingleMetricsDiagnostic(t *testing.T, output, wantCode, wantFamily, wantMessage string) {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	lines := strings.Split(trimmed, "\n")
	if trimmed == "" || len(lines) != 1 {
		t.Fatalf("diagnostic output = %q, want exactly one JSON line", output)
	}
	var diagnostic struct {
		Code    string `json:"code"`
		Family  string `json:"family"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(trimmed), &diagnostic); err != nil {
		t.Fatalf("decode central diagnostic: %v; output=%q", err, output)
	}
	if diagnostic.Code != wantCode || diagnostic.Family != wantFamily || diagnostic.Message != wantMessage {
		t.Fatalf("diagnostic = %#v, want code %q, family %q, and message %q", diagnostic, wantCode, wantFamily, wantMessage)
	}
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
func (injectedModelsCLIService) Remove(modelscli.RemoveConfig) error   { return nil }

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
func TestNewCommandFactoryPreservesInjectedModelsCLIAdapter(t *testing.T) {
	t.Parallel()

	adapter := modelscli.BindService(modelscli.Config{
		Models: compositionModelsRootForFactoryTest{},
	})
	factory := NewCommandFactory(CommandOperations{ModelsCLI: adapter})
	if factory.ModelsCLI == nil {
		t.Fatal("injected Models CLI adapter is missing from composed factory")
	}
}

type compositionModelsRootForFactoryTest struct{}

func (compositionModelsRootForFactoryTest) ListModels(context.Context) (modelinference.List, error) {
	return modelinference.List{}, nil
}

func (compositionModelsRootForFactoryTest) GetModel(context.Context, string) (modelinference.Detail, error) {
	return modelinference.Detail{}, nil
}

func (compositionModelsRootForFactoryTest) PullModel(context.Context, string) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, nil
}

func (compositionModelsRootForFactoryTest) OpenRuntimeScope(context.Context, modelinference.OpenRuntimeScopeRequest) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) CloseRuntimeScope(context.Context, modelinference.CloseRuntimeScopeRequest) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) ListCatalog(context.Context, modelinference.ListModelsRequest) (modelinference.ListModelsResult, error) {
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) GetCatalogModel(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) ResolveModelReference(context.Context, modelinference.ResolveModelReferenceRequest) (modelinference.ResolveModelReferenceResult, error) {
	return modelinference.ResolveModelReferenceResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) PreflightModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PreflightModelAssetsResult, error) {
	return modelinference.PreflightModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) PullModelForScope(context.Context, modelinference.PullModelRequest) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) PrepareModelAssets(context.Context, modelinference.PrepareModelAssetsRequest) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InspectModelAssets(context.Context, modelinference.InspectModelAssetsRequest) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) RemoveModelAssets(context.Context, modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) EnsureModelHost(context.Context, modelinference.EnsureModelHostRequest) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InspectModelHost(context.Context, modelinference.InspectModelHostRequest) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) StopModelHost(context.Context, modelinference.StopModelHostRequest) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) AcquireModelLease(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) GetModelLease(context.Context, modelinference.GetModelLeaseRequest) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) ReleaseModelLease(context.Context, modelinference.ReleaseModelLeaseRequest) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InvokeModelWithLease(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InvokeModel(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) CancelInvocation(context.Context, modelinference.CancelInvocationRequest) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InspectRuntime(context.Context, string) (modelinference.Runtime, error) {
	return modelinference.Runtime{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return modelinference.ErrUnsupportedOperation
}

func (compositionModelsRootForFactoryTest) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, modelinference.ErrUnsupportedOperation
}

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
