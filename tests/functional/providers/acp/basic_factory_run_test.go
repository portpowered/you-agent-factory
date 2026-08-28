package acp_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	acpHelperEnvironment                    = "YOU_TEST_ACP_AGENT_HELPER"
	acpRetryAttemptDirectoryEnvironment     = "YOU_TEST_ACP_RETRY_ATTEMPT_DIR"
	acpRetryHoldEnvironment                 = "YOU_TEST_ACP_RETRY_HOLD"
	acpDisconnectMarkerEnvironment          = "YOU_TEST_ACP_DISCONNECT_MARKER"
	acpDisconnectReadyEnvironment           = "YOU_TEST_ACP_DISCONNECT_READY"
	acpDisconnectReleaseEnvironment         = "YOU_TEST_ACP_DISCONNECT_RELEASE"
	acpPackageConformanceReleaseEnvironment = "YOU_TEST_ACP_PACKAGE_CONFORMANCE_RELEASE"
)

// TestFactoryRunRetriesACPProviderByResumingExactSession exercises the public
// Factory execution path through an ACP server error. The helper accepts a
// fresh session only before it writes the failure marker, then accepts only
// session/load with the original opaque id. A passing run therefore proves the
// worker retry kept its Provider Session and called Providers.Continue rather
// than silently opening a new ACP session. The fixture uses an operator-
// configured ACP integration because packaged ACP profiles may truthfully omit
// session resume; packaged behavior is covered by the package conformance
// matrix and capability tests.
func TestFactoryRunRetriesACPProviderByResumingExactSession(t *testing.T) {
	const sessionID = "acp-session-retry-resume"
	const providerID = "retry-acp"
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"retry ACP through its prior session"}`))
	writeACPWorker(t, dir, providerID)
	t.Setenv(acpHelperEnvironment, "retry-resume")
	retryAttemptDir := t.TempDir()
	retryHoldMarker := filepath.Join(retryAttemptDir, "first-prompt-held")
	t.Setenv(acpRetryHoldEnvironment, retryHoldMarker)
	t.Setenv(acpRetryAttemptDirectoryEnvironment, retryAttemptDir)
	t.Setenv("YOU_TEST_ACP_SESSION_ID", sessionID)

	var processStarts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithConfiguredHome(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: retryACPCommandFactory(&processStarts, retryAttemptDir),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second, func(home string) {
		configDir := filepath.Join(home, ".you-agent-factory")
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			t.Fatalf("create operator config directory: %v", err)
		}
		config := []byte(`{"workers":{"acp":{"integrations":[{"id":"retry-entry","name":"retry-acp","transport":"stdio","command":"custom-agent acp"}]}}}`)
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
			t.Fatalf("write operator config: %v", err)
		}
	})

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; %s", got, acpFailureDiagnostics(events))
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0; %s", got, acpFailureDiagnostics(events))
	}
	if got := processStarts.Load(); got != 2 {
		t.Fatalf("ACP process starts = %d, want 2 for the failed attempt and resumed retry", got)
	}
	if _, err := os.Stat(retryHoldMarker); err != nil {
		t.Fatalf("first ACP retry peer did not reach its controlled live-process checkpoint: %v", err)
	}
	assertProviderSessionID(t, events, providerID, sessionID)
}

func assertACPProviderSession(t *testing.T, events []factoryapi.FactoryEvent) {
	assertProviderSession(t, events, "cursor-acp")
}

func assertProviderSession(t *testing.T, events []factoryapi.FactoryEvent, provider string) {
	assertProviderSessionID(t, events, provider, "acp-session-functional-1")
}

func assertProviderSessionID(t *testing.T, events []factoryapi.FactoryEvent, provider, sessionID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil || payload.ProviderSession.Id == nil {
			continue
		}
		if *payload.ProviderSession.Provider != provider || *payload.ProviderSession.Id != sessionID {
			t.Fatalf("Provider Session = %#v, want %s/%s", payload.ProviderSession, provider, sessionID)
		}
		return
	}
	t.Fatal("Factory events omitted the ACP Provider Session reference")
}

func TestRootConstructionDoesNotStartACPProcess(t *testing.T) {
	var processStarts atomic.Int32
	_ = support.BuildProcess(t, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
	})
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts during root construction = %d, want 0", got)
	}
}

func TestUnknownExecutorProviderFailsBeforeACPProcessStart(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"unknown ACP provider"}`))
	writeLegacyACPWorker(t, dir, "missing-acp")

	var processStarts atomic.Int32
	fallback := &legacyProvider{response: providers.ExecuteResult{Content: "legacy COMPLETE"}}
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts for unknown provider = %d, want 0", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("legacy provider calls for unknown ACP provider = %d, want 0", got)
	}
}

