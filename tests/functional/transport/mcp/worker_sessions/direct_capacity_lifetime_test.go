package workersessions_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each cohort owns a different immutable outcome and host-wide capacity. Within
// a cohort, one process serves START, observation, control and replacement.
func TestDirectCapacityJoinedLifetime(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancel", "terminate"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			client := newSelectedHostClientProcess(t)
			runner := &joinedCapacityProvider{outcome: outcome, started: make(chan struct{}, 2), canceled: make(chan struct{}), release: make(chan struct{}), replacement: make(chan struct{})}
			host, workDir := newCapacityLifetimeHost(t, runner)
			t.Cleanup(runner.unblock)
			session, ctx := startMCP(t, client, host.URL())
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			start := func(id string) *mcp.CallToolResult {
				return callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs(id, workDir))
			}
			assertSubagentAdmission(t, start("held"), "held")
			waitCapacityGate(t, ctx, runner.started)
			assertToolError(t, start("excess"), "worker_session.capacity_exceeded", true)
			var control <-chan *mcp.CallToolResult
			if outcome == "cancel" || outcome == "terminate" {
				done := make(chan *mcp.CallToolResult, 1)
				control = done
				go func() {
					result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "CONTROL", "workerSessionId": "held", "operation": strings.ToUpper(outcome)}})
					if err != nil {
						done <- nil
						return
					}
					done <- result
				}()
				waitCapacityGate(t, ctx, runner.canceled)
				// Cancellation is acknowledged at the edge, but the effect has
				// not joined. It still owns the slot even if projection is terminal.
				assertToolError(t, start("before-join"), "worker_session.capacity_exceeded", true)
			}
			runner.releaseOnce.Do(func() { close(runner.release) })
			if control != nil {
				select {
				case result := <-control:
					if result == nil || result.IsError {
						t.Fatal("joined public control failed")
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			states := map[string]string{"success": "COMPLETED", "failure": "FAILED", "cancel": "CANCELED", "terminate": "TERMINATED"}
			followCapacityTerminal(t, ctx, client, host.URL(), workDir, "held", states[outcome])
			assertSubagentAdmission(t, start("replacement"), "replacement")
			waitCapacityGate(t, ctx, runner.started)
			assertToolError(t, start("replacement-excess"), "worker_session.capacity_exceeded", true)
			assertSubagentAdmission(t, start("held"), "held")
			assertToolError(t, start("replay-excess"), "worker_session.capacity_exceeded", true)
			if runner.calls.Load() != 2 {
				t.Fatalf("refusal/replay effect count=%d", runner.calls.Load())
			}
			runner.replacementOnce.Do(func() { close(runner.replacement) })
			followCapacityTerminal(t, ctx, client, host.URL(), workDir, "replacement", "COMPLETED")
		})
	}
}

func TestDirectCapacitySingleSlotRace(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	runner := &joinedCapacityProvider{outcome: "success", started: make(chan struct{}, 2), canceled: make(chan struct{}), release: make(chan struct{}), replacement: make(chan struct{})}
	host, workDir := newCapacityLifetimeHost(t, runner)
	t.Cleanup(runner.unblock)
	session, ctx := startMCP(t, client, host.URL())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type response struct {
		id     string
		result *mcp.CallToolResult
		err    error
	}
	results := make(chan response, 2)
	for _, id := range []string{"racing-a", "racing-b"} {
		go func() {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: capacityLifetimeArgs(id, workDir)})
			results <- response{id, result, err}
		}()
	}
	winner, refused := "", ""
	for range 2 {
		select {
		case r := <-results:
			if r.err != nil || r.result == nil {
				t.Fatalf("racing START transport=%v", r.err)
			}
			if r.result.IsError {
				assertToolError(t, r.result, "worker_session.capacity_exceeded", true)
				if refused != "" {
					t.Fatal("both racing requests refused")
				}
				refused = r.id
			} else {
				assertSubagentAdmission(t, r.result, r.id)
				if winner != "" {
					t.Fatal("both racing requests admitted")
				}
				winner = r.id
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	waitCapacityGate(t, ctx, runner.started)
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": refused}), "worker_session.not_found", false)
	if runner.calls.Load() != 1 {
		t.Fatalf("race provider effects=%d", runner.calls.Load())
	}
	runner.releaseOnce.Do(func() { close(runner.release) })
	followCapacityTerminal(t, ctx, client, host.URL(), workDir, winner, "COMPLETED")
}

func newCapacityLifetimeHost(t *testing.T, runner platformprocess.CommandRunner) (*support.FunctionalAPIServer, string) {
	t.Helper()
	home, workDir := t.TempDir(), t.TempDir()
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"defaults":{"directSessionCapacity":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: support.ScaffoldSingleStepFactory(t, "capacity-lifetime"), WorkingDirectory: workDir,
		Edges: serviceedges.Edges{FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }, ProviderCommandRunner: runner,
			ProviderCatalogProbe: func(_ context.Context, d providers.Descriptor) (providers.Descriptor, error) {
				d.Readiness = providers.ReadinessReady
				return d, nil
			}},
	})
	return host, workDir
}

func capacityLifetimeArgs(id, workDir string) map[string]any {
	return map[string]any{"requestId": id, "workerSessionId": id, "prompt": id, "provider": "codex", "model": "controlled-model", "workingRoot": workDir}
}

func waitCapacityGate(t *testing.T, ctx context.Context, gate <-chan struct{}) {
	t.Helper()
	select {
	case <-gate:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func followCapacityTerminal(t *testing.T, ctx context.Context, client selectedHostClientProcess, host, workDir, id, state string) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{"you", "--server", host, "--json", "worker-sessions", "follow", "--terminal", id})
	inputs.Input.WorkingDirectory = workDir
	if err := client.Execute(inputs.Input); err != nil {
		t.Fatalf("terminal follow: %v", err)
	}
	if !strings.Contains(inputs.Stdout(), `"state":"`+state+`"`) || strings.Count(strings.TrimSpace(inputs.Stdout()), "\n") != 0 {
		t.Fatalf("terminal result for %s=%s", id, inputs.Stdout())
	}
}

type joinedCapacityProvider struct {
	outcome                                 string
	started, canceled, release, replacement chan struct{}
	releaseOnce, replacementOnce            sync.Once
	calls                                   atomic.Int32
}

func (r *joinedCapacityProvider) unblock() {
	r.releaseOnce.Do(func() { close(r.release) })
	r.replacementOnce.Do(func() { close(r.replacement) })
}

func (r *joinedCapacityProvider) Run(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	r.started <- struct{}{}
	if string(command.Stdin) == "replacement" {
		select {
		case <-r.replacement:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	} else {
		if r.outcome == "cancel" || r.outcome == "terminate" {
			<-ctx.Done()
			close(r.canceled)
		}
		// Explicit physical join barrier; cancellation alone cannot release it.
		<-r.release
		if ctx.Err() != nil {
			return platformprocess.CommandResult{}, ctx.Err()
		}
		if r.outcome == "failure" {
			return platformprocess.CommandResult{}, errors.New("controlled provider failure")
		}
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("controlled completion")}, nil
}
