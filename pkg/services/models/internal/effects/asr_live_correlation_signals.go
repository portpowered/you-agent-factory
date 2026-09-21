package effects

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrASRLiveCorrelationInvalid        = errors.New("invalid ASR live-correlation signal")
	ErrASRLiveCorrelationDuplicate      = errors.New("duplicate ASR live-correlation signal")
	ErrASRLiveCorrelationOwnership      = errors.New("ASR live-correlation endpoint ownership rejected")
	ErrASRLiveCorrelationOutOfOrder     = errors.New("ASR live-correlation signal arrived out of order")
	ErrASRLiveCorrelationMissing        = errors.New("ASR live-correlation signal missing")
	ErrASRLiveCorrelationCancelled      = errors.New("ASR live-correlation response cancelled")
	ErrASRLiveCorrelationListenerAbsent = errors.New("ASR live-correlation listener is absent")
	ErrASRLiveCorrelationListenerLookup = errors.New("ASR live-correlation listener lookup failed")
)

type ASRLiveCorrelationEventKind string

const (
	ASRLiveCorrelationChildStarted     ASRLiveCorrelationEventKind = "CHILD_STARTED"
	ASRLiveCorrelationEndpointObserved ASRLiveCorrelationEventKind = "ENDPOINT_OBSERVED"
	ASRLiveCorrelationRPCTerminal      ASRLiveCorrelationEventKind = "RPC_TERMINAL"
	ASRLiveCorrelationResponseSent     ASRLiveCorrelationEventKind = "RESPONSE_RELEASED"
	ASRLiveCorrelationChildWaited      ASRLiveCorrelationEventKind = "CHILD_WAITED"
	ASRLiveCorrelationHostFailureSeen  ASRLiveCorrelationEventKind = "HOST_FAILURE_OBSERVED"
	ASRLiveCorrelationCancelled        ASRLiveCorrelationEventKind = "CANCELLED"
)

// ASRLiveCorrelationEvent contains only endpoint identity, process identity,
// semantic digests, and terminal classifications. It never retains payloads,
// paths, endpoint strings, or backend errors.
type ASRLiveCorrelationEvent struct {
	Sequence               uint64                      `json:"sequence"`
	Kind                   ASRLiveCorrelationEventKind `json:"kind"`
	Endpoint               ASRLiveCorrelationEndpoint  `json:"endpoint,omitempty"`
	ProcessID              int                         `json:"process_id,omitempty"`
	ExitClass              string                      `json:"exit_class,omitempty"`
	ExitCodeKnown          bool                        `json:"exit_code_known,omitempty"`
	ExitCode               int                         `json:"exit_code,omitempty"`
	RequestSemanticSHA256  string                      `json:"request_semantic_sha256,omitempty"`
	ResponseSemanticSHA256 string                      `json:"response_semantic_sha256,omitempty"`
}

// ASRLiveCorrelationListenerPIDLookup returns the process that owns a loopback
// TCP listener. The callback receives only a validated loopback address and
// port; its error is intentionally discarded at the evidence boundary.
type ASRLiveCorrelationListenerPIDLookup func(context.Context, string, int) (int, error)

// ASRLiveCorrelationController owns the private signal sequence for one
// invocation. Its caller supplies one process-owned listener lookup and must
// explicitly release the decoded response after its chosen causal boundary.
type ASRLiveCorrelationController struct {
	mu sync.Mutex

	lookup ASRLiveCorrelationListenerPIDLookup

	childPID        int
	childHost       string
	childPort       int
	started         bool
	endpointCheck   bool
	endpointSeen    bool
	endpoint        ASRLiveCorrelationEndpoint
	requestDigest   string
	terminalSeen    bool
	released        bool
	cancelled       bool
	waitSeen        bool
	hostFailureSeen bool

	nextSequence uint64
	events       []ASRLiveCorrelationEvent
	changed      chan struct{}
	release      chan struct{}
}

// NewASRLiveCorrelationController constructs an invocation-owned controller.
func NewASRLiveCorrelationController(
	lookup ASRLiveCorrelationListenerPIDLookup,
) (*ASRLiveCorrelationController, error) {
	if lookup == nil {
		return nil, fmt.Errorf("%w: listener PID lookup is required", ErrASRLiveCorrelationInvalid)
	}
	return &ASRLiveCorrelationController{
		lookup: lookup, changed: make(chan struct{}), release: make(chan struct{}),
	}, nil
}

