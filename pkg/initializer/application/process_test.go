package application

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/spf13/cobra"
)

type processContextTestKey struct{}

func TestProcessExecuteUsesInjectedLifecycleAndInvocationInputs(t *testing.T) {
	t.Parallel()

	lifecycleCalls := 0
	stopCalls := 0
	wantWorkingDirectory := t.TempDir()
	factory := processCommandFactory{newCommand: func(func(string) (string, bool)) *cobra.Command {
		return &cobra.Command{Use: "you", RunE: func(cmd *cobra.Command, _ []string) error {
			if got, _ := cmd.Context().Value(processContextTestKey{}).(string); got != "injected" {
				return fmt.Errorf("process context marker = %q, want injected", got)
			}
			if got := startupcli.WorkingDirectory(cmd.Context()); got != wantWorkingDirectory {
				return fmt.Errorf("working directory = %q, want %q", got, wantWorkingDirectory)
			}
			if !startupcli.StdinIsTTY(cmd.Context()) {
				return fmt.Errorf("stdin terminal classification was not propagated")
			}
			if !startupcli.StdoutIsTTY(cmd.Context()) {
				return fmt.Errorf("stdout terminal classification was not propagated")
			}
			return nil
		}}
	}}
	initializer := startupcli.Functions{ProcessContextFunc: func(parent context.Context) (context.Context, func()) {
		lifecycleCalls++
		return context.WithValue(parent, processContextTestKey{}, "injected"), func() { stopCalls++ }
	}}
	process := newProcessForTest(t, factory, initializer)
	stdinTTY, stdoutTTY := true, true
	if err := process.Execute(Input{
		Args: []string{"you"}, Env: homeEnvironmentForProcessTest(t.TempDir()),
		Stdin: strings.NewReader(""), WorkingDirectory: wantWorkingDirectory,
		StdinIsTTY: &stdinTTY, StdoutIsTTY: &stdoutTTY,
	}); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	if lifecycleCalls != 1 || stopCalls != 1 {
		t.Fatalf("lifecycle calls/stops = %d/%d, want 1/1", lifecycleCalls, stopCalls)
	}
}

func TestProcessExecuteHonorsExplicitTerminalEdgeOverrides(t *testing.T) {
	t.Parallel()

	stdinTTY := true
	stdoutTTY := false
	factory := processCommandFactory{newCommand: func(func(string) (string, bool)) *cobra.Command {
		return &cobra.Command{Use: "you", RunE: func(cmd *cobra.Command, _ []string) error {
			if !startupcli.StdinIsTTY(cmd.Context()) {
				return fmt.Errorf("stdin override was not propagated")
			}
			if startupcli.StdoutIsTTY(cmd.Context()) {
				return fmt.Errorf("stdout override was not propagated")
			}
			return nil
		}}
	}}
	process := newProcessForTest(t, factory, startupcli.Functions{})
	if err := process.Execute(Input{
		Args: []string{"you"}, Env: homeEnvironmentForProcessTest(t.TempDir()),
		StdinIsTTY: &stdinTTY, StdoutIsTTY: &stdoutTTY, WorkingDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
}

func TestProcessSupportsConcurrentDaemonAndClientInvocations(t *testing.T) {
	t.Parallel()

	daemonStarted := make(chan struct{})
	factory := processCommandFactory{
		newCommand: func(lookupEnv func(string) (string, bool)) *cobra.Command {
			root := &cobra.Command{Use: "you", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(&cobra.Command{
				Use: "daemon",
				RunE: func(cmd *cobra.Command, _ []string) error {
					_, _ = io.WriteString(cmd.OutOrStdout(), "daemon:"+environmentValue(lookupEnv, "PROCESS_TEST")+"\n")
					close(daemonStarted)
					<-cmd.Context().Done()
					return cmd.Context().Err()
				},
			})
			root.AddCommand(&cobra.Command{
				Use:  "client [value]",
				Args: cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					_, err := fmt.Fprintf(
						cmd.OutOrStdout(),
						"client:%s:%s\n",
						environmentValue(lookupEnv, "PROCESS_TEST"),
						args[0],
					)
					return err
				},
			})
			return root
		},
	}
	process := newProcessForTest(t, factory, startupcli.Functions{})

	daemonCtx, cancelDaemon := context.WithCancel(context.Background())
	var daemonOutput bytes.Buffer
	daemonDone := make(chan error, 1)
	daemonHome := t.TempDir()
	go func() {
		daemonDone <- process.Execute(Input{
			Args:             []string{"you", "daemon"},
			Env:              append(homeEnvironmentForProcessTest(daemonHome), "PROCESS_TEST=daemon"),
			Stdout:           &daemonOutput,
			Context:          daemonCtx,
			WorkingDirectory: daemonHome,
		})
	}()

	select {
	case <-daemonStarted:
	case <-time.After(time.Second):
		t.Fatal("daemon invocation did not start")
	}

	var clientOutput bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "client", "first"},
		Env:              append(homeEnvironmentForProcessTest(t.TempDir()), "PROCESS_TEST=client"),
		Stdout:           &clientOutput,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("Process.Execute(client) error = %v", err)
	}
	if got := clientOutput.String(); got != "client:client:first\n" {
		t.Fatalf("client output = %q", got)
	}
	if got := daemonOutput.String(); got != "daemon:daemon\n" {
		t.Fatalf("daemon output = %q", got)
	}

	cancelDaemon()
	select {
	case err := <-daemonDone:
		if err != context.Canceled {
			t.Fatalf("daemon error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon invocation did not stop")
	}

	var laterOutput bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "client", "second"},
		Env:              append(homeEnvironmentForProcessTest(t.TempDir()), "PROCESS_TEST=later"),
		Stdout:           &laterOutput,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("Process.Execute(client after daemon) error = %v", err)
	}
	if got := laterOutput.String(); got != "client:later:second\n" {
		t.Fatalf("later output = %q", got)
	}
}

