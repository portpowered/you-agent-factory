package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	acpfixture "github.com/portpowered/infinite-you/tests/functional/workers/invoke_continue/testdata/acpfixture"
	"io"
)

// F7-C5/C6/C7: one reusable root per immutable negotiated capability shape.
// Sources are direct executions correlated to explicit Factory Sessions; all
// observations and controls enter public CLI/HTTP, with scenario-owned pipes.
func TestCapturedACPContinuation(t *testing.T) {
	t.Parallel()
	for _, shape := range []struct {
		name               string
		supported, changed bool
	}{
		{"supported", true, false}, {"unsupported", false, false}, {"changed-handshake", true, true},
	} {
		t.Run(shape.name, func(t *testing.T) {
			t.Parallel()
			runCapturedACPShape(t, shape.supported, shape.changed)
		})
	}
}

func runCapturedACPShape(t *testing.T, supported, changed bool) {
	t.Helper()
	root := t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	routes := make(map[string]*continuationACPPeer)
	cases := []string{"load"}
	if supported {
		cases = append(cases, "stale")
	}
	if changed {
		// Negotiated facts are provider-wide. A peer that changes those facts
		// needs a distinct immutable edge shape, independent of stable peers.
		cases = []string{"changed-handshake"}
	}
	for _, name := range cases {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		routes[dir] = &continuationACPPeer{loadSupported: supported, stale: name == "stale", changedHandshake: name == "changed-handshake", ready: make(chan struct{})}
	}
	started, err := startInvokeContinuePackageProcessWithEdges(t, host, home, &invokeContinueStaticCommandRoute{}, serviceedges.Edges{
		// Controlled policy authorizes resume for this peer. Both shapes share
		// that policy; the negative shape must still honor its actual handshake.
		ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{Provider: providers.IDCursor,
			Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilitySessionResume}}},
		PlatformProcessCommandFactory: acpfixture.CommandFactory(),
		ProvidersExecutableLocator:    continuationACPLocator{},
		ProvidersStdioPipeFactory: acpfixture.StdioFactory(func(dir string, reader io.Reader, writer io.Writer) {
			if peer := routes[dir]; peer != nil {
				peer.serve(reader, writer)
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := started.command.stop(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := started.process.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	fixture := &invokeContinuePackageFixture{process: started.process, baseURL: started.baseURL, hostDir: host}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, name)
			scenario := &invokeContinueScenario{fixture: fixture, name: "acp-" + name, runNumber: fixture.scenarioRuns.Add(1),
				workingDirectory: dir, homeDirectory: home, session: fixture.openSession(t)}
			runCapturedACPContinuation(t, scenario, routes[dir])
		})
	}
}

func runCapturedACPContinuation(t *testing.T, scenario *invokeContinueScenario, peer *continuationACPPeer) {
	t.Helper()
	source, successor := scenarioScopedID(scenario, "source"), scenarioScopedID(scenario, "successor")
	invokeCapturedACPSource(t, scenario, peer, source)
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, source, true) })
	assertNativeContinuationObservation(t, scenario, source, "RUNNING", "opaque-acp-source", "", "")
	if !peer.loadSupported {
		assertUnsupportedACPInterrupt(t, scenario, source, successor)
	}
	stopNativeContinuationWorker(t, scenario, source, true)
	assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-acp-source", "", "")
	args := []string{"worker-sessions", "continue", source, "--request-id", scenarioScopedID(scenario, "continue"),
		"--successor-worker-session-id", successor, "--user-message", "ACP follow-up"}
	if !peer.loadSupported || peer.stale || peer.changedHandshake {
		assertFailedACPContinuation(t, scenario, peer, args, successor)
	} else {
		result := raceNativeContinuationCLI(t, scenario, args)
		var continued directWorkerSessionCLIResult
		decodeDirectWorkerSessionResult(t, result.Stdout(), &continued)
		if !continued.Accepted || continued.State != "COMPLETED" || continued.SuccessorWorkerSessionID != successor || !strings.Contains(continued.Output, "ACP continued COMPLETE") {
			t.Fatalf("ACP continuation result: %#v", continued)
		}
		assertNativeContinuationObservation(t, scenario, successor, "COMPLETED", "opaque-acp-source", source, "")
		assertNativeContinuationObservation(t, scenario, source, "TERMINATED", "opaque-acp-source", "", successor)
	}
	assertCapturedACPRPCs(t, peer, scenario.workingDirectory)
}