// RecordManagedChildStarted binds this controller to the one owned child and
// its loopback endpoint before the process can be supervised by Runtime Host.
func (controller *ASRLiveCorrelationController) RecordManagedChildStarted(
	processID int,
	endpointAddress string,
) error {
	host, port, err := parseASRCorrelationEndpointAddress(endpointAddress)
	if err != nil || processID <= 0 {
		return ErrASRLiveCorrelationOwnership
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.started {
		return ErrASRLiveCorrelationDuplicate
	}
	controller.childPID = processID
	controller.childHost = host
	controller.childPort = port
	controller.started = true
	controller.appendLocked(ASRLiveCorrelationEvent{
		Kind: ASRLiveCorrelationChildStarted, ProcessID: processID,
	})
	return nil
}

// ObserveEndpoint validates the invocation address and resolves the actual
// listener PID before assigning a sequence number to the endpoint fact.
func (controller *ASRLiveCorrelationController) ObserveEndpoint(
	ctx context.Context,
	address string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	host, port, err := parseASRCorrelationEndpointAddress(address)
	if err != nil {
		return ErrASRLiveCorrelationOwnership
	}
	controller.mu.Lock()
	if !controller.started || controller.waitSeen || host != controller.childHost || port != controller.childPort {
		controller.mu.Unlock()
		return ErrASRLiveCorrelationOwnership
	}
	if controller.endpointSeen || controller.endpointCheck {
		controller.mu.Unlock()
		return ErrASRLiveCorrelationDuplicate
	}
	controller.endpointCheck = true
	processID := controller.childPID
	controller.mu.Unlock()

	listenerPID, lookupErr := controller.lookup(ctx, host, port)
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.endpointCheck = false
	if lookupErr != nil || ctx.Err() != nil || listenerPID != processID || listenerPID <= 0 {
		return ErrASRLiveCorrelationOwnership
	}
	if controller.waitSeen {
		return ErrASRLiveCorrelationOutOfOrder
	}
	endpoint, err := ASRLiveCorrelationEndpointFromAddress(address, listenerPID, controller.childPort)
	if err != nil {
		return ErrASRLiveCorrelationOwnership
	}
	controller.endpoint = endpoint
	controller.endpointSeen = true
	controller.appendLocked(ASRLiveCorrelationEvent{
		Kind: ASRLiveCorrelationEndpointObserved, Endpoint: endpoint, ProcessID: processID,
	})
	return nil
}

// RecordRequestSemanticSHA256 retains only the request's normalized digest.
func (controller *ASRLiveCorrelationController) RecordRequestSemanticSHA256(digest string) error {
	if !validASRCorrelationDigest(digest) {
		return ErrASRLiveCorrelationInvalid
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if !controller.started || controller.requestDigest != "" {
		return ErrASRLiveCorrelationDuplicate
	}
	controller.requestDigest = digest
	return nil
}

// AwaitResponseRelease commits the post-decode terminal then holds its
// unchanged normalized result until the owner explicitly releases it.
func (controller *ASRLiveCorrelationController) AwaitResponseRelease(
	ctx context.Context,
	requestSemanticSHA256 string,
	responseSemanticSHA256 string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !validASRCorrelationDigest(requestSemanticSHA256) ||
		!validASRCorrelationDigest(responseSemanticSHA256) {
		return ErrASRLiveCorrelationInvalid
	}
	controller.mu.Lock()
	if !controller.started || !controller.endpointSeen || controller.waitSeen ||
		controller.terminalSeen || controller.requestDigest == "" ||
		controller.requestDigest != requestSemanticSHA256 {
		controller.mu.Unlock()
		return ErrASRLiveCorrelationOutOfOrder
	}
	controller.terminalSeen = true
	controller.appendLocked(ASRLiveCorrelationEvent{
		Kind:                   ASRLiveCorrelationRPCTerminal,
		RequestSemanticSHA256:  requestSemanticSHA256,
		ResponseSemanticSHA256: responseSemanticSHA256,
	})
	controller.mu.Unlock()

	select {
	case <-ctx.Done():
		return controller.cancelResponse(ctx.Err())
	case <-controller.release:
		controller.mu.Lock()
		cancelled := controller.cancelled
		controller.mu.Unlock()
		if cancelled {
			return fmt.Errorf("%w: %w", ErrASRLiveCorrelationCancelled, ctx.Err())
		}
		return nil
	}
}

// ReleaseResponse appends one release signal and wakes the decoded invocation.
func (controller *ASRLiveCorrelationController) ReleaseResponse() error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if !controller.terminalSeen || controller.cancelled {
		return ErrASRLiveCorrelationOutOfOrder
	}
	if controller.released {
		return ErrASRLiveCorrelationDuplicate
	}
	controller.released = true
	controller.appendLocked(ASRLiveCorrelationEvent{Kind: ASRLiveCorrelationResponseSent})
	close(controller.release)
	return nil
}

// RecordChildWaited appends terminal facts only for the owned process.
func (controller *ASRLiveCorrelationController) RecordChildWaited(
	processID int,
	exitClass string,
	exitCodeKnown bool,
	exitCode int,
) error {
	child := ASRLiveCorrelationChild{
		ProcessID: processID, ExitClass: exitClass,
		ExitCodeKnown: exitCodeKnown, ExitCode: exitCode,
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if !controller.started || processID != controller.childPID || !validASRCorrelationExit(child) {
		return ErrASRLiveCorrelationOwnership
	}
	if controller.waitSeen {
		return ErrASRLiveCorrelationDuplicate
	}
	controller.waitSeen = true
	controller.appendLocked(ASRLiveCorrelationEvent{
		Kind:      ASRLiveCorrelationChildWaited,
		ProcessID: processID, ExitClass: exitClass,
		ExitCodeKnown: exitCodeKnown, ExitCode: exitCode,
	})
	return nil
}

// RecordHostFailureObserved follows the process Wait signal only after
// Runtime Host has revoked the process-owned leases.
func (controller *ASRLiveCorrelationController) RecordHostFailureObserved(processID int) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if !controller.started || processID != controller.childPID || !controller.waitSeen {
		return ErrASRLiveCorrelationOutOfOrder
	}
	if controller.hostFailureSeen {
		return ErrASRLiveCorrelationDuplicate
	}
	controller.hostFailureSeen = true
	controller.appendLocked(ASRLiveCorrelationEvent{
		Kind: ASRLiveCorrelationHostFailureSeen, ProcessID: processID,
	})
	return nil
}

// WaitForSignal waits for a committed event, with context cancellation as a
// bounded safety ceiling. It never polls or sleeps.
func (controller *ASRLiveCorrelationController) WaitForSignal(
	ctx context.Context,
	kind ASRLiveCorrelationEventKind,
) (ASRLiveCorrelationEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		controller.mu.Lock()
		for _, event := range controller.events {
			if event.Kind == kind {
				controller.mu.Unlock()
				return event, nil
			}
		}
		changed := controller.changed
		controller.mu.Unlock()
		select {
		case <-ctx.Done():
			if event, ok := controller.event(kind); ok {
				return event, nil
			}
			return ASRLiveCorrelationEvent{}, fmt.Errorf("%w: %w", ErrASRLiveCorrelationMissing, ctx.Err())
		case <-changed:
		}
	}
}

