package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli/worker_sessions"
	"github.com/spf13/cobra"
)

func TestProductionProvidersCommandWiresGeneratedHandlersAndHelp(t *testing.T) {
	providerService := providerscli.New(&providerServiceStub{})
	command, err := newProductionProvidersCommand(
		&cliDiagnosticsOptions{},
		CommandFactory{ProvidersCLI: providerService},
	)
	if err != nil {
		t.Fatalf("newProductionProvidersCommand() error = %v", err)
	}
	if command.RunE != nil {
		t.Fatal("you providers must remain non-runnable")
	}
	list, _, err := command.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find you providers list: %v", err)
	}
	if list.RunE == nil {
		t.Fatal("you providers list must attach its resolved manifest handler")
	}

	manifest, err := generated.ProvidersFamilyManifest()
	if err != nil {
		t.Fatalf("ProvidersFamilyManifest() error = %v", err)
	}
	if manifest.Commands["you.providers.list"].Documentation.Documentation.Title.CanonicalEnglish != "List provider capabilities" {
		t.Fatalf("providers list title = %q", manifest.Commands["you.providers.list"].Documentation.Documentation.Title.CanonicalEnglish)
	}

	var help bytes.Buffer
	command.SetOut(&help)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute providers help: %v", err)
	}
	for _, want := range []string{"Discover provider capabilities", "you providers list", "Available Commands"} {
		if !bytes.Contains(help.Bytes(), []byte(want)) {
			t.Fatalf("providers help missing %q:\n%s", want, help.String())
		}
	}
}

// providerServiceStub uses the published root interface without giving the
// CLI package a peer implementation or a second service graph. This test only
// exercises command projection and never invokes a Providers operation.
type providerServiceStub struct {
	providers.Service
}

var _ providers.Service = (*providerServiceStub)(nil)

func (providerServiceStub) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func TestProductionWorkerSessionsUsesResolvedOwnerInputs(t *testing.T) {
	var got workersessionscli.ListConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ModelsCLI: rootModelsCLI,
		ListWorkerSessions: func(cfg workersessionscli.ListConfig) error {
			got = cfg
			return nil
		},
	})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{},
	)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--server", "https://factory.example",
		"--json",
		"worker-sessions", "list", "--work-id", "work-1", "--scope", "all",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute resolved worker-sessions list: %v", err)
	}
	if got.WorkID != "work-1" || got.Scope != "all" {
		t.Fatalf("worker-sessions list config = %+v, want resolved work and scope", got)
	}
	if got.Server != "https://factory.example" || !got.JSON {
		t.Fatalf("worker-sessions list globals = server %q json %t, want resolved values", got.Server, got.JSON)
	}
}

func TestWorkerSessionsGeneratedHandlersReportUnavailableCommandInputs(t *testing.T) {
	cmd := &cobra.Command{}
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	checks := []struct {
		name string
		call func() error
	}{
		{name: "invoke", call: func() error {
			return executeGeneratedWorkerSessionsInvokeWithValues(cmd, nil, globals, diagnostics, func(workersessionscli.InvokeConfig) error { return nil }, nil, nil)
		}},
		{name: "continue", call: func() error {
			return executeGeneratedWorkerSessionsContinueWithValues(cmd, nil, globals, diagnostics, func(workersessionscli.ContinueConfig) error { return nil }, nil, nil)
		}},
		{name: "interrupt", call: func() error {
			return executeGeneratedWorkerSessionsInterruptWithValues(cmd, nil, globals, diagnostics, func(workersessionscli.InterruptConfig) error { return nil }, nil)
		}},
		{name: "control", call: func() error {
			return executeGeneratedWorkerSessionsControlWithValues(cmd, globals, diagnostics, func(workersessionscli.ControlConfig) error { return nil }, nil, workersessions.ControlActionCancel, nil)
		}},
		{name: "list", call: func() error {
			return executeGeneratedWorkerSessionsListWithValues(cmd, globals, diagnostics, func(workersessionscli.ListConfig) error { return nil }, nil)
		}},
		{name: "show", call: func() error {
			return executeGeneratedWorkerSessionsShowWithValues(cmd, globals, diagnostics, func(workersessionscli.ShowConfig) error { return nil }, nil)
		}},
		{name: "stream", call: func() error {
			return executeGeneratedWorkerSessionsStreamWithValues(cmd, globals, diagnostics, func(workersessionscli.StreamConfig) error { return nil }, nil)
		}},
		{name: "read", call: func() error {
			return executeGeneratedWorkerSessionsReadWithValues(cmd, globals, diagnostics, func(workersessionscli.ReadConfig) error { return nil }, nil)
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil || !strings.Contains(err.Error(), "resolved CLI input") {
				t.Fatalf("%s handler error = %v, want unavailable generated input", check.name, err)
			}
		})
	}
}

