package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSelectedSessionDiagnosticsAndConcurrentOutput(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	base := zap.New(core).With(zap.String("backend", "selected-diagnostics"))
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 16)}
	process := support.BuildProcess(t, serviceedges.Edges{ProcessLogger: base, ProviderCommandRunner: runner})
	config := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	flags := [][]string{{"--quiet"}, {}, {"--verbose", "--debug"}, {"--json", "--output", "primary"}, {"--json", "--output", "response-stream"}}
	runs := make([]selectedInvocation, 0, len(flags))
	for _, selection := range flags {
		runs = append(runs, startSelectedOutputInvocation(t, process, t.Context(), config, selection, "private-prompt-canary"))
	}
	// File output uses the documented primary stdout redirection contract.
	// Process.Execute receives the invocation-owned file writer directly.
	path := filepath.Join(t.TempDir(), "selected-result.txt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	runs = append(runs, startSelectedWriterInvocation(t, process, t.Context(), config, []string{"--output", "primary"}, "private-prompt-canary", file))
	// All provider commands enter before any release, proving actual overlap
	// without a process-wide call lock or a host-time synchronization delay.
	calls := make([]selectedCommandCall, 0, len(runs))
	for range runs {
		calls = append(calls, selectedAwait(t, runner.calls))
	}
	releaseSelectedOutputCalls(calls, runs)
	for index, run := range runs {
		if err := selectedAwait(t, run.done); err != nil {
			t.Fatal(err)
		}
		if index == len(flags) {
			assertSelectedFileOutput(t, run, path)
		} else {
			assertSelectedOutputMode(t, index, run, runs)
		}
		assertSelectedSessionLog(t, logs, run)
	}
	base.Info("unchanged selected base")
	fields := logs.FilterMessage("unchanged selected base").All()[0].ContextMap()
	if len(fields) != 1 || fields["backend"] != "selected-diagnostics" {
		t.Fatalf("base metadata changed: %#v", fields)
	}
	t.Log("L01/L03 and L04 JSON/NDJSON: concurrent selected-backend operations retain correlation and invocation framing")
}

func releaseSelectedOutputCalls(calls []selectedCommandCall, runs []selectedInvocation) {
	for _, call := range calls {
		for _, run := range runs {
			if call.request.ExecutionScopeID == run.sessionID {
				call.reply <- platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(run.sessionID + " COMPLETE")}
				break
			}
		}
	}
}

func assertSelectedFileOutput(t *testing.T, run selectedInvocation, path string) {
	t.Helper()
	stdout, stderr := run.stdout(), run.inputs.Stderr()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != run.sessionID+" COMPLETE" || stdout != "" || stderr != "" {
		t.Fatalf("selected file = %q, %v; stdout=%q stderr=%q", content, err, stdout, stderr)
	}
}

func assertSelectedOutputMode(t *testing.T, index int, run selectedInvocation, runs []selectedInvocation) {
	t.Helper()
	stdout, stderr := run.stdout(), run.inputs.Stderr()
	if stderr != "" || !strings.Contains(stdout, run.sessionID) {
		t.Fatalf("mode %d stdout=%q stderr=%q", index, stdout, stderr)
	}
	for _, peer := range runs {
		if peer.sessionID != run.sessionID && strings.Contains(stdout+stderr, peer.sessionID) {
			t.Fatal("peer output crossed invocation boundary")
		}
	}
	switch index {
	case 0:
		if stdout != run.sessionID+" COMPLETE" {
			t.Fatalf("quiet raw result = %q", stdout)
		}
	case 1, 2:
		if !strings.Contains(stdout, "--- primary result ---\n"+run.sessionID) {
			t.Fatalf("human framing = %q", stdout)
		}
	case 3:
		assertSelectedDiagnosticJSON(t, stdout)
	case 4:
		assertSelectedDiagnosticNDJSON(t, stdout)
	}
}

func assertSelectedDiagnosticJSON(t *testing.T, stdout string) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var response factoryapi.InvocationResponse
	if err := decoder.Decode(&response); err != nil || response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("JSON response = %#v, %v", response, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("diagnostics contaminated JSON: %v", err)
	}
}

func assertSelectedSessionLog(t *testing.T, logs *observer.ObservedLogs, run selectedInvocation) {
	t.Helper()
	for _, entry := range logs.FilterMessage("loading factory config").All() {
		fields := entry.ContextMap()
		if fields["session_id"] == run.sessionID {
			folder, folderOK := fields["folder_path"].(string)
			factory, factoryOK := fields["factory_dir"].(string)
			// --factory supplied the config file, while folder_path identifies
			// its containing working directory. Preserve that existing contract.
			if fields["backend"] != "selected-diagnostics" || !folderOK || !factoryOK || filepath.Clean(folder) != filepath.Clean(run.dir) || filepath.Clean(factory) != filepath.Join(run.dir, factorydefinitions.FactoryConfigFile) {
				t.Fatalf("session operation attribution = %#v", fields)
			}
			return
		}
	}
	t.Fatalf("session %s never reached selected diagnostic backend", run.sessionID)
}

func assertSelectedDiagnosticNDJSON(t *testing.T, stdout string) {
	t.Helper()
	terminal := 0
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for index, line := range lines {
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("diagnostic contamination of NDJSON: %v", err)
		}
		var kind string
		if err := json.Unmarshal(record["recordType"], &kind); err != nil {
			t.Fatal(err)
		}
		if kind == "invocation_result" {
			terminal++
			if index != len(lines)-1 {
				t.Fatal("frames after terminal result")
			}
		}
	}
	if terminal != 1 {
		t.Fatalf("terminal NDJSON records = %d", terminal)
	}
}

