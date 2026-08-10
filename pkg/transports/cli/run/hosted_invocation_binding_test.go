package run

import (
	"context"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
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
		testInvocationOperation{},
		nil,
		nil,
		nil,
		true,
		0,
		func(
			_ context.Context,
			opening factorysessions.ApplicationOpeningRequest,
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
	if !sessions.invoked {
		t.Fatal("hosted Factory Sessions invocation was not used")
	}
}

type hostedInvocationCapabilityFake struct {
	factorysessions.Service
	invoked bool
}

func (fake *hostedInvocationCapabilityFake) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{}, nil
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
