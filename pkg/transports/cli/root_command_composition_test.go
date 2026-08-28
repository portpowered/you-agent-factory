package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	serverstopcli "github.com/portpowered/infinite-you/pkg/transports/cli/serverstop"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/spf13/cobra"
)

func TestNewRootCommandFromSubcommandsAttachesInjectedCommands(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	docs := &cobra.Command{Use: "docs"}
	run := &cobra.Command{Use: "run"}

	got := NewRootCommandFromSubcommands(root, RootSubcommands{
		Commands: []*cobra.Command{docs, run},
	})

	if got != root {
		t.Fatal("root constructor replaced the injected root command")
	}
	if found, _, err := got.Find([]string{"docs"}); err != nil || found != docs {
		t.Fatalf("injected docs command = (%v, %v), want supplied command", found, err)
	}
	if found, _, err := got.Find([]string{"run"}); err != nil || found != run {
		t.Fatalf("injected run command = (%v, %v), want supplied command", found, err)
	}
}

func TestSessionListHelpOutputDocumentsCombinedHistoryWorkflow(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "session list",
			args: []string{"session", "list", "--help"},
			want: []string{
				"default scope is all",
				"--live-only",
				"--history-only",
				"recorded-history artifacts",
			},
		},
		{
			name: "session",
			args: []string{"session", "--help"},
			want: []string{
				"recorded history",
				"--live-only or --history-only",
				"mutually exclusive",
			},
		},
		{
			name: "root",
			args: []string{"--help"},
			want: []string{
				"session",
				"List, open, and close factory sessions on a running host",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute %s: %v", strings.Join(test.args, " "), err)
			}

			help := out.String()
			for _, want := range test.want {
				if !strings.Contains(help, want) {
					t.Fatalf("help missing %q:\n%s", want, help)
				}
			}
			if strings.Contains(strings.ToLower(help), "recording list") {
				t.Fatalf("help introduced a competing recording-list vocabulary:\n%s", help)
			}
		})
	}
}

func TestSessionListCommand_ConflictingFlagsFailBeforeHTTP(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--server", "http://127.0.0.1:1",
		"session", "list", "--live-only", "--history-only",
	})

	err := root.Execute()
	if err == nil ||
		!strings.Contains(err.Error(), "cannot be used together") ||
		!strings.Contains(err.Error(), "--live-only") ||
		!strings.Contains(err.Error(), "--history-only") {
		t.Fatalf("conflicting session-list flags error = %v, want actionable mutual-exclusion error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("conflict stdout = %q, want empty output", out.String())
	}
}