func TestSelectedDiagnosticOutputConflictsRejectBeforeProvider(t *testing.T) {
	t.Parallel()
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 1)}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	for _, flags := range [][]string{{"--quiet", "--json"}, {"--quiet", "--output", "primary"}} {
		run := startSelectedOutputInvocation(t, process, t.Context(), support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"), flags, "private-prompt-canary")
		var failure *runcli.InvocationError
		if err := selectedAwait(t, run.done); !errors.As(err, &failure) || failure.Code != runcli.InvocationOutputConflictCode {
			t.Fatalf("output conflict = %v", err)
		}
		if run.stdout() != "" || !strings.Contains(run.inputs.Stderr(), runcli.InvocationOutputConflictCode) {
			t.Fatalf("conflict framing stdout=%q stderr=%q", run.stdout(), run.inputs.Stderr())
		}
		select {
		case <-runner.calls:
			t.Fatal("invalid output selection reached provider")
		default:
		}
	}
	t.Log("L05: quiet/JSON and quiet/explicit-output reject with typed validation and zero provider commands")
}

func TestSelectedRetryCancellationDiagnosticsKeepPeerIndependent(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	scheduler := &selectedOperationScheduler{Deterministic: platformclock.NewDeterministic(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC), time.Millisecond), timers: make(chan *selectedOperationTimer, 32), polls: make(chan *selectedOperationTimer, 256)}
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 16)}
	process := support.BuildProcess(t, serviceedges.Edges{ProcessLogger: zap.New(core).With(zap.String("backend", "selected-diagnostics")), ProcessScheduler: scheduler, ProviderCommandRunner: runner})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	run := startSelectedInvocation(t, process, ctx)
	selectedAwait(t, runner.calls).reply <- selectedRetryFailure()
	timer := selectedAwaitRetry(t, scheduler, runner, run)
	scheduler.SetTick(99)
	select {
	case <-runner.calls:
		t.Fatal("selected retry ran before its deadline")
	default:
	}
	peer := startSelectedInvocation(t, process, t.Context())
	peerCall := selectedAwait(t, runner.calls)
	if peerCall.request.ExecutionScopeID != peer.sessionID {
		t.Fatal("held retry crossed into healthy peer")
	}
	cancel()
	if err := selectedAwait(t, run.done); err == nil || !timer.stopped.Load() {
		t.Fatalf("retry cancel = %v; timer stopped=%v", err, timer.stopped.Load())
	}
	peerCall.reply <- platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("healthy-peer-result COMPLETE")}
	tick := 99
	if err := selectedAwaitCompletion(t, scheduler, peer, func(delay time.Duration) { tick += int(delay / time.Millisecond); scheduler.SetTick(tick) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(peer.stdout(), "healthy-peer-result") || strings.Contains(run.stdout(), "healthy-peer-result") {
		t.Fatal("peer output was lost or mixed")
	}
	for _, invocation := range []selectedInvocation{run, peer} {
		assertSelectedSessionLog(t, logs, invocation)
	}
	t.Log("L07: selected retry held/advanced/canceled with independent live peer output and selected-backend session diagnostics")
}

func TestSelectedFailedPrimaryOutputPreservesExistingFile(t *testing.T) {
	t.Parallel()
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 1)}
	core, logs := observer.New(zap.DebugLevel)
	process := support.BuildProcess(t, serviceedges.Edges{ProcessLogger: zap.New(core).With(zap.String("backend", "selected-diagnostics")), ProviderCommandRunner: runner})
	path := filepath.Join(t.TempDir(), "previous-result.txt")
	if err := os.WriteFile(path, []byte("previous result"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Retain an existing destination without shell truncation. The application
	// must publish no primary bytes when its invocation fails.
	file, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	run := startSelectedWriterInvocation(t, process, t.Context(), support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"), []string{"--output", "primary"}, "private-prompt-canary", file)
	selectedAwait(t, runner.calls).reply <- platformprocess.CommandResult{ExitCode: 1, Stdout: []byte("{\"type\":\"turn.failed\",\"error\":{\"message\":\"authentication failed: sk-fi-private-provider-canary\"}}\n"), Stderr: []byte("private-stderr-canary")}
	var failure interface{ InvocationErrorCode() string }
	if err := selectedAwait(t, run.done); !errors.As(err, &failure) || failure.InvocationErrorCode() != "INVOCATION_RUNTIME_FAILURE" {
		t.Fatalf("file-output failure = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "previous result" || run.stdout() != "" {
		t.Fatalf("failed file output = %q, %v; stdout=%q", content, err, run.stdout())
	}
	decoder := json.NewDecoder(strings.NewReader(run.inputs.Stderr()))
	var diagnostic factoryapi.ErrorResponse
	if err := decoder.Decode(&diagnostic); err != nil || diagnostic.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("failure stderr = %q, %v", run.inputs.Stderr(), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("failure diagnostics contaminated stderr: %v", err)
	}
	for _, private := range []string{"private-prompt-canary", "sk-fi-private-provider-canary", "private-stderr-canary", "turn.failed"} {
		if strings.Contains(run.inputs.Stderr(), private) {
			t.Fatalf("private content in file-output error: %s", private)
		}
	}
	assertSelectedSessionLog(t, logs, run)
	t.Log("L06: failed primary redirection preserves existing destination and emits one safe typed stderr error")
}
