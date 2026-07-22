package testkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract/testkit"
)

func TestAdverseConformance(t *testing.T) {
	response := contract.NewResponse(contract.ResponseInput{Content: "fixture response"})
	factory := func(behavior adverseBehavior) testkit.IntegrationFactory {
		return func(identity contract.Identity) contract.Integration {
			return &adverseIntegration{identity: identity, behavior: behavior, response: response}
		}
	}
	testkit.RunAdverse(t, testkit.AdverseSuite{
		Identities: []contract.Identity{"customer.alpha", "customer-beta"},
		Failures: func(identity contract.Identity, kind contract.FailureKind) contract.Integration {
			return &adverseIntegration{identity: identity, behavior: adverseFailure, failureKind: kind}
		},
		Cancellation:        factory(adverseInterrupted),
		Timeout:             factory(adverseInterrupted),
		Backpressure:        factory(adverseBackpressure),
		DoubleClose:         factory(adverseDoubleClose),
		WriteAfter:          factory(adverseWriteAfterClose),
		MissingClose:        factory(adverseMissingClose),
		Disagreement:        factory(adverseDisagreement),
		FailureAfterSuccess: factory(adverseFailureAfterSuccess),
		Request:             request("invocation-adverse", contract.CapabilityPromptSubmission),
	})
}

type adverseBehavior int

const (
	adverseFailure adverseBehavior = iota
	adverseInterrupted
	adverseBackpressure
	adverseDoubleClose
	adverseWriteAfterClose
	adverseMissingClose
	adverseDisagreement
	adverseFailureAfterSuccess
)

type adverseIntegration struct {
	identity    contract.Identity
	behavior    adverseBehavior
	failureKind contract.FailureKind
	response    contract.Response
}

func (f *adverseIntegration) Identity() contract.Identity { return f.identity }

func (f *adverseIntegration) MaximumCapabilities() contract.CapabilitySet {
	return contract.NewCapabilitySet(contract.CapabilityPromptSubmission)
}

func (f *adverseIntegration) Discover(context.Context) (contract.Discovery, error) {
	return contract.NewDiscovery(contract.ReadinessReady), nil
}

func (f *adverseIntegration) Capabilities(_ context.Context, request contract.InvocationRequest) (contract.CapabilitySet, error) {
	return request.RequiredCapabilities(), nil
}

func (f *adverseIntegration) Invoke(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
	switch f.behavior {
	case adverseFailure:
		return writer.Close(ctx, contract.FailedCompletion(contract.NewFailure(contract.FailureInput{
			Kind: f.failureKind, Message: "safe deterministic provider failure",
		})))
	case adverseInterrupted:
		<-ctx.Done()
		return ctx.Err()
	case adverseBackpressure:
		return writer.WriteEvent(ctx, adverseRunEvent(request.InvocationID(), f.identity, workers.PhaseStarted))
	case adverseDoubleClose:
		return closeConcurrently(ctx, writer, contract.SuccessfulCompletion(f.response))
	case adverseWriteAfterClose:
		if err := writer.Close(ctx, contract.SuccessfulCompletion(f.response)); err != nil {
			return err
		}
		return writer.WriteEvent(ctx, adverseRunEvent(request.InvocationID(), f.identity, workers.PhaseStarted))
	case adverseMissingClose:
		return nil
	case adverseDisagreement:
		if err := writer.WriteEvent(ctx, adverseMessageEvent(request.InvocationID(), f.identity)); err != nil {
			return err
		}
		return writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "contradictory response"})))
	case adverseFailureAfterSuccess:
		if err := writer.WriteEvent(ctx, adverseMessageEvent(request.InvocationID(), f.identity)); err != nil {
			return err
		}
		return writer.Close(ctx, contract.FailedCompletion(contract.NewFailure(contract.FailureInput{
			Kind: contract.FailureDependency, Message: "provider dependency failed",
		})))
	default:
		return errors.New("unsupported adverse behavior")
	}
}

func closeConcurrently(ctx context.Context, writer contract.ResponseWriter, completion contract.Completion) error {
	var wait sync.WaitGroup
	errorsByAttempt := make([]error, 2)
	wait.Add(len(errorsByAttempt))
	for index := range errorsByAttempt {
		index := index
		go func() {
			defer wait.Done()
			errorsByAttempt[index] = writer.Close(ctx, completion)
		}()
	}
	wait.Wait()
	for _, err := range errorsByAttempt {
		if err != nil {
			return err
		}
	}
	return errors.New("concurrent double close unexpectedly succeeded")
}

func adverseRunEvent(invocationID string, identity contract.Identity, phase workers.Phase) contract.EventDraft {
	event, err := contract.NewEventDraft(contract.EventDraftInput{
		RunID: invocationID, Kind: workers.KindRun, Phase: phase,
		Payload:    mustJSON(workers.RunPayload{Status: "running"}),
		Provenance: adverseProvenance(identity, workers.RepresentationSnapshot),
	})
	if err != nil {
		panic(err)
	}
	return event
}

func adverseMessageEvent(invocationID string, identity contract.Identity) contract.EventDraft {
	event, err := contract.NewEventDraft(contract.EventDraftInput{
		RunID: invocationID, Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ItemID: "message-adverse",
		Payload:    mustJSON(messagePayload()),
		Provenance: adverseProvenance(identity, workers.RepresentationSnapshot),
	})
	if err != nil {
		panic(err)
	}
	return event
}

func adverseProvenance(identity contract.Identity, representation workers.Representation) workers.Provenance {
	return workers.Provenance{
		Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
		NativeEventType: "fixture", Provider: string(identity), Representation: representation,
	}
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}
