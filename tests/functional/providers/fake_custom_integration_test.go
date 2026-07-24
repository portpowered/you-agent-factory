package providers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFakeCustomIntegrationCompletesFactoryDispatchThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExternalProviderWorker(t, dir, "customer.provider")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"fake custom integration"}`))

	integration := &fakeCustomIntegration{identity: "customer.provider"}
	manifest := externalProviderManifest(t, "customer.provider", "customer")

	session, _, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	}, 20*time.Second)

	if got := support.SessionPlaceTokenCount(session, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.SessionPlaceTokenCount(session, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	stats := integration.stats()
	if stats.invokeCalls != 1 {
		t.Fatalf("fake integration invoke calls = %d, want 1", stats.invokeCalls)
	}
	if stats.progressWrites < 1 {
		t.Fatalf("structured progress writes = %d, want at least 1 through the conductor response writer", stats.progressWrites)
	}
	if stats.terminalCloses != 1 {
		t.Fatalf("terminal closes = %d, want exactly one terminal outcome", stats.terminalCloses)
	}
	if stats.discoverBeforeInvoke != 0 || stats.capabilitiesBeforeInvoke != 0 {
		t.Fatalf(
			"provider I/O before invoke = discover:%d capabilities:%d, want zero until dispatch",
			stats.discoverBeforeInvoke,
			stats.capabilitiesBeforeInvoke,
		)
	}
}

func TestFakeCustomIntegrationRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	integration := &fakeCustomIntegration{identity: "customer.provider"}
	manifest := externalProviderManifest(t, "customer.provider", "customer")
	_ = support.BuildProcess(t, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	})

	stats := integration.stats()
	if stats.invokeCalls != 0 || stats.progressWrites != 0 || stats.terminalCloses != 0 ||
		stats.discoverCalls != 0 || stats.capabilityCalls != 0 {
		t.Fatalf("construction side effects = %#v, want inert registry composition", stats)
	}
}

type fakeCustomIntegration struct {
	mu                       sync.Mutex
	identity                 inference.Identity
	discoverCalls            int
	capabilityCalls          int
	invokeCalls              int
	progressWrites           int
	terminalCloses           int
	discoverBeforeInvoke     int
	capabilitiesBeforeInvoke int
	invokeStarted            bool
}

type fakeCustomIntegrationStats struct {
	discoverCalls            int
	capabilityCalls          int
	invokeCalls              int
	progressWrites           int
	terminalCloses           int
	discoverBeforeInvoke     int
	capabilitiesBeforeInvoke int
}

func (i *fakeCustomIntegration) stats() fakeCustomIntegrationStats {
	i.mu.Lock()
	defer i.mu.Unlock()
	return fakeCustomIntegrationStats{
		discoverCalls:            i.discoverCalls,
		capabilityCalls:          i.capabilityCalls,
		invokeCalls:              i.invokeCalls,
		progressWrites:           i.progressWrites,
		terminalCloses:           i.terminalCloses,
		discoverBeforeInvoke:     i.discoverBeforeInvoke,
		capabilitiesBeforeInvoke: i.capabilitiesBeforeInvoke,
	}
}

func (i *fakeCustomIntegration) Identity() inference.Identity { return i.identity }

func (*fakeCustomIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
}

func (i *fakeCustomIntegration) Discover(context.Context) (inference.Discovery, error) {
	i.mu.Lock()
	i.discoverCalls++
	if !i.invokeStarted {
		i.discoverBeforeInvoke++
	}
	i.mu.Unlock()
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *fakeCustomIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	i.mu.Lock()
	i.capabilityCalls++
	if !i.invokeStarted {
		i.capabilitiesBeforeInvoke++
	}
	i.mu.Unlock()
	return i.MaximumCapabilities(), nil
}

func (i *fakeCustomIntegration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	i.mu.Lock()
	i.invokeStarted = true
	i.invokeCalls++
	i.mu.Unlock()

	events, err := fakeProgressEvents(request.InvocationID(), string(i.identity), "structured progress COMPLETE")
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := writer.WriteEvent(ctx, event); err != nil {
			return err
		}
		i.mu.Lock()
		i.progressWrites++
		i.mu.Unlock()
	}
	if err := writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: "structured progress COMPLETE",
	}))); err != nil {
		return err
	}
	i.mu.Lock()
	i.terminalCloses++
	i.mu.Unlock()
	return nil
}

func writeExternalProviderWorker(t *testing.T, factoryDir, provider string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: test-model",
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write external provider worker: %v", err)
	}
}

func externalProviderManifest(t *testing.T, identity, alias string) inference.Manifest {
	t.Helper()
	var catalog struct {
		Providers []inference.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = inference.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = inference.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = inference.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = inference.ResponseFidelityCapabilities{}
	return manifest
}

func fakeProgressEvents(runID, provider, content string) ([]inference.EventDraft, error) {
	started, err := fakeRunEvent(runID, provider, workers.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := fakeMessageEvent(runID, provider, "message-1", content)
	if err != nil {
		return nil, err
	}
	completed, err := fakeRunEvent(runID, provider, workers.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []inference.EventDraft{started, message, completed}, nil
}

func fakeRunEvent(runID, provider string, phase workers.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workers.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal run payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workers.KindRun,
		Phase:   phase,
		Payload: payload,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeStream,
			Fidelity:        workers.FidelityNormalized,
			NativeEventType: "fake.custom.run",
			Provider:        provider,
			Representation:  workers.RepresentationSnapshot,
		},
	})
}

func fakeMessageEvent(runID, provider, itemID, content string) (inference.EventDraft, error) {
	payload, err := json.Marshal(workers.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workers.ContentBlock{{
			Kind: workers.ContentBlockText,
			Text: content,
		}},
	})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal message payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workers.KindMessage,
		Phase:   workers.PhaseCompleted,
		ItemID:  itemID,
		Payload: payload,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeFinal,
			Fidelity:        workers.FidelityFinalOnly,
			NativeEventType: "fake.custom.message",
			Provider:        provider,
			Representation:  workers.RepresentationSnapshot,
		},
	})
}
