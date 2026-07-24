package invocation_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	invocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Compile-time: the nested invocation Service is only the prepare/command/
// observe roles. Opening, identity, live-runtime, durable-execution, and
// response-stream ownership stay outside this public surface.
var (
	_ roles.SessionInvoker          = (invocation.Service)(nil)
	_ roles.InvocationInputResolver = (invocation.Service)(nil)
)

// surfaceFakeWorkPeer is a CTR-WORK peer-root stand-in that is not a Work
// implementation type. Assignability to WorkAdmission proves construction
// accepts the injected admission port rather than selecting work/service.
type surfaceFakeWorkPeer struct {
	submitCalls int
}

func (f *surfaceFakeWorkPeer) SubmitWorkRequestForSession(
	_ context.Context,
	_ string,
	_ work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.submitCalls++
	return work.WorkRequestSubmitResult{RequestID: "req-surface-1", TraceID: "trace-surface-1", Accepted: true}, nil
}

var _ invocation.WorkAdmission = (*surfaceFakeWorkPeer)(nil)

// TestInvocationPublicSurfaceAcceptsOnlyInjectedWorkAdmissionPort proves
// Dependencies.Work is the WorkAdmission port and wire construction binds a
// fake peer without Work implementation, Runtime/Petri, or Wire/root types at
// the call site.
func TestInvocationPublicSurfaceAcceptsOnlyInjectedWorkAdmissionPort(t *testing.T) {
	t.Parallel()

	peer := &surfaceFakeWorkPeer{}
	var deps invocation.Dependencies
	deps.Work = peer
	deps.FactoryConfig = func(string) (*factorydefinitions.FactoryConfig, error) {
		return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		}}}, nil
	}
	deps.Observe = func(
		context.Context,
		string,
		legacyinvocation.SessionInvocationWaitInput,
	) (legacyinvocation.SessionInvocationObservation, error) {
		return legacyinvocation.SessionInvocationObservation{}, nil
	}
	deps.Interpolation = factorydefinitionfixtures.InvocationInterpolation{}
	deps.WorkTypes = surfaceWorkType("task")
	deps.InputFiles = func(string) ([]byte, error) { return nil, nil }

	service, err := invocationwire.New(deps)
	if err != nil {
		t.Fatalf("wire.New with injected WorkAdmission only: %v", err)
	}
	if service == nil {
		t.Fatal("wire.New service = nil")
	}

	var asInvoker roles.SessionInvoker = service
	var asResolver roles.InvocationInputResolver = service
	if asInvoker == nil || asResolver == nil {
		t.Fatal("Service must remain assignable only to SessionInvoker and InvocationInputResolver roles")
	}
	if peer.submitCalls != 0 {
		t.Fatalf("construction must not command Work; submitCalls = %d", peer.submitCalls)
	}
}

type surfaceWorkType string

func (workType surfaceWorkType) DefaultWorkType(*factorydefinitions.FactoryConfig) (string, error) {
	return string(workType), nil
}