func TestSessionListCommand_ConflictingFlagsRenderTypedDiagnostic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := withTestInjectedPlatformRoles(CommandFactory{}).ExecuteCommand(startupcli.CommandInvocation{
		Arguments: []string{
			"--server", "http://127.0.0.1:1",
			"session", "list", "--live-only", "--history-only",
		},
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Context: context.Background(),
	})
	if err == nil {
		t.Fatal("conflicting session-list flags error = nil, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("conflict stdout = %q, want empty output", stdout.String())
	}
	for _, want := range []string{
		`"code":"CLI_FLAG_CONFLICT"`,
		`"family":"BAD_REQUEST"`,
		"cannot be used together",
		"--live-only",
		"--history-only",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("conflict stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestMain(m *testing.M) {
	// Cobra's Windows mousetrap check enumerates processes on every Execute.
	// These tests invoke commands in-process and never exercise Explorer launch
	// behavior, so avoid paying that external-system cost for each command tree.
	cobra.MousetrapHelpText = ""

	homeDir, err := os.MkdirTemp("", "you-cli-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create cli test home: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(homeDir)
	}()

	os.Setenv("HOME", homeDir)
	os.Setenv("USERPROFILE", homeDir)
	os.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	os.Setenv("HOMEPATH", string(os.PathSeparator))

	os.Exit(m.Run())
}

func TestProductionRunSubmitFamilyCutoverEnabled(t *testing.T) {
	root := (CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)
	for _, path := range [][]string{{"run"}, {"submit"}, {"submit", "batch"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if len(remaining) != 0 {
			t.Fatalf("Find(%v) remaining = %v, want none", path, remaining)
		}
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("Find(%v) lifecycle = (%t, %t), want retained PreRunE and RunE", path, cmd.PreRunE != nil, cmd.RunE != nil)
		}
	}

	assertDirectCommandCount(t, root, "run", 1)
	assertDirectCommandCount(t, root, "submit", 1)
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}
	assertDirectCommandCount(t, submitCmd, "batch", 1)
}

func TestProductionServerStopDispatchesOnlyInjectedOperation(t *testing.T) {
	var calls int
	var selected string
	factory := withTestInjectedPlatformRoles(CommandFactory{
		serverStopCLI: func(_ context.Context, config serverstopcli.Config) error {
			calls++
			selected = config.Server
			_, _ = config.Output.Write([]byte("stopped\n"))
			return nil
		},
	})
	var stdout, stderr bytes.Buffer
	err := factory.ExecuteCommand(startupcli.CommandInvocation{
		Arguments: []string{"--server", "http://127.0.0.1:7437", "server", "stop"},
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
		Context:   context.Background(),
		HomeDir:   func() (string, error) { return "operator-home", nil },
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("server stop error = %v; stderr=%s", err, stderr.String())
	}
	if calls != 1 || selected != "http://127.0.0.1:7437" {
		t.Fatalf("injected stop calls=%d selected=%q", calls, selected)
	}
	if stdout.String() != "stopped\n" {
		t.Fatalf("stdout = %q, want injected operation output", stdout.String())
	}
}

func TestProductionMetricsCostsTimeoutDiagnosticPreservesEndpointAcrossModes(t *testing.T) {
	const requestTimeout = 25 * time.Millisecond
	modes := [][]string{
		{"--server", "PLACEHOLDER", "metrics", "costs"},
		{"--server", "PLACEHOLDER", "--verbose", "metrics", "costs"},
		{"--server", "PLACEHOLDER", "--debug", "metrics", "costs"},
	}
	started := make(chan struct{}, len(modes))
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		started <- struct{}{}
		<-request.Context().Done()
	}))
	defer server.Close()

	operation := costscli.NewOperation(func(serverURL string) (costscli.Client, error) {
		return generatedclient.NewClientWithResponses(
			serverURL,
			generatedclient.WithHTTPClient(&http.Client{}),
		)
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{
		costsCLI: func(ctx context.Context, config costscli.CostsConfig) error {
			config.RequestTimeout = requestTimeout
			return operation(ctx, config)
		},
	})

	wantMessage := fmt.Sprintf(
		"GET /metrics/costs at %s timed out within the configured %s request timeout; retry or narrow the request with --session",
		server.URL,
		requestTimeout,
	)
	for _, template := range modes {
		args := append([]string(nil), template...)
		args[1] = server.URL
		var stdout, stderr bytes.Buffer
		err := factory.ExecuteCommand(startupcli.CommandInvocation{
			Arguments: args,
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
			Context:   context.Background(),
			HomeDir:   func() (string, error) { return "operator-home", nil },
			LookupEnv: func(string) (string, bool) { return "", false },
		})
		assertMetricsTimeoutCommand(t, args, err, stdout.String(), stderr.String(), wantMessage)
		waitForDelayedCostsRequest(t, started, args)
	}
}

func assertMetricsTimeoutCommand(
	t *testing.T,
	args []string,
	err error,
	stdout string,
	stderr string,
	wantMessage string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("ExecuteCommand(%v) error = nil, want timeout", args)
	}
	if stdout != "" {
		t.Fatalf("stdout for %v = %q, want empty failure output", args, stdout)
	}
	if slices.Contains(args, "--debug") {
		lines := strings.Split(strings.TrimSpace(stderr), "\n")
		if len(lines) < 2 {
			t.Fatalf("debug timeout diagnostic = %q, want structured diagnostic plus wrapped cause", stderr)
		}
		assertSingleMetricsDiagnostic(t, lines[0]+"\n", costscli.CostsRequestTimeoutCode, "INTERNAL_SERVER_ERROR", wantMessage)
		if !strings.Contains(strings.Join(lines[1:], "\n"), "debug: cause[0]=") {
			t.Fatalf("debug timeout diagnostic = %q, want wrapped cause context", stderr)
		}
	} else {
		assertSingleMetricsDiagnostic(t, stderr, costscli.CostsRequestTimeoutCode, "INTERNAL_SERVER_ERROR", wantMessage)
	}
	if strings.Contains(stderr, "CLI_COMMAND_FAILED") || strings.Contains(stderr, "INTERNAL_SERVER_ERROR: command failed") {
		t.Fatalf("timeout diagnostic for %v collapsed to a generic failure: %q", args, stderr)
	}
	var costsErr *costscli.CostsError
	if !errors.As(err, &costsErr) || costsErr.CLIErrorCode() != costscli.CostsRequestTimeoutCode {
		t.Fatalf("ExecuteCommand(%v) error = %v, want typed timeout", args, err)
	}
}

func waitForDelayedCostsRequest(t *testing.T, started <-chan struct{}, args []string) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("delayed costs server did not receive %v", args)
	}
}
