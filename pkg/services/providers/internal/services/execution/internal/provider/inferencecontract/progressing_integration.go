package inferencecontract

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProgressingIntegration is a deterministic fake Integration that publishes
// structured progress through the response writer and closes with one success.
// Construction of Workers-owned Draft/Response values stays inside this package
// so functional scenarios can register the integration without calling
// inferencecontract constructors from outside Workers.
type ProgressingIntegration struct {
	mu                       sync.Mutex
	identity                 Identity
	content                  string
	discoverCalls            int
	capabilityCalls          int
	invokeCalls              int
	progressWrites           int
	terminalCloses           int
	discoverBeforeInvoke     int
	capabilitiesBeforeInvoke int
	invokeStarted            bool
}

// ProgressingIntegrationStats is a snapshot of ProgressingIntegration counters.
type ProgressingIntegrationStats struct {
	DiscoverCalls            int
	CapabilityCalls          int
	InvokeCalls              int
	ProgressWrites           int
	TerminalCloses           int
	DiscoverBeforeInvoke     int
	CapabilitiesBeforeInvoke int
}

// ProgressingExternalIntegration returns a fake Integration that completes one
// invocation with ordered progress events and a single successful terminal.
func ProgressingExternalIntegration(identity Identity, content string) *ProgressingIntegration {
	return &ProgressingIntegration{identity: identity, content: content}
}

// Stats returns a race-safe snapshot of observed Integration I/O.
func (i *ProgressingIntegration) Stats() ProgressingIntegrationStats {
	i.mu.Lock()
	defer i.mu.Unlock()
	return ProgressingIntegrationStats{
		DiscoverCalls:            i.discoverCalls,
		CapabilityCalls:          i.capabilityCalls,
		InvokeCalls:              i.invokeCalls,
		ProgressWrites:           i.progressWrites,
		TerminalCloses:           i.terminalCloses,
		DiscoverBeforeInvoke:     i.discoverBeforeInvoke,
		CapabilitiesBeforeInvoke: i.capabilitiesBeforeInvoke,
	}
}

// Identity returns the authored external identity.
func (i *ProgressingIntegration) Identity() Identity { return i.identity }

// MaximumCapabilities returns the prompt-submission capability ceiling.
func (*ProgressingIntegration) MaximumCapabilities() CapabilitySet {
	return NewCapabilitySet(CapabilityPromptSubmission)
}

// Discover reports readiness without depending on request-sensitive state.
func (i *ProgressingIntegration) Discover(context.Context) (Discovery, error) {
	i.mu.Lock()
	i.discoverCalls++
	if !i.invokeStarted {
		i.discoverBeforeInvoke++
	}
	i.mu.Unlock()
	return NewDiscovery(ReadinessReady), nil
}

// Capabilities returns the maximum capability set for the request.
func (i *ProgressingIntegration) Capabilities(
	context.Context,
	InvocationRequest,
) (CapabilitySet, error) {
	i.mu.Lock()
	i.capabilityCalls++
	if !i.invokeStarted {
		i.capabilitiesBeforeInvoke++
	}
	i.mu.Unlock()
	return i.MaximumCapabilities(), nil
}

// Invoke publishes ordered progress drafts and closes with one success.
func (i *ProgressingIntegration) Invoke(
	ctx context.Context,
	request InvocationRequest,
	writer ResponseWriter,
) error {
	i.mu.Lock()
	i.invokeStarted = true
	i.invokeCalls++
	i.mu.Unlock()

	events, err := progressingEvents(request.InvocationID(), string(i.identity), i.content)
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
	if err := writer.Close(ctx, SuccessfulCompletion(NewResponse(ResponseInput{
		Content: i.content,
	}))); err != nil {
		return err
	}
	i.mu.Lock()
	i.terminalCloses++
	i.mu.Unlock()
	return nil
}

func progressingEvents(runID, provider, content string) ([]EventDraft, error) {
	started, err := progressingRunEvent(runID, provider, workers.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := progressingMessageEvent(runID, provider, "message-1", content)
	if err != nil {
		return nil, err
	}
	completed, err := progressingRunEvent(runID, provider, workers.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []EventDraft{started, message, completed}, nil
}

func progressingRunEvent(runID, provider string, phase workers.Phase) (EventDraft, error) {
	payload, err := json.Marshal(workers.RunPayload{Status: string(phase)})
	if err != nil {
		return EventDraft{}, fmt.Errorf("marshal run payload: %w", err)
	}
	return NewEventDraft(EventDraftInput{
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

func progressingMessageEvent(runID, provider, itemID, content string) (EventDraft, error) {
	payload, err := json.Marshal(workers.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workers.ContentBlock{{
			Kind: workers.ContentBlockText,
			Text: content,
		}},
	})
	if err != nil {
		return EventDraft{}, fmt.Errorf("marshal message payload: %w", err)
	}
	return NewEventDraft(EventDraftInput{
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
