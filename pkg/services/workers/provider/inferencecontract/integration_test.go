package inferencecontract_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type deterministicIntegration struct {
	identity contract.Identity
}

func (p deterministicIntegration) Identity() contract.Identity { return p.identity }

func (p deterministicIntegration) MaximumCapabilities() contract.CapabilitySet {
	return contract.NewCapabilitySet(
		contract.CapabilityPromptSubmission,
		contract.CapabilityMessageSnapshots,
	)
}

func (p deterministicIntegration) Discover(context.Context) (contract.Discovery, error) {
	return contract.NewDiscovery(
		contract.ReadinessReady,
		contract.NewPrerequisite(
			contract.PrerequisiteConfiguration,
			"provider configuration",
			contract.PrerequisiteSatisfied,
			"provider configuration is available",
		),
	), nil
}

func (p deterministicIntegration) Capabilities(
	_ context.Context,
	request contract.InvocationRequest,
) (contract.CapabilitySet, error) {
	if request.RequiredCapabilities().Has(contract.CapabilityMessageSnapshots) {
		return p.MaximumCapabilities(), nil
	}
	return contract.NewCapabilitySet(contract.CapabilityPromptSubmission), nil
}

func (p deterministicIntegration) Invoke(
	ctx context.Context,
	request contract.InvocationRequest,
	writer contract.ResponseWriter,
) error {
	payload, err := json.Marshal(workers.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workers.ContentBlock{{
			Kind: workers.ContentBlockText,
			Text: "response for " + request.UserMessage(),
		}},
	})
	if err != nil {
		return err
	}
	draft, err := contract.NewEventDraft(contract.EventDraftInput{
		RunID:   request.InvocationID(),
		Kind:    workers.KindMessage,
		Phase:   workers.PhaseCompleted,
		Payload: payload,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeFinal,
			Fidelity:        workers.FidelityFinalOnly,
			NativeEventType: "completion",
			Provider:        string(p.identity),
			Representation:  workers.RepresentationSnapshot,
		},
		ItemID: "message-1",
	})
	if err != nil {
		return err
	}
	if err := writer.WriteEvent(ctx, draft); err != nil {
		return err
	}
	sessionMetadata := map[string]string{"region": "test-region"}
	responseMetadata := map[string]string{"model": request.Model()}
	session := contract.NewProviderSession(string(p.identity), "session_id", "session-1", sessionMetadata)
	response := contract.NewResponse(contract.ResponseInput{
		Content:         "response for " + request.UserMessage(),
		ProviderSession: &session,
		Metadata:        responseMetadata,
	})
	sessionMetadata["region"] = "mutated-after-return"
	responseMetadata["model"] = "mutated-after-return"
	return writer.Close(ctx, contract.SuccessfulCompletion(response))
}

type recordingWriter struct {
	drafts     []contract.EventDraft
	completion *contract.Completion
	closes     int
}

func (w *recordingWriter) WriteEvent(_ context.Context, draft contract.EventDraft) error {
	w.drafts = append(w.drafts, draft)
	return nil
}

func (w *recordingWriter) Close(_ context.Context, completion contract.Completion) error {
	w.closes++
	w.completion = &completion
	return nil
}

func TestIntegrationContractIsIdentityOpaque(t *testing.T) {
	t.Parallel()

	for _, identity := range []contract.Identity{"acme.alpha", "customer-provider-42"} {
		identity := identity
		t.Run(string(identity), func(t *testing.T) {
			t.Parallel()
			assertSuccessfulIntegration(t, deterministicIntegration{identity: identity})
		})
	}
}

func assertSuccessfulIntegration(t *testing.T, integration contract.Integration) {
	t.Helper()
	ctx := context.Background()
	assertDiscovery(t, ctx, integration)
	request := newFixtureRequest()
	assertNegotiatedCapabilities(t, ctx, integration, request)

	writer := &recordingWriter{}
	if err := integration.Invoke(ctx, request, writer); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	assertDraft(t, writer)
	assertCompletion(t, integration.Identity(), writer)
}

