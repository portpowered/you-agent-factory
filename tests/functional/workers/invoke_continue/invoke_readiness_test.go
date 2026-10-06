package acceptance

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

type t7ReadinessProvider struct {
	unavailable atomic.Bool
	integration *providerswire.ProgressingIntegration
}

// Each mode owns a distinct external registration. The injected probe reads
// an atomic readiness fact and never discovers or executes an integration.
type t7ReadinessBoundary struct {
	providers     map[providers.ID]*t7ReadinessProvider
	registrations providerswire.ProviderRegistrations
}

func newT7ReadinessBoundary() *t7ReadinessBoundary {
	b := &t7ReadinessBoundary{providers: make(map[providers.ID]*t7ReadinessProvider)}
	for _, mode := range []string{"local", "remote", "http"} {
		id := "t7-ready-" + mode
		integration := providerswire.ProgressingExternalIntegration(providerswire.Identity(id), "T7 recovered provider COMPLETE")
		b.providers[providers.ID(id)] = &t7ReadinessProvider{integration: integration}
		b.registrations = append(b.registrations, providerswire.Registration{
			Manifest: providerswire.Manifest{ID: id,
				ImplementationAvailability:   providerswire.ImplementationExternallySupplied,
				TechnicalSupportLevel:        providerswire.SupportProduction,
				MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{PromptSubmission: true},
			}, Integration: integration,
		})
	}
	return b
}

func (b *t7ReadinessBoundary) probe(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	if p := b.providers[descriptor.ID]; p != nil && p.unavailable.Load() {
		descriptor.Readiness = providers.ReadinessUnavailable
	}
	return descriptor, ctx.Err()
}

// F6-06/08 prove recovery without a reserved identity, then accepted replay
// before changed readiness, through the real Providers/Workers/Start owners.
func TestT7ReadinessRejectionRecoveryAndAcceptedReplay(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/startWorkerSession", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"local", "remote", "http"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			t7ReadinessJourney(t, fixture, mode)
		})
	}
}

func t7ReadinessJourney(t *testing.T, fixture *invokeContinuePackageFixture, mode string) {
	t.Helper()
	scenario := fixture.scenario(t, "t7-ready-"+mode)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := fixture.readiness.providers[providers.ID(scenario.name)]
	before := provider.integration.Stats()
	provider.unavailable.Store(true)
	defer provider.unavailable.Store(false)
	id := scenarioScopedID(scenario, "readiness-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "readiness controlled input",
	})
	execution := document["execution"].(map[string]any)
	execution["dispatch"].(map[string]any)["execution"] = map[string]any{"requestId": id + "-request"}
	for _, key := range []string{"runnerId", "executorProvider", "modelProvider"} {
		execution[key] = scenario.name
	}
	path := filepath.Join(scenario.workingDirectory, "readiness.json")
	writeInvokeContinueJSON(t, path, document)
	t7AssertUnavailableInvoke(t, ctx, fixture, scenario, mode, path, document)
	for _, suffix := range []string{"", "/logs"} {
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+suffix, nil)
		if status != http.StatusNotFound {
			t.Fatalf("unavailable request opened capture: %d %s", status, body)
		}
	}
	if provider.integration.Stats().InvokeCalls != before.InvokeCalls || scenario.providerRunner.CallCount() != 0 {
		t.Fatal("preflight executed an external provider")
	}
	provider.unavailable.Store(false)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err != nil {
		t.Fatalf("readiness recovery = %v: %s", err, join.Stderr())
	}
	if !strings.Contains(join.Stdout(), id) || !strings.Contains(join.Stdout(), `"state":"COMPLETED"`) || !strings.Contains(join.Stdout(), "T7 recovered provider COMPLETE") {
		t.Fatalf("recovered identity/output = %s", join.Stdout())
	}
	provider.unavailable.Store(true)
	t7AssertHTTPReplay(t, ctx, fixture.baseURL, id, document)
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, "T7 recovered provider COMPLETE") || !strings.Contains(body, `"health":"COMPLETE"`) {
		t.Fatalf("replayed capture = %d %s", status, body)
	}
	document["requestId"], document["workerSessionId"] = id+"-new-request", id+"-new-session"
	status, body = t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
	if status != http.StatusServiceUnavailable || provider.integration.Stats().InvokeCalls != before.InvokeCalls+1 {
		t.Fatalf("new admission bypassed changed readiness or replay reexecuted: %d %s", status, body)
	}
}

func t7AssertUnavailableInvoke(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, mode, path string, document map[string]any) {
	t.Helper()
	if mode == "http" {
		status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
		if status != http.StatusServiceUnavailable || !strings.Contains(body, "WORKER_SESSION_ADMISSION_FAILED") {
			t.Fatalf("unavailable Start = %d %s", status, body)
		}
	} else {
		args := []string{"you", "--json"}
		if mode == "remote" {
			args = append(args, "--remote", "--server", fixture.baseURL)
		}
		args = append(args, "worker-sessions", "invoke", "--execution", path)
		inputs := support.FakeInputs(ctx, args)
		inputs.Input.Env, inputs.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
		if err := fixture.process.Execute(inputs.Input); err == nil || inputs.Stdout() != "" {
			t.Fatalf("unavailable invoke = %v stdout=%s", err, inputs.Stdout())
		}
		assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_ADMISSION_FAILED")
	}
}
