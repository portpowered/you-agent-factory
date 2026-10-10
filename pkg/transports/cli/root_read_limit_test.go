package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func TestWorkerSessionsReadLimitHandoff(t *testing.T) {
	t.Parallel()
	for _, output := range []string{"human", "json", "global-json"} {
		for _, follow := range []bool{false, true} {
			for _, limit := range []string{"", "0", "-1", "1", "1000", "1001"} {
				t.Run(fmt.Sprintf("%s/follow=%t/limit=%s", output, follow, limit), func(t *testing.T) {
					t.Parallel()
					calls := 0
					var got workersessionscli.ReadConfig
					factory := withTestInjectedPlatformRoles(CommandFactory{
						ReadWorkerSession:        func(config workersessionscli.ReadConfig) error { calls++; got = config; return nil },
						factoryConfigInitHandler: testFactoryConfigInitHandler(CommandFactory{}), sessionResolvedHandlers: testSessionHandlers(nil, nil),
					})
					root := factory.NewCommand(context.Background(), nil, nil, nil)
					var stdout, stderr bytes.Buffer
					root.SetOut(&stdout)
					root.SetErr(&stderr)
					args := []string{"worker-sessions", "read", "--worker-session-id", "limit-worker", "--view", "logs"}
					if output == "global-json" {
						args = append([]string{"--json"}, args...)
					} else {
						args = append(args, "--output", output)
					}
					if follow {
						args = append(args, "--follow")
					}
					if limit != "" {
						args = append(args, "--limit", limit)
					}
					root.SetArgs(args)
					err := root.Execute()
					if limit == "0" || limit == "-1" || limit == "1001" {
						assertReadLimitFailure(t, err)
						if calls != 0 || stdout.Len() != 0 {
							t.Fatalf("invalid limit: calls=%d stdout=%q", calls, stdout.String())
						}
						return
					}
					want := 0
					if limit == "1" {
						want = 1
					}
					if limit == "1000" {
						want = 1000
					}
					assertReadLimitHandoff(t, err, calls, got, want, follow, output != "human")
				})
			}
		}
	}
}

func assertReadLimitFailure(t *testing.T, err error) {
	t.Helper()
	var failure *clidiag.LocalFailure
	if !errors.As(err, &failure) || failure.CLIErrorCode() != "BAD_REQUEST" || string(failure.CLIErrorFamily()) != "BAD_REQUEST" || failure.CLIErrorMessage() != "--limit must be between 1 and 1000" {
		t.Fatalf("error = %v, want typed actionable bad request", err)
	}
}
func assertReadLimitHandoff(t *testing.T, err error, calls int, got workersessionscli.ReadConfig, limit int, follow, jsonOutput bool) {
	t.Helper()
	if err != nil || calls != 1 || got.Limit != limit || got.Follow != follow || got.View != "logs" || got.JSON != jsonOutput {
		t.Fatalf("handoff error=%v calls=%d config=%+v", err, calls, got)
	}
}
