package run

import (
	"context"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestOpenHostedRuntimeBindsFactorySessionsInvocationCapability(t *testing.T) {
	sessions := &hostedInvocationCapabilityFake{}
	request := invocationRequestFromText("summarize the dispatch")
	owner := factorysessionwire.NewOpeningPresentationOwner()

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
		) (initializer.LocalRuntimeRunner, error) {
			scope, ok := owner.Application(opening.ScopeID)
			if !ok || scope.RuntimeHTTPServicesBound == nil {
				t.Fatal("runtime HTTP services binding = nil")
			}
			scope.RuntimeHTTPServicesBound(factorysessions.RuntimeHTTPServices{FactorySessions: sessions})
			if scope.RuntimeHostObserver != nil {
				scope.RuntimeHostObserver(factorysessions.RuntimeHostBinding{Host: "127.0.0.1", Port: 1})
			}
			return stubFactoryService{run: scope.Completion}, nil
		},
		func(RunConfig, *workers.MockWorkersConfig) factorysessions.ApplicationOpeningRequest {
			return factorysessions.ApplicationOpeningRequest{}
		},
		owner,
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
