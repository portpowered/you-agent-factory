package durableexecution_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
)

// Compile-time seal: durable_execution exposes only the published durable
// durable execution facet. Identity, live-runtime, invocation, response-stream,
// and runtime-opening ownership stay outside this public surface.
var _ durableexecution.Service = (durableexecution.Service)(nil)

// surfaceExecutionCollaborator is an injected execution backend stand-in. Wire
// construction must bind this collaborator without selecting sibling Session
// subservices or application Wire/root types at the durable_execution boundary.
type surfaceExecutionCollaborator struct {
	durableexecution.Service
	startCalls int
}

func (c *surfaceExecutionCollaborator) StartAsync(
	context.Context,
	factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	c.startCalls++
	return factorysessions.AsyncStartResult{SessionID: "dur-sess-surface"}, nil
}

func TestDurableExecutionPublicSurfaceAcceptsOnlyInjectedExecutionCollaborator(t *testing.T) {
	t.Parallel()

	collaborator := &surfaceExecutionCollaborator{}
	service, err := durableexecutionwire.NewService(collaborator)
	if err != nil {
		t.Fatalf("wire.NewService with injected execution only: %v", err)
	}
	if service == nil {
		t.Fatal("wire.NewService returned nil service")
	}

	var execution durableexecution.Service = service
	if execution == nil {
		t.Fatal("owner Service must remain assignable to the durable execution contract")
	}
	if collaborator.startCalls != 0 {
		t.Fatalf("construction invoked execution %d times, want inert bind", collaborator.startCalls)
	}

	started, err := service.StartAsync(context.Background(), factorysessions.StartRequest{RequestID: "req-surface"})
	if err != nil || started.SessionID != "dur-sess-surface" {
		t.Fatalf("StartAsync = (%#v, %v), want published durable start through owner", started, err)
	}
	if collaborator.startCalls != 1 {
		t.Fatalf("runtime start calls = %d, want 1 through injected collaborator", collaborator.startCalls)
	}
}
