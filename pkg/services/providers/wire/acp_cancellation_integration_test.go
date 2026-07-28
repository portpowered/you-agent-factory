package wire_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// Cancellation is tested at the Provider attempt boundary because a terminal
// one-shot `you run` deliberately converts a cancelled dispatch into terminal
// Work. This keeps the assertion on the ACP RPC/process lifecycle itself.
func TestProviderServiceCancelsBlockingACPAttemptsForCancellationAndDeadline(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{name: "cancellation", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		}, want: context.Canceled},
		{name: "deadline", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 500*time.Millisecond)
		}, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			signal := filepath.Join(t.TempDir(), "prompt-started")
			started := make(chan *exec.Cmd, 1)
			service, err := providerswire.New(func(name string, args ...string) *exec.Cmd {
				if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
					cmd := exec.Command(os.Args[0], "-test.run=^TestProvidersACPAgentProcess$")
					started <- cmd
					return cmd
				}
				return exec.Command(name, args...)
			}, integrationExecutableLocator{})
			if err != nil {
				t.Fatalf("construct Providers service: %v", err)
			}

			ctx, cancel := test.ctx()
			defer cancel()
			stream, err := service.ExecuteStream(ctx, providers.ExecuteRequest{
				ProviderID: "cursor-acp", Instructions: "block until cancellation",
				Prompt:           []providers.ContentPart{{Kind: providers.ContentKindText, Text: "cancel ACP"}},
				WorkingDirectory: t.TempDir(),
				Environment: []providers.EnvironmentEntry{
					{Name: providersACPModeEnvironment, Value: "block"},
					{Name: "YOU_TEST_ACP_PROMPT_SIGNAL", Value: signal},
				},
			})
			if err != nil {
				t.Fatalf("ExecuteStream() error = %v", err)
			}
			defer stream.Close()

			var child *exec.Cmd
			select {
			case child = <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("ACP child process did not start")
			}
			waitForFile(t, signal, 5*time.Second)
			if test.want == context.Canceled {
				cancel()
			}
			for range stream.Updates {
			}
			outcome, ok := <-stream.Outcome
			if !ok {
				t.Fatal("ACP outcome stream closed without a result")
			}
			if !errors.Is(outcome.Err, test.want) {
				t.Fatalf("ACP cancellation outcome = %v, want %v", outcome.Err, test.want)
			}
			if child.ProcessState == nil {
				t.Fatal("ACP child process was not joined before the outcome was published")
			}
		})
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
