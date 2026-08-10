package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/spf13/cobra"
)

func TestRemoteRunRejectsLocalHostingBeforeRunSideEffects(t *testing.T) {
	const wantMessage = "--remote selects a running server through --server and cannot be combined with --with-server or --with-site; remove --remote for local hosting and use --listen <host:port> to choose an exact local bind"
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "persistent flags before run with server",
			args: []string{"--remote", "--server", "http://selected.example:9443", "run", "--with-server", "--no-record", "same request"},
		},
		{
			name: "persistent flags before run with site",
			args: []string{"--remote", "--server", "http://selected.example:9443", "run", "--with-site", "--no-record", "same request"},
		},
		{
			name: "persistent flags after run with server",
			args: []string{"run", "--with-server", "--no-record", "same request", "--remote", "--server", "http://selected.example:9443"},
		},
		{
			name: "persistent flags after run with site",
			args: []string{"run", "--with-site", "--no-record", "same request", "--remote", "--server", "http://selected.example:9443"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remoteCalls := 0
			localRunCalls := 0
			browserCalls := 0
			factory := withTestInjectedPlatformRoles(CommandFactory{
				remoteInvocation: rootRemoteInvocationFunc(func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
					remoteCalls++
					return factoryapi.InvocationResponse{}, nil
				}),
			})
			factory.browserOpener = func(context.Context, string) error {
				browserCalls++
				return nil
			}
			root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
				InitializeSystemFunc: func(context.Context, string) error {
					t.Fatal("system initialization should not run for rejected hosting placement")
					return nil
				},
				RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
					localRunCalls++
					return nil
				},
			})
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(test.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("remote/local hosting conflict error = nil")
			}
			var invocationErr *runcli.InvocationError
			if !errors.As(err, &invocationErr) {
				t.Fatalf("error = %v, want InvocationError", err)
			}
			if invocationErr.Code != runcli.RemoteLocalHostingConflictCode || invocationErr.Message != wantMessage {
				t.Fatalf("invocation error = %#v, want stable placement conflict", invocationErr)
			}
			var response struct {
				Code    string `json:"code"`
				Family  string `json:"family"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatalf("stderr = %q, want one JSON ErrorResponse: %v", stderr.String(), err)
			}
			if response.Code != runcli.RemoteLocalHostingConflictCode || response.Family != string(factoryapi.ErrorFamilyBadRequest) || response.Message != wantMessage {
				t.Fatalf("ErrorResponse = %#v, want stable bad-request placement conflict", response)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on rejected placement", stdout.String())
			}
			if remoteCalls != 0 || localRunCalls != 0 || browserCalls != 0 {
				t.Fatalf("side effects = remote %d, local run %d, browser %d; want all zero", remoteCalls, localRunCalls, browserCalls)
			}
		})
	}
}

func TestRunHelpDocumentsRemotePlacementAndLocalHosting(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("run --help: %v\nstderr=%s", err, stderr.String())
	}
	for _, marker := range []string{
		"--remote",
		"--server",
		"--listen",
		"conflicts with --remote",
		"you --remote --server <uri> run",
	} {
		if !strings.Contains(stdout.String(), marker) {
			t.Fatalf("run help missing %q:\n%s", marker, stdout.String())
		}
	}
}

func TestRemoteRunDispatchesExactNormalizedRequestWithoutOpeningLocalRun(t *testing.T) {
	var got runcli.RemoteInvocationRequest
	remote := rootRemoteInvocationFunc(func(_ context.Context, request runcli.RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
		got = request
		return factoryapi.InvocationResponse{
			RequestId: "remote-request", TraceId: "remote-trace",
			Status: factoryapi.InvocationTerminalStatusCompleted,
			PrimaryResult: contentcontract.GeneratedPtrFromParts([]work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "remote result",
			}}),
		}, nil
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{remoteInvocation: remote})
	factory.prepareInvocationInput = programmedTextInvocationInput(work.InputSourcePositionalText, "same request")

	localRunCalls := 0
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
			localRunCalls++
			return nil
		},
	})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	factoryPath, err := filepath.Abs(filepath.Join("factory", "factory.json"))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	selectedServer := "http://selected.example:9443/base"
	root.SetArgs([]string{
		"--remote", "--server", selectedServer, "run",
		"--factory", factoryPath, "--output", "primary", "same request",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("remote run: %v\nstderr=%s", err, stderr.String())
	}
	if localRunCalls != 0 {
		t.Fatalf("local run calls = %d, want zero", localRunCalls)
	}
	if got.Server != selectedServer {
		t.Fatalf("remote server = %q, want %q", got.Server, selectedServer)
	}
	parts := contentcontract.PartsFromGenerated(got.Request.Content)
	if len(parts) != 1 || parts[0].Text != "same request" {
		t.Fatalf("normalized remote request = %#v, want same request", got.Request.Content)
	}
	if stdout.String() != "remote result" {
		t.Fatalf("stdout = %q, want remote result", stdout.String())
	}
}

func TestRunServerPlacementRejectsRemoteLocalOnlyCommandBeforeRun(t *testing.T) {
	globals := &cliGlobalOptions{remote: true}
	options := withTestInjectedPlatformRoles(CommandFactory{})
	commands, err := buildRunServerProductionCommands(
		globals, &cliDiagnosticsOptions{}, &cliOperatorDefaultsOptions{}, options,
	)
	if err != nil {
		t.Fatalf("buildRunServerProductionCommands: %v", err)
	}
	root := &cobra.Command{Use: "you"}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(commands.Server)
	root.SetArgs([]string{"server"})
	err = root.Execute()
	if err == nil {
		t.Fatal("remote local-only server command error = nil")
	}
	want := `command "you server" supports local placement only; remove --remote`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want actionable placement error containing %q", err, want)
	}
}

func TestRemoteRunFailureDoesNotFallBackToLocalRun(t *testing.T) {
	remote := rootRemoteInvocationFunc(func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
		return factoryapi.InvocationResponse{}, errors.New("selected remote failed")
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{remoteInvocation: remote})
	factory.prepareInvocationInput = programmedTextInvocationInput(work.InputSourcePositionalText, "same request")
	localRunCalls := 0
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
			localRunCalls++
			return nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	factoryPath, err := filepath.Abs(filepath.Join("factory", "factory.json"))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	root.SetArgs([]string{
		"--remote", "--server", "http://selected.example:9443", "run",
		"--factory", factoryPath, "--output", "primary", "same request",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("remote failure error = nil")
	}
	if localRunCalls != 0 {
		t.Fatalf("local run calls after remote failure = %d, want zero", localRunCalls)
	}
}

type rootRemoteInvocationFunc func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.InvocationResponse, error)

func (fn rootRemoteInvocationFunc) InvokeFactory(ctx context.Context, request runcli.RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
	return fn(ctx, request)
}
