package run

import (
	"context"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestOpenHostedRuntimeBindsFactorySessionsInvocationCapability(t *testing.T) {
	sessions := &hostedInvocationCapabilityFake{}
	request := invocationRequestFromText("summarize the dispatch")

	operation, err := openHostedRuntime(
		t.Context(),
		RunConfig{Output: io.Discard},
		zap.NewNop(),
		request,
		resolvedRunRecordPath{},
		testInvocationOperation{invokeFactory: func(
			_ context.Context,
			target factorysessions.InvocationTarget,
			_ factorysessions.InvocationRequest,
			_ factorysessions.FactoryEventConsumer,
		) (factorysessions.FactoryInvocationOutcome, error) {
			hosted := target.HostedLiveInvocation
			if hosted == nil {
				t.Fatal("hosted live invocation = nil")
			}
			if hosted.Sessions != sessions {
				t.Fatalf("hosted live sessions = %T, want Factory Sessions runtime service", hosted.Sessions)
			}
			invoker, ok := hosted.Invoker.(*hostedInvocationCapabilityFake)
			if !ok || invoker != sessions {
				t.Fatalf("hosted invoker = %T, want the bound Factory Sessions invocation capability", hosted.Invoker)
			}
			return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
				Status: interfaces.InvocationTerminalStatusCompleted,
				PrimaryResult: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "completed",
				}},
			}}, nil
		}},
		nil,
		nil,
		nil,
		true,
		0,
		func(
			_ context.Context,
			opening factorysessions.ApplicationOpeningRequest,
			_ *zap.Logger,
			_ factoryvisualization.Sink,
		) (initializer.LocalRuntimeRunner, error) {
			if opening.Ports.RuntimeHTTPServicesBound == nil {
				t.Fatal("runtime HTTP services binding = nil")
			}
			opening.Ports.RuntimeHTTPServicesBound(factorysessions.RuntimeHTTPServices{FactorySessions: sessions})
			return stubFactoryService{run: opening.Completion}, nil
		},
		func(RunConfig, *workers.MockWorkersConfig, factorysessions.RuntimeHostObserver) factorysessions.ApplicationOpeningRequest {
			return factorysessions.ApplicationOpeningRequest{}
		},
	)
	if err != nil {
		t.Fatalf("openHostedRuntime() error = %v", err)
	}
	if err := operation.Run(t.Context()); err != nil {
		t.Fatalf("Operation.Run() error = %v", err)
	}
}

type hostedInvocationCapabilityFake struct {
	factorysessions.Service
}