func TestWorkerSessionsGeneratedHandlersRequireConfiguredServices(t *testing.T) {
	cmd := &cobra.Command{}
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	checks := []struct {
		name string
		call func() error
	}{
		{name: "invoke", call: func() error {
			return executeGeneratedWorkerSessionsInvokeWithValues(cmd, nil, globals, diagnostics, nil, nil, nil)
		}},
		{name: "continue", call: func() error {
			return executeGeneratedWorkerSessionsContinueWithValues(cmd, nil, globals, diagnostics, nil, nil, nil)
		}},
		{name: "interrupt", call: func() error {
			return executeGeneratedWorkerSessionsInterruptWithValues(cmd, nil, globals, diagnostics, nil, nil)
		}},
		{name: "control", call: func() error {
			return executeGeneratedWorkerSessionsControlWithValues(cmd, globals, diagnostics, nil, nil, workersessions.ControlActionCancel, nil)
		}},
		{name: "list", call: func() error {
			return executeGeneratedWorkerSessionsListWithValues(cmd, globals, diagnostics, nil, nil)
		}},
		{name: "show", call: func() error {
			return executeGeneratedWorkerSessionsShowWithValues(cmd, globals, diagnostics, nil, nil)
		}},
		{name: "stream", call: func() error {
			return executeGeneratedWorkerSessionsStreamWithValues(cmd, globals, diagnostics, nil, nil)
		}},
		{name: "read", call: func() error {
			return executeGeneratedWorkerSessionsReadWithValues(cmd, globals, diagnostics, nil, nil)
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), "service is required") {
				t.Fatalf("%s handler error = %v, want missing service error", check.name, err)
			}
		})
	}
}

func TestWorkerSessionsGeneratedHandlersRejectNilCommands(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	checks := []struct {
		name string
		call func() error
	}{
		{name: "invoke", call: func() error {
			return executeGeneratedWorkerSessionsInvokeWithValues(nil, nil, globals, diagnostics, func(workersessionscli.InvokeConfig) error { return nil }, nil, nil)
		}},
		{name: "continue", call: func() error {
			return executeGeneratedWorkerSessionsContinueWithValues(nil, nil, globals, diagnostics, func(workersessionscli.ContinueConfig) error { return nil }, nil, nil)
		}},
		{name: "interrupt", call: func() error {
			return executeGeneratedWorkerSessionsInterruptWithValues(nil, nil, globals, diagnostics, func(workersessionscli.InterruptConfig) error { return nil }, nil)
		}},
		{name: "control", call: func() error {
			return executeGeneratedWorkerSessionsControlWithValues(nil, globals, diagnostics, func(workersessionscli.ControlConfig) error { return nil }, nil, workersessions.ControlActionCancel, nil)
		}},
		{name: "list", call: func() error {
			return executeGeneratedWorkerSessionsListWithValues(nil, globals, diagnostics, func(workersessionscli.ListConfig) error { return nil }, nil)
		}},
		{name: "show", call: func() error {
			return executeGeneratedWorkerSessionsShowWithValues(nil, globals, diagnostics, func(workersessionscli.ShowConfig) error { return nil }, nil)
		}},
		{name: "stream", call: func() error {
			return executeGeneratedWorkerSessionsStreamWithValues(nil, globals, diagnostics, func(workersessionscli.StreamConfig) error { return nil }, nil)
		}},
		{name: "read", call: func() error {
			return executeGeneratedWorkerSessionsReadWithValues(nil, globals, diagnostics, func(workersessionscli.ReadConfig) error { return nil }, nil)
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), "command is required") {
				t.Fatalf("%s handler error = %v, want nil-command error", check.name, err)
			}
		})
	}
}

