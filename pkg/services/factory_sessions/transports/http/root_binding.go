package http

import (
	"reflect"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// SessionsRoot is the accepted Factory Sessions root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Sessions
// internal packages.
type SessionsRoot = factorysessions.Service

// RootBinding binds the HTTP adapter to one injected Sessions root.
type RootBinding struct {
	Sessions SessionsRoot
	Prepare  RequestPreparation
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Sessions root. Tests inject a focused fake implementing SessionsRoot without
// constructing durable runtime state or importing Sessions internals.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Adapter {
	if binding.Prepare == nil {
		binding.Prepare = noopRequestPreparation{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	durable := factorysessionmapping.NewDurableAPI(binding.Sessions)
	liveControl, _ := binding.Sessions.(factorysessions.LiveControlService)
	deletion, _ := binding.Sessions.(factorysessions.LiveDeletionService)
	return NewHandler(Dependencies{
		SessionsRoot:          binding.Sessions,
		LiveControl:           liveControl,
		SessionDeletion:       deletion,
		DurableExecution:      durable,
		DurableLifecycle:      durable,
		DurableListing:        durable,
		DurableResponseEvents: durable,
		SessionRequests:       binding.Prepare,
	}, logger)
}

// DurableRequestPreparation restates Factory Sessions request preparation in
// the shared transport input vocabulary. Compatibility transports that forward
// a durable read without owning the Factory Sessions request contract accept
// this adapter instead of the service-typed preparation role.
type DurableRequestPreparation struct {
	prepare RequestPreparation
}

// NewDurableRequestPreparation adapts one Factory Sessions preparation role for
// compatibility transports. It returns nil when no role is supplied so the
// caller keeps its own "no preparation bound" behavior.
func NewDurableRequestPreparation(prepare RequestPreparation) *DurableRequestPreparation {
	if isAbsentPreparation(prepare) {
		return nil
	}
	return &DurableRequestPreparation{prepare: prepare}
}

// PrepareResult normalizes one transport-resolved durable result read.
func (adapter *DurableRequestPreparation) PrepareResult(
	input factorysessionmapping.DurableResultInput,
) (factorysessionmapping.DurableResultInput, error) {
	request, err := factorysessionmapping.ResultRequestFromInput(input)
	if err != nil {
		return factorysessionmapping.DurableResultInput{}, err
	}
	prepared, err := adapter.prepare.PrepareResult(request)
	if err != nil {
		return factorysessionmapping.DurableResultInput{}, err
	}
	return factorysessionmapping.DurableResultInput{
		Mode:             string(prepared.Mode),
		IncludeArtifacts: prepared.IncludeArtifacts,
	}, nil
}

// PrepareEventReconnect normalizes one transport-resolved reconnect read.
func (adapter *DurableRequestPreparation) PrepareEventReconnect(
	input factorysessionmapping.DurableEventReconnectInput,
) (factorysessionmapping.DurableEventReconnectInput, error) {
	request, err := factorysessionmapping.EventReconnectRequestFromInput(input)
	if err != nil {
		return factorysessionmapping.DurableEventReconnectInput{}, err
	}
	prepared, err := adapter.prepare.PrepareEventReconnect(request)
	if err != nil {
		return factorysessionmapping.DurableEventReconnectInput{}, err
	}
	return factorysessionmapping.DurableEventReconnectInput{
		AfterEventID:  prepared.AfterEventID,
		AfterSequence: prepared.AfterSequence,
	}, nil
}

// isAbsentPreparation reports whether a preparation role is absent, including
// the typed-nil values compatibility callers routinely pass for "not bound".
func isAbsentPreparation(prepare RequestPreparation) bool {
	if prepare == nil {
		return true
	}
	reflected := reflect.ValueOf(prepare)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type noopRequestPreparation struct{}

func (noopRequestPreparation) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	return normalizeListSessionsScope(request)
}
func (noopRequestPreparation) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	return request, nil
}