func invokeCapturedACPSource(t *testing.T, scenario *invokeContinueScenario, peer *continuationACPPeer, source string) {
	t.Helper()
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{requestID: source + "-request", workerSessionID: source,
		dispatchID: source + "-attempt", factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "ACP source"})
	execution := document["execution"].(map[string]any)
	execution["runnerId"], execution["executorProvider"], execution["modelProvider"] = "cursor", "cursor", "cursor"
	writeInvokeContinueJSON(t, path, document)
	_ = executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "invoke", "--execution", path, "--async"}, true)
	select {
	case <-peer.ready:
	case <-time.After(10 * time.Second):
		result := executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "show", "--worker-session-id", source}, true)
		t.Fatalf("ACP source not ready: calls=%v observation=%s", peer.requests(), result.Stdout())
	}
}

func assertUnsupportedACPInterrupt(t *testing.T, scenario *invokeContinueScenario, source, successor string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, []string{"worker-sessions", "interrupt", source,
			"--request-id", scenarioScopedID(scenario, "interrupt"), "--successor-worker-session-id", successor,
			"--replacement-message", "ACP follow-up", "--async"})
		assertDirectWorkerSessionCLIError(t, request, "PROVIDER_UNSUPPORTED")
		assertNativeContinuationObservation(t, scenario, source, "RUNNING", "opaque-acp-source", "", "")
	}
}

func assertFailedACPContinuation(t *testing.T, scenario *invokeContinueScenario, peer *continuationACPPeer, args []string, successor string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		request := missingReferenceRequest(t, scenario, remote, args)
		code := "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID"
		if peer.stale || peer.changedHandshake {
			code = "WORKER_SESSION_FAILED"
		}
		assertDirectWorkerSessionCLIError(t, request, code)
	}
	if !peer.loadSupported {
		absent := missingReferenceRequest(t, scenario, true, []string{"worker-sessions", "show", "--worker-session-id", successor})
		assertDirectWorkerSessionCLIError(t, absent, "WORKER_SESSION_NOT_FOUND")
	} else {
		result := executeNativeContinuationCLI(t, scenario, []string{"worker-sessions", "show", "--worker-session-id", successor}, true)
		var failed struct {
			State                      string
			PredecessorWorkerSessionID string
		}
		decodeDirectWorkerSessionResult(t, result.Stdout(), &failed)
		if failed.State != "FAILED" || failed.PredecessorWorkerSessionID == "" {
			t.Fatalf("failed ACP continuation lost public state/lineage: %s", result.Stdout())
		}
	}
}

func assertCapturedACPRPCs(t *testing.T, peer *continuationACPPeer, dir string) {
	t.Helper()
	counts := make(map[string]int)
	for _, call := range peer.requests() {
		counts[call.Method]++
		if call.Method == "session/load" {
			var params struct{ SessionID, Cwd string }
			if err := json.Unmarshal(call.Params, &params); err != nil || params.SessionID != "opaque-acp-source" || params.Cwd != dir {
				t.Fatalf("ACP load lost captured identity/workspace: %s (%v)", call.Params, err)
			}
		}
		assertCapturedACPInput(t, call, counts[call.Method])
	}
	wantLoads, wantPrompts := 0, 1
	if peer.loadSupported {
		if !peer.changedHandshake {
			wantLoads = 1
		}
		if !peer.stale && !peer.changedHandshake {
			wantPrompts = 2
		}
	}
	if counts["session/new"] != 1 || counts["session/load"] != wantLoads || counts["session/prompt"] != wantPrompts {
		t.Fatalf("ACP fallback/duplicate or missing prompt: counts=%v", counts)
	}
}

func assertCapturedACPInput(t *testing.T, call continuationACPRPC, ordinal int) {
	t.Helper()
	if call.Method == "session/prompt" && ordinal == 2 {
		var params struct {
			SessionID string
			Prompt    []struct{ Type, Text string }
		}
		if err := json.Unmarshal(call.Params, &params); err != nil || params.SessionID != "opaque-acp-source" ||
			len(params.Prompt) != 1 || params.Prompt[0].Text != "ACP follow-up" {
			t.Fatalf("ACP load-then-prompt changed identity/input: %s (%v)", call.Params, err)
		}
	}
	if call.Method == "session/set_config_option" {
		var params struct{ ConfigID, Value string }
		if err := json.Unmarshal(call.Params, &params); err != nil || params.ConfigID != "model" || params.Value != "functional-model" {
			t.Fatalf("ACP model changed: %s (%v)", call.Params, err)
		}
	}
}