func assertDiscovery(t *testing.T, ctx context.Context, integration contract.Integration) {
	t.Helper()
	discovery, err := integration.Discover(ctx)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if discovery.Readiness() != contract.ReadinessReady {
		t.Fatalf("readiness = %q, want %q", discovery.Readiness(), contract.ReadinessReady)
	}
	prerequisites := discovery.Prerequisites()
	if len(prerequisites) != 1 || prerequisites[0].Status() != contract.PrerequisiteSatisfied {
		t.Fatalf("prerequisites = %#v, want one satisfied prerequisite", prerequisites)
	}
}

func newFixtureRequest() contract.InvocationRequest {
	return contract.NewInvocationRequest(contract.InvocationInput{
		InvocationID: "invocation-1",
		Model:        "test-model",
		UserMessage:  "hello",
		Required:     contract.NewCapabilitySet(contract.CapabilityMessageSnapshots),
	})
}

func assertNegotiatedCapabilities(
	t *testing.T,
	ctx context.Context,
	integration contract.Integration,
	request contract.InvocationRequest,
) {
	t.Helper()
	capabilities, err := integration.Capabilities(ctx, request)
	if err != nil {
		t.Fatalf("negotiate capabilities: %v", err)
	}
	if !capabilities.Has(contract.CapabilityMessageSnapshots) {
		t.Fatalf("capabilities = %v, want message snapshots", capabilities.Values())
	}
}

func assertDraft(t *testing.T, writer *recordingWriter) {
	t.Helper()
	if writer.closes != 1 || writer.completion == nil {
		t.Fatalf("close count = %d, want exactly one completion", writer.closes)
	}
	if len(writer.drafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(writer.drafts))
	}
	draft := writer.drafts[0].Draft()
	if draft.Kind != workers.KindMessage || draft.Phase != workers.PhaseCompleted {
		t.Fatalf("draft = %#v, want completed message", draft)
	}
	if draft.DispatchID != "" {
		t.Fatalf("dispatch ID = %q, want provider draft without Factory-owned identity", draft.DispatchID)
	}
}

func assertCompletion(t *testing.T, identity contract.Identity, writer *recordingWriter) {
	t.Helper()
	response := writer.completion.Response()
	if response == nil || writer.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want response only", writer.completion)
	}
	if response.Content() != "response for hello" {
		t.Fatalf("response content = %q", response.Content())
	}
	session := response.ProviderSession()
	if session == nil || session.Provider() != string(identity) {
		t.Fatalf("provider session = %#v, want provider %q", session, identity)
	}
	if got := session.Metadata()["region"]; got != "test-region" {
		t.Fatalf("session region = %q, want detached test-region", got)
	}
	if got := response.Metadata()["model"]; got != "test-model" {
		t.Fatalf("response model = %q, want detached test-model", got)
	}
}

func TestEventDraftRejectsFactoryOwnedStreamGap(t *testing.T) {
	t.Parallel()

	_, err := contract.NewEventDraft(contract.EventDraftInput{Kind: workers.KindStreamGap})
	if err == nil {
		t.Fatal("expected STREAM_GAP draft to be rejected")
	}
}

func TestImmutableContractValuesReturnDetachedCollections(t *testing.T) {
	t.Parallel()

	capabilities := contract.NewCapabilitySet(contract.CapabilityPromptSubmission)
	values := capabilities.Values()
	values[0] = contract.CapabilityUsage
	if !capabilities.Has(contract.CapabilityPromptSubmission) {
		t.Fatalf("capabilities changed through returned values: %v", capabilities.Values())
	}

	metadata := map[string]string{"scope": "original"}
	session := contract.NewProviderSession("opaque-provider", "session", "one", metadata)
	metadata["scope"] = "changed"
	returned := session.Metadata()
	returned["scope"] = "changed-again"
	if got := session.Metadata()["scope"]; got != "original" {
		t.Fatalf("session metadata = %q, want detached original", got)
	}
}
