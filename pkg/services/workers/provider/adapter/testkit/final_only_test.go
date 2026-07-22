package testkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/testkit"
)

const (
	finalOnlyPrompt = "private final-only prompt"
	finalOnlySecret = "sk-final-only-secret"
)

type finalOnlyParseError struct {
	providerSession *workerexecution.ProviderSessionMetadata
}

func (finalOnlyParseError) Error() string { return "fixture final response was unusable" }

func TestFinalOnlyAdapterConformance(t *testing.T) {
	session := workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: "session-final-9"}
	testkit.RunFinalOnly(t, testkit.FinalOnlyFixture{
		NewAdapter: func() adapter.Adapter { return finalOnlyAdapter{} },
		Request:    workerexecution.ProviderInferenceRequest{Model: "fixture-model", UserMessage: finalOnlyPrompt},
		Success:    workerprocess.CommandResult{Stdout: []byte(`{"content":"Complete response","session":"session-final-9"}`)},
		Failures: []testkit.FinalOnlyFailureCase{
			{
				Name:                    "empty final output is a normalized failure",
				Result:                  workerprocess.CommandResult{Stdout: []byte(`{"content":"","session":"session-final-9"}`)},
				ExpectedProviderSession: &session,
			},
			{
				Name:   "malformed final output is a normalized failure",
				Result: workerprocess.CommandResult{Stdout: []byte(`{"content":"` + finalOnlySecret)},
			},
		},
		Expected:            testkit.FinalOnlyExpected{Content: "Complete response", ProviderSession: &session},
		ForbiddenDiagnostic: []string{finalOnlyPrompt, finalOnlySecret},
	})
}

type finalOnlyAdapter struct{}

func (finalOnlyAdapter) Identity() adapter.Identity { return "fixture-final-only" }

func (finalOnlyAdapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	return adapter.CommandBuildResult{Request: workerprocess.CommandRequest{
		Command: "fixture-final-provider", Args: []string{"run", input.Request.Model},
		Stdin: []byte(input.Request.UserMessage), DispatchID: input.Request.Dispatch.DispatchID,
	}}, nil
}

func (finalOnlyAdapter) NewDecoder(context.Context, adapter.DecoderContext) (adapter.Decoder, error) {
	return finalOnlyDecoder{}, nil
}

func (finalOnlyAdapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	var native struct {
		Content string `json:"content"`
		Session string `json:"session"`
	}
	if err := json.Unmarshal(input.CommandResult.Stdout, &native); err != nil {
		return adapter.FinalParseResult{}, finalOnlyParseError{}
	}
	var session *workerexecution.ProviderSessionMetadata
	if strings.TrimSpace(native.Session) != "" {
		session = &workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: native.Session}
	}
	if strings.TrimSpace(native.Content) == "" {
		return adapter.FinalParseResult{}, finalOnlyParseError{providerSession: session}
	}
	return adapter.FinalParseResult{
		Response: workerexecution.InferenceResponse{Content: native.Content, ProviderSession: session},
		Drafts:   finalOnlyDrafts(input, native.Content, native.Session),
	}, nil
}

func (finalOnlyAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{MessageSnapshots: true, FinalOnly: true}}, nil
}

func (finalOnlyAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	var parseFailure finalOnlyParseError
	if !errors.As(input.ParseError, &parseFailure) {
		return adapter.FailureResult{}
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family:          workerexecution.WorkFailureFamilyTerminal,
		Type:            workerexecution.WorkFailureTypePermanentBadRequest,
		Message:         "fixture provider returned no usable final response",
		Retry:           adapter.RetryGuidance{Retryable: false},
		ProviderSession: parseFailure.providerSession,
	}}
}

type finalOnlyDecoder struct{}

func (finalOnlyDecoder) Observe(context.Context, adapter.Observation) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func (finalOnlyDecoder) Flush(context.Context, adapter.FlushContext) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func finalOnlyDrafts(input adapter.FinalParseContext, content, providerSession string) []factorysessions.ResponseEventDraft {
	correlation := func(draft factorysessions.ResponseEventDraft) factorysessions.ResponseEventDraft {
		draft.RunID = input.RunID
		draft.DispatchID = input.DispatchID
		return draft
	}
	return []factorysessions.ResponseEventDraft{
		correlation(runLifecycleDraft(factorysessions.ResponseEventPhaseStarted, "started")),
		correlation(factorysessions.ResponseEventDraft{
			Kind: factorysessions.ResponseEventKindMessage, Phase: factorysessions.ResponseEventPhaseCompleted,
			ProviderSessionRef: providerSession,
			Provenance: factorysessions.ResponseEventProvenance{
				Provider: "fixture", NativeEventType: "final_response",
				Delivery: factorysessions.ResponseEventDeliveryNativeFinal, Representation: factorysessions.ResponseEventRepresentationSnapshot,
				Fidelity: factorysessions.ResponseEventFidelityFinalOnly,
			},
			Payload: marshalFinalOnlyPayload(factorysessions.ResponseEventMessage{
				Role: "assistant", ContentBlocks: []factorysessions.ResponseEventContentBlock{{Kind: factorysessions.ResponseEventContentBlockText, Text: content}},
			}),
		}),
		correlation(runLifecycleDraft(factorysessions.ResponseEventPhaseCompleted, "completed")),
	}
}

func runLifecycleDraft(phase factorysessions.ResponseEventPhase, status string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindRun, Phase: phase,
		Provenance: factorysessions.ResponseEventProvenance{
			Provider: "fixture", NativeEventType: "command_completion",
			Delivery: factorysessions.ResponseEventDeliverySynthesized, Representation: factorysessions.ResponseEventRepresentationNotification,
			Fidelity: factorysessions.ResponseEventFidelityLifecycleOnly,
		},
		Payload: marshalFinalOnlyPayload(factorysessions.ResponseEventRun{Status: status}),
	}
}

func marshalFinalOnlyPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

var _ adapter.Adapter = finalOnlyAdapter{}
var _ adapter.Decoder = finalOnlyDecoder{}