// Snapshot returns a detached ordered copy of this invocation's signals.
func (controller *ASRLiveCorrelationController) Snapshot() []ASRLiveCorrelationEvent {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return append([]ASRLiveCorrelationEvent(nil), controller.events...)
}

func (controller *ASRLiveCorrelationController) event(
	kind ASRLiveCorrelationEventKind,
) (ASRLiveCorrelationEvent, bool) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	for _, event := range controller.events {
		if event.Kind == kind {
			return event, true
		}
	}
	return ASRLiveCorrelationEvent{}, false
}

func (controller *ASRLiveCorrelationController) cancelResponse(cause error) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.released {
		return nil
	}
	if !controller.cancelled {
		controller.cancelled = true
		controller.appendLocked(ASRLiveCorrelationEvent{Kind: ASRLiveCorrelationCancelled})
	}
	return fmt.Errorf("%w: %w", ErrASRLiveCorrelationCancelled, cause)
}

func (controller *ASRLiveCorrelationController) appendLocked(event ASRLiveCorrelationEvent) {
	controller.nextSequence++
	event.Sequence = controller.nextSequence
	controller.events = append(controller.events, event)
	close(controller.changed)
	controller.changed = make(chan struct{})
}

type asrLiveCorrelationContextKey struct{}

// WithASRLiveCorrelation opts one invocation into the private correlation seam.
func WithASRLiveCorrelation(
	ctx context.Context,
	controller *ASRLiveCorrelationController,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if controller == nil {
		return ctx
	}
	return context.WithValue(ctx, asrLiveCorrelationContextKey{}, controller)
}

// ASRLiveCorrelationFromContext returns the optional per-invocation owner.
func ASRLiveCorrelationFromContext(ctx context.Context) *ASRLiveCorrelationController {
	if ctx == nil {
		return nil
	}
	controller, _ := ctx.Value(asrLiveCorrelationContextKey{}).(*ASRLiveCorrelationController)
	return controller
}