func TestProcessPropagatesInvocationWorkingDirectoryWithoutChangingHostProcess(t *testing.T) {
	t.Parallel()

	want := t.TempDir()
	factory := processCommandFactory{
		newCommand: func(func(string) (string, bool)) *cobra.Command {
			return &cobra.Command{
				Use: "you",
				RunE: func(cmd *cobra.Command, _ []string) error {
					_, err := io.WriteString(cmd.OutOrStdout(), startupcli.WorkingDirectory(cmd.Context()))
					return err
				},
			}
		},
	}
	process := newProcessForTest(t, factory, startupcli.Functions{})
	var output bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you"},
		Env:              homeEnvironmentForProcessTest(t.TempDir()),
		Stdout:           &output,
		WorkingDirectory: want,
	}); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	if got := output.String(); got != want {
		t.Fatalf("working directory = %q, want %q", got, want)
	}
}

type processCommandFactory struct {
	newCommand func(func(string) (string, bool)) *cobra.Command
}

func newProcessForTest(
	t *testing.T,
	factory startupcli.CommandFactory,
	initializer startupcli.Initializer,
) *Process {
	t.Helper()
	process, err := NewProcess(factory, initializer, processTestProviderRegistry{})
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}
	return process
}

func TestProcessRequiresAndExposesProviderRegistry(t *testing.T) {
	t.Parallel()

	if process, err := NewProcess(nil, nil, nil); err == nil || process != nil {
		t.Fatalf("NewProcess(nil registry) = (%#v, %v), want construction failure", process, err)
	}
	if registry := (*Process)(nil).ProviderRegistry(); registry != nil {
		t.Fatalf("nil Process.ProviderRegistry() = %#v, want nil", registry)
	}

	want := processTestProviderRegistry{}
	process, err := NewProcess(nil, nil, want)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}
	if got := process.ProviderRegistry(); got != want {
		t.Fatalf("ProviderRegistry() = %#v, want %#v", got, want)
	}
}

type processTestProviderRegistry struct{}

func (processTestProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}

func (factory processCommandFactory) ExecuteCommand(input startupcli.CommandInvocation) error {
	command := factory.newCommand(input.LookupEnv)
	command.SetArgs(append([]string(nil), input.Arguments...))
	command.SetIn(input.Stdin)
	command.SetOut(input.Stdout)
	command.SetErr(input.Stderr)
	command.SetContext(input.Context)
	return command.Execute()
}

func environmentValue(lookup func(string) (string, bool), name string) string {
	value, _ := lookup(name)
	return value
}

func homeEnvironmentForProcessTest(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home}
}
