package testkit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract/testkit"
)

func TestSuccessConformance(t *testing.T) {
	expected := contract.NewResponse(contract.ResponseInput{
		Content:         "fixture response",
		ProviderSession: providerSession(),
		Metadata:        map[string]string{"model": "fixture-model"},
	})
	testkit.Run(t, testkit.Suite{
		Identities: []contract.Identity{"customer.alpha", "customer-beta"},
		FinalOnly:  factory(modeFinalOnly, expected),
		Streaming:  factory(modeStreaming, expected),
		Tool:       factory(modeTool, expected),
		Fixture: testkit.Fixture{
			FinalOnlyRequest: request("invocation-final", contract.CapabilityPromptSubmission),
			StreamingRequest: request("invocation-stream", contract.CapabilityPromptSubmission,
				contract.CapabilityNativeStreaming, contract.CapabilityMessageDeltas, contract.CapabilityMessageSnapshots),
			ToolRequest: request("invocation-tool", contract.CapabilityPromptSubmission,
				contract.CapabilityNativeStreaming, contract.CapabilityMessageSnapshots, contract.CapabilityToolLifecycle,
				contract.CapabilityToolOutputDeltas, contract.CapabilityStableItemIDs),
			ExpectedResponse: expected,
		},
	})
}

type fakeMode int

const (
	modeFinalOnly fakeMode = iota
	modeStreaming
	modeTool
)

type fakeIntegration struct {
	identity contract.Identity
	mode     fakeMode
	response contract.Response
}

func factory(mode fakeMode, response contract.Response) testkit.IntegrationFactory {
	return func(identity contract.Identity) contract.Integration {
		return &fakeIntegration{identity: identity, mode: mode, response: response}
	}
}

func (f *fakeIntegration) Identity() contract.Identity { return f.identity }

func (f *fakeIntegration) MaximumCapabilities() contract.CapabilitySet {
	capabilities := []contract.Capability{contract.CapabilityPromptSubmission}
	if f.mode >= modeStreaming {
		capabilities = append(capabilities, contract.CapabilityNativeStreaming, contract.CapabilityMessageDeltas,
			contract.CapabilityMessageSnapshots, contract.CapabilityStableItemIDs)
	}
	if f.mode == modeTool {
		capabilities = append(capabilities, contract.CapabilityToolLifecycle, contract.CapabilityToolOutputDeltas)
	}
	return contract.NewCapabilitySet(capabilities...)
}

func (f *fakeIntegration) Discover(context.Context) (contract.Discovery, error) {
	return contract.NewDiscovery(contract.ReadinessReady,
		contract.NewPrerequisite(contract.PrerequisiteDependency, "fixture runtime", contract.PrerequisiteSatisfied, "Fixture runtime is available.")), nil
}

func (f *fakeIntegration) Capabilities(_ context.Context, request contract.InvocationRequest) (contract.CapabilitySet, error) {
	return request.RequiredCapabilities(), nil
}

func (f *fakeIntegration) Invoke(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
	switch f.mode {
	case modeFinalOnly:
		return writer.Close(ctx, contract.SuccessfulCompletion(f.response))
	case modeStreaming:
		if err := f.write(ctx, request, writer, streamingDrafts); err != nil {
			return err
		}
	case modeTool:
		if err := f.write(ctx, request, writer, toolDrafts); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported fake mode %d", f.mode)
	}
	return writer.Close(ctx, contract.SuccessfulCompletion(f.response))
}

type draftSpec struct {
	kind    workers.Kind
	phase   workers.Phase
	itemID  string
	payload any
}

var streamingDrafts = []draftSpec{
	{workers.KindRun, workers.PhaseStarted, "", workers.RunPayload{Status: "running"}},
	{workers.KindMessage, workers.PhaseStarted, "message-1", workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText}}}},
	{workers.KindMessage, workers.PhaseDelta, "message-1", workers.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: "fixture "}},
	{workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload()},
	{workers.KindRun, workers.PhaseCompleted, "", workers.RunPayload{Status: "completed"}},
}

var toolDrafts = []draftSpec{
	{workers.KindRun, workers.PhaseStarted, "", workers.RunPayload{Status: "running"}},
	{workers.KindTool, workers.PhaseStarted, "tool-item-1", workers.ToolPayload{ToolCallID: "tool-call-1", ToolName: "fixture_lookup", Status: "running"}},
	{workers.KindTool, workers.PhaseDelta, "tool-item-1", workers.ToolDeltaPayload{ToolCallID: "tool-call-1", OutputDelta: "bounded output"}},
	{workers.KindTool, workers.PhaseCompleted, "tool-item-1", workers.ToolPayload{ToolCallID: "tool-call-1", ToolName: "fixture_lookup", Status: "completed"}},
	{workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload()},
	{workers.KindRun, workers.PhaseCompleted, "", workers.RunPayload{Status: "completed"}},
}

func (f *fakeIntegration) write(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter, specs []draftSpec) error {
	for _, spec := range specs {
		payload, err := json.Marshal(spec.payload)
		if err != nil {
			return err
		}
		representation := workers.RepresentationSnapshot
		if spec.phase == workers.PhaseDelta {
			representation = workers.RepresentationDelta
		}
		event, err := contract.NewEventDraft(contract.EventDraftInput{
			RunID: request.InvocationID(), Kind: spec.kind, Phase: spec.phase, ItemID: spec.itemID, Payload: payload,
			Provenance: workers.Provenance{Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
				NativeEventType: "fixture", Provider: string(f.identity), Representation: representation},
		})
		if err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func request(invocationID string, required ...contract.Capability) contract.InvocationRequest {
	return contract.NewInvocationRequest(contract.InvocationInput{
		InvocationID: invocationID, Model: "fixture-model", UserMessage: "deterministic fixture prompt",
		Required: contract.NewCapabilitySet(required...),
	})
}

func providerSession() *contract.ProviderSession {
	session := contract.NewProviderSession("fixture", "conversation", "session-1", map[string]string{"region": "test"})
	return &session
}

func messagePayload() workers.MessagePayload {
	return workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "fixture response"}}}
}