func TestWorkerSessionsInvokeAndContinueReadersReportTypedInputErrors(t *testing.T) {
	invokeValues := map[string]any{
		"you.worker-sessions.invoke.flag.execution":          "",
		"you.worker-sessions.invoke.flag.request-id":         "",
		"you.worker-sessions.invoke.flag.worker-session-id":  "",
		"you.worker-sessions.invoke.flag.dispatch-id":        "",
		"you.worker-sessions.invoke.flag.workstation":        "",
		"you.worker-sessions.invoke.flag.worker-type":        "",
		"you.worker-sessions.invoke.flag.runner":             "",
		"you.worker-sessions.invoke.flag.provider":           "",
		"you.worker-sessions.invoke.flag.model":              "",
		"you.worker-sessions.invoke.flag.reasoning-effort":   "",
		"you.worker-sessions.invoke.flag.system-prompt":      "",
		"you.worker-sessions.invoke.flag.user-message":       "",
		"you.worker-sessions.invoke.flag.output":             "",
		"you.worker-sessions.invoke.flag.async":              false,
		"you.worker-sessions.invoke.flag.retry-max-attempts": 0,
	}
	for _, key := range []string{
		"you.worker-sessions.invoke.flag.execution",
		"you.worker-sessions.invoke.flag.async",
		"you.worker-sessions.invoke.flag.retry-max-attempts",
	} {
		candidate := cloneCLIInputValues(invokeValues)
		delete(candidate, key)
		if _, err := readGeneratedWorkerSessionsInvokeInputs(candidate); err == nil {
			t.Errorf("readGeneratedWorkerSessionsInvokeInputs(missing %s) = nil error, want typed input error", key)
		}
	}

	continueValues := map[string]any{
		"you.worker-sessions.continue.arg.0":                            "source-1",
		"you.worker-sessions.continue.flag.request-id":                  "request-1",
		"you.worker-sessions.continue.flag.successor-worker-session-id": "successor-1",
		"you.worker-sessions.continue.flag.user-message":                "message",
		"you.worker-sessions.continue.flag.output":                      "json",
		"you.worker-sessions.continue.arg.1":                            []string{"follow-up"},
		"you.worker-sessions.continue.flag.async":                       false,
	}
	for _, key := range []string{
		"you.worker-sessions.continue.arg.0",
		"you.worker-sessions.continue.flag.async",
	} {
		candidate := cloneCLIInputValues(continueValues)
		delete(candidate, key)
		if _, err := readGeneratedWorkerSessionsContinueInputs(candidate); err == nil {
			t.Errorf("readGeneratedWorkerSessionsContinueInputs(missing %s) = nil error, want typed input error", key)
		}
	}
	delete(continueValues, "you.worker-sessions.continue.arg.1")
	if got, err := readGeneratedWorkerSessionsContinueInputs(continueValues); err != nil {
		t.Fatalf("readGeneratedWorkerSessionsContinueInputs(missing optional prompt) error = %v", err)
	} else if len(got.followUpInput) != 0 {
		t.Fatalf("optional continue prompt = %#v, want empty", got.followUpInput)
	}
}