func writeACPWorker(t *testing.T, factoryDir, providerID string) {
	t.Helper()
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: ACP\n" +
		"modelProvider: " + providerID + "\n" +
		"model: test-model\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nTest ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ACP worker: %v", err)
	}
}

func writeLegacyACPWorker(t *testing.T, factoryDir, providerID string) {
	t.Helper()
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: " + providerID + "\n" +
		"model: test-model\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nLegacy ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write legacy ACP worker: %v", err)
	}
}

func acpHelperCommandFactory(starts *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if (name == "cursor-agent" || name == "custom-agent") && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPAgentHelperProcess$")
		}
		return exec.Command(name, args...)
	}
}

func retryACPCommandFactory(starts *atomic.Int32, attemptDir string) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if name != "custom-agent" || !sameStringSlice(args, []string{"acp"}) {
			return exec.Command(name, args...)
		}
		attempt := starts.Add(1)
		// The Providers ACP service replaces the command's environment with the
		// invocation environment before Start, so the attempt phase is carried
		// through a pre-start filesystem edge instead of cmd.Env. The helper reads
		// the highest phase file after the process has started; this makes the
		// first failure and resumed second process deterministic without a prompt
		// marker race.
		_ = os.WriteFile(filepath.Join(attemptDir, strconv.Itoa(int(attempt))), []byte("started"), 0o600)
		return exec.Command(os.Args[0], "-test.run=^TestACPAgentHelperProcess$")
	}
}

func acpFailureDiagnostics(events []factoryapi.FactoryEvent) string {
	var failures []string
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			failures = append(failures, fmt.Sprintf("model response decode failed: %v", err))
			continue
		}
		if payload.FailureDetail == nil {
			continue
		}
		providerSession := "<none>"
		if payload.ProviderSession != nil {
			provider := "<unknown>"
			if payload.ProviderSession.Provider != nil {
				provider = *payload.ProviderSession.Provider
			}
			sessionID := "<unknown>"
			if payload.ProviderSession.Id != nil {
				sessionID = *payload.ProviderSession.Id
			}
			providerSession = provider + "/" + sessionID
		}
		failures = append(failures, fmt.Sprintf(
			"event=%s attempt=%d session=%s reason=%s message=%q",
			event.Id,
			payload.Attempt,
			providerSession,
			payload.FailureDetail.Reason,
			payload.FailureDetail.Message,
		))
	}
	if len(failures) == 0 {
		return fmt.Sprintf("ACP model-response failure detail unavailable (event count=%d)", len(events))
	}
	return "ACP model-response failures: " + strings.Join(failures, "; ")
}

type legacyProvider struct {
	testutil.NativeProvider
	calls    atomic.Int32
	response providers.ExecuteResult
	err      error
}

type availableExecutableLocator struct{}

func (availableExecutableLocator) LookPath(file string) (string, error) { return file, nil }

func (p *legacyProvider) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	p.calls.Add(1)
	response := p.response.Clone()
	if response.Content != "" && response.Diagnostics == nil {
		response.Diagnostics = &providers.ExecuteDiagnostics{Metadata: map[string]string{
			"completion_evidence": "provider_response",
		}}
	}
	return response, p.err
}

func (p *legacyProvider) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	result, err := p.Execute(ctx, request.Attempt)
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    result,
	}, nil
}

func TestACPAgentHelperProcess(t *testing.T) {
	mode := os.Getenv(acpHelperEnvironment)
	if mode != "1" && mode != "fail" && mode != "auth" && mode != "model" && mode != "package-conformance" && mode != "resource" && mode != "content" && mode != "version" && mode != "init-fail" && mode != "stderr" && mode != "malformed" && mode != "eof" && mode != "block" && mode != "isolate" && mode != "unsupported" && mode != "persistent" && mode != "serialize" && mode != "crash-once" && mode != "spawn" && mode != "tournament" && mode != "cancelled-response" && mode != "resume" && mode != "resume-not-found" && mode != "retry-resume" && mode != "disconnect-once" && mode != "shared-spine" {
		return
	}
	if err := runFunctionalRPCPeer(mode, os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	os.Exit(0)
}
