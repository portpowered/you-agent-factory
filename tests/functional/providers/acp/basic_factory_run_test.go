package acp_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const acpHelperEnvironment = "YOU_TEST_ACP_AGENT_HELPER"

func TestFactoryRunRoutesExecutorProviderThroughACPAdapter(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP vertical slice"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var processStarts atomic.Int32
	fallback := &legacyProvider{err: errors.New("legacy provider route was unexpectedly invoked")}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; events=%#v", got, events)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want 1", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("legacy provider calls = %d, want 0; ACP must be selected by executorProvider", got)
	}
	assertACPProviderSession(t, events)
}

func TestFactoryRunRetainsLegacyNamedExecutorProviderCompatibility(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"legacy ACP spelling"}`))
	writeLegacyACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var processStarts atomic.Int32
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want 1", got)
	}
}

func TestFactoryRunProjectsOperatorConfiguredACPIntegrationIntoInvocationCatalog(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"configured ACP provider"}`))
	writeACPWorker(t, dir, "custom-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var processStarts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithConfiguredHome(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second, func(home string) {
		configDir := filepath.Join(home, ".you-agent-factory")
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			t.Fatalf("create operator config directory: %v", err)
		}
		config := []byte(`{"workers":{"acp":{"integrations":[{"id":"entry-1","name":"custom-acp","transport":"stdio","command":"custom-agent acp"}]}}}`)
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
			t.Fatalf("write operator config: %v", err)
		}
	})

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; events=%#v", got, events)
	}
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("configured ACP process starts = %d, want 1", got)
	}
	assertProviderSession(t, events, "custom-acp")
}

func assertACPProviderSession(t *testing.T, events []factoryapi.FactoryEvent) {
	assertProviderSession(t, events, "cursor-acp")
}

func assertProviderSession(t *testing.T, events []factoryapi.FactoryEvent, provider string) {
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
		if *payload.ProviderSession.Provider != provider || *payload.ProviderSession.Id != "acp-session-functional-1" {
			t.Fatalf("Provider Session = %#v, want %s/acp-session-functional-1", payload.ProviderSession, provider)
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
	fallback := &legacyProvider{response: workers.InferenceResponse{Content: "legacy COMPLETE"}}
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

func TestScriptWrapExecutorProviderRetainsLegacyProviderRoute(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"script wrap compatibility"}`))
	writeLegacyACPWorker(t, dir, "SCRIPT_WRAP")

	var processStarts atomic.Int32
	fallback := &legacyProvider{response: workers.InferenceResponse{Content: "legacy route COMPLETE"}}
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := fallback.calls.Load(); got != 1 {
		t.Fatalf("legacy provider calls = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts for SCRIPT_WRAP = %d, want 0", got)
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

type legacyProvider struct {
	calls    atomic.Int32
	response workers.InferenceResponse
	err      error
}

type availableExecutableLocator struct{}

func (availableExecutableLocator) LookPath(file string) (string, error) { return file, nil }

func (p *legacyProvider) Execute(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	p.calls.Add(1)
	return p.response, p.err
}

func TestACPAgentHelperProcess(t *testing.T) {
	mode := os.Getenv(acpHelperEnvironment)
	if mode != "1" && mode != "fail" && mode != "auth" && mode != "model" && mode != "resource" && mode != "content" && mode != "version" && mode != "init-fail" && mode != "stderr" && mode != "malformed" && mode != "eof" && mode != "block" && mode != "isolate" && mode != "unsupported" && mode != "persistent" && mode != "serialize" && mode != "crash-once" && mode != "spawn" && mode != "tournament" {
		return
	}
	if err := runFunctionalRPCPeer(mode, os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	os.Exit(0)
}