func TestWorkerSessionsGeneratedHandlersRejectMissingResolvedValues(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	tests := []struct {
		name   string
		values map[string]any
		call   func(*cobra.Command, map[string]any) error
	}{
		{
			name:   "list missing scope",
			values: map[string]any{"you.worker-sessions.list.flag.work-id": "work-1"},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsListWithValues(cmd, globals, diagnostics, func(workersessionscli.ListConfig) error { return nil }, values)
			},
		},
		{
			name: "list missing max results",
			values: map[string]any{
				"you.worker-sessions.list.flag.work-id": "work-1",
				"you.worker-sessions.list.flag.scope":   "all",
				"you.worker-sessions.list.flag.state":   []string{"RUNNING"},
			},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsListWithValues(cmd, globals, diagnostics, func(workersessionscli.ListConfig) error { return nil }, values)
			},
		},
		{
			name: "show missing output",
			values: map[string]any{
				"you.worker-sessions.show.flag.provider":          "codex",
				"you.worker-sessions.show.flag.worker-session-id": "worker-1",
				"you.worker-sessions.show.flag.kind":              "session_id",
				"you.worker-sessions.show.flag.id":                "provider-1",
				"you.worker-sessions.show.flag.session":           "session-1",
			},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsShowWithValues(cmd, globals, diagnostics, func(workersessionscli.ShowConfig) error { return nil }, values)
			},
		},
		{
			name: "stream missing follow",
			values: map[string]any{
				"you.worker-sessions.stream.flag.provider":          "codex",
				"you.worker-sessions.stream.flag.worker-session-id": "worker-1",
				"you.worker-sessions.stream.flag.kind":              "session_id",
				"you.worker-sessions.stream.flag.id":                "provider-1",
				"you.worker-sessions.stream.flag.session":           "session-1",
				"you.worker-sessions.stream.flag.output":            "json",
				"you.worker-sessions.stream.flag.replay-only":       false,
			},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsStreamWithValues(cmd, globals, diagnostics, func(workersessionscli.StreamConfig) error { return nil }, values)
			},
		},
		{
			name: "read missing output",
			values: map[string]any{
				"you.worker-sessions.read.flag.provider":          "codex",
				"you.worker-sessions.read.flag.worker-session-id": "worker-1",
				"you.worker-sessions.read.flag.kind":              "session_id",
				"you.worker-sessions.read.flag.id":                "provider-1",
				"you.worker-sessions.read.flag.session":           "session-1",
			},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsReadWithValues(cmd, globals, diagnostics, func(workersessionscli.ReadConfig) error { return nil }, values)
			},
		},
		{
			name:   "control missing output",
			values: map[string]any{"you.worker-sessions.cancel.arg.0": "worker-1"},
			call: func(cmd *cobra.Command, values map[string]any) error {
				return executeGeneratedWorkerSessionsControlWithValues(cmd, globals, diagnostics, func(workersessionscli.ControlConfig) error { return nil }, nil, workersessions.ControlActionCancel, values)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := commandWithResolvedCLIInputs(test.values)
			values, err := generatedCommandInputs(cmd)
			if err != nil {
				t.Fatalf("resolve test worker-session inputs: %v", err)
			}
			err = test.call(cmd, values)
			if err == nil || !strings.Contains(err.Error(), "resolved CLI input") {
				t.Fatalf("%s error = %v, want missing resolved CLI input", test.name, err)
			}
		})
	}
}

func commandWithResolvedCLIInputs(values map[string]any) *cobra.Command {
	command := &cobra.Command{}
	command.Annotations = make(map[string]string, len(values))
	index := 0
	for inputID, value := range values {
		flagName := fmt.Sprintf("resolved-input-%d", index)
		switch typed := value.(type) {
		case string:
			command.Flags().String(flagName, typed, "")
		case []string:
			command.Flags().Var(&cliTestStringSliceValue{values: typed}, flagName, "")
		case bool:
			command.Flags().Bool(flagName, typed, "")
		case int:
			command.Flags().Int(flagName, typed, "")
		default:
			panic(fmt.Sprintf("unsupported test CLI input type %T", value))
		}
		command.Annotations["infinite-you/input-id/"+inputID] = flagName
		index++
	}
	return command
}

type cliTestStringSliceValue struct {
	values []string
}

func (value *cliTestStringSliceValue) String() string {
	return strings.Join(value.values, ",")
}

func (value *cliTestStringSliceValue) Set(input string) error {
	value.values = []string{input}
	return nil
}

func (value *cliTestStringSliceValue) Type() string {
	return "stringArray"
}

func (value *cliTestStringSliceValue) Get() any {
	return append([]string(nil), value.values...)
}

func cloneCLIInputValues(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func TestWorkersListCommandReportsUnavailableProviderService(t *testing.T) {
	command := newWorkersListCommand(CommandFactory{homeDir: func() (string, error) { return t.TempDir(), nil }})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "Providers service is required") {
		t.Fatalf("workers list error = %v, want unavailable Providers service", err)
	}
}
