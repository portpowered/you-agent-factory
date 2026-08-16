package run

import (
	"context"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestOpenHostedRuntimeUsesOpenedHostedInvocationCapability(t *testing.T) {
	sessions := &hostedInvocationCapabilityFake{}
	request := invocationRequestFromText("summarize the dispatch")

	operation, err := openHostedRuntime(
		t.Context(),
		RunConfig{Output: io.Discard, WithServer: true},
		zap.NewNop(),
		request,
		resolvedRunRecordPath{},
		testInvocationOperation{},
		nil,
		nil,
		nil,
		true,
		0,
		func(
			_ context.Context,
			_ *factorysessions.RuntimeOpeningRequest,
			_ factorysessions.VisualizationSinkID,
		) (initializer.LocalRuntimeRunner, error) {
			return WithHostedInvocation(hostedInvocationCompletionRunner{}, sessions), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.RuntimeOpeningRequest {
			return &factorysessions.RuntimeOpeningRequest{}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("openHostedRuntime() error = %v", err)
	}
	if err := operation.Run(t.Context()); err != nil {
		t.Fatalf("Operation.Run() error = %v", err)
	}
	if !sessions.invoked {
		t.Fatal("hosted Factory Sessions invocation was not used")
	}
}

type hostedInvocationCompletionRunner struct{}

func (hostedInvocationCompletionRunner) Run(context.Context) error { return nil }

func (hostedInvocationCompletionRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	return completion(ctx)
}

type hostedInvocationCapabilityFake struct {
	invoked bool
}

func (fake *hostedInvocationCapabilityFake) InvokeFactorySession(
	context.Context,
	string,
	factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	fake.invoked = true
	return factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "completed",
		}},
	}, nil
}

func (*hostedInvocationCapabilityFake) SubscribeFactoryEventsForSession(
	context.Context,
	string,
	*interfaces.FactoryEventReconnectCursor,
) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (*hostedInvocationCapabilityFake) ReadDurableFactorySessionEventStream(
	context.Context,
	string,
	factorysessions.EventReconnectRequest,
) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

var _ HostedInvocationOperation = (*hostedInvocationCapabilityFake)(nil)
