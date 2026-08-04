// Package wire is the Chat Sessions service composition boundary.
//
// Wire performs construction only, starts no lifecycle components, and
// composes both canonical Chat Sessions roots this package publishes:
// chatsessions.Service (the in-memory session-state engine, backed by the
// chat_sessions-private Store) and chatsessions.FactoryTargetCatalogService
// (the Factory target-catalog operation, composed by direct single
// injection of the singular Operator Settings and Factory Definitions
// public service roots). Neither constructor is a dependency bag, service
// locator, or alternate construction path for the other's root.
package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/factorysessionsshim"
	internalservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// IDGenerator produces a new opaque, process-unique entity identity for
// every Session, Turn, and Attachment the constructed Service creates.
type IDGenerator = internalservice.IDGenerator

// Clock returns the current time for every timestamp the constructed
// Service records.
type Clock = internalservice.Clock

// EventsAppender is the narrow Events dependency the constructed Service's
// Sequence operation commits source-native records through. Any
// events.Service value satisfies this interface structurally.
type EventsAppender = internalservice.EventsAppender

// EventsReader is the narrow Events dependency the constructed Service's
// AcknowledgeAttachment operation reads through to detect a retention gap
// between an attachment's current and requested position. Any
// events.Service value satisfies this interface structurally.
type EventsReader = internalservice.EventsReader

// NewService constructs the singular in-memory Chat Sessions root from
// explicit construction ports. newID, now, eventsAppender, and eventsReader
// are required; logger is optional and defaults to a no-op logger when
// omitted. This is the one canonical constructor for chatsessions.Service:
// production code has no alternate path to a Service value.
func NewService(newID IDGenerator, now Clock, eventsAppender EventsAppender, eventsReader EventsReader, logger ...logging.Logger) (chatsessions.Service, error) {
	if newID == nil {
		return nil, fmt.Errorf("construct chat sessions: id generator is required")
	}
	if now == nil {
		return nil, fmt.Errorf("construct chat sessions: clock is required")
	}
	if eventsAppender == nil {
		return nil, fmt.Errorf("construct chat sessions: events appender is required")
	}
	if eventsReader == nil {
		return nil, fmt.Errorf("construct chat sessions: events reader is required")
	}
	return internalservice.NewStore(newID, now, eventsAppender, eventsReader, logger...), nil
}

// NewFactoryTargetCatalogService constructs the Chat Sessions Factory
// target-catalog root from the singular Operator Settings and Factory
// Definitions public service roots. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.Service,
	logger logging.Logger,
) (chatsessions.FactoryTargetCatalogService, error) {
	return internalservice.New(operatorSettings, factoryDefinitions, logger)
}

// FactoryTargetService is the Factory-target start/invoke/cancel/close
// dependency the existing, consumer-owned Factory Sessions shim exposes.
// Re-published here (an alias for factorysessionsshim.FactoryTargetService,
// this shim's own private contract) exclusively for pkg/wire's use, per the
// pkg-boundary rule that only pkg/wire may import a service's own wire
// subpackage -- see NewFactoryTargetService.
type FactoryTargetService = factorysessionsshim.FactoryTargetService

// FactoryTargetExecutionService is the narrow start/invoke/cancel/close
// execution dependency the shim actually forwards to (re-published for the
// same reason as FactoryTargetService). Any concrete
// factorysessions.Service -- the CLI daemon's full singleton, or a narrower,
// consumer-owned activation like factory_sessions/wire's own
// OnDemandFactoryTargetService that implements only these four methods --
// satisfies it structurally.
type FactoryTargetExecutionService = factorysessionsshim.FactoryTargetExecutionService

// NewFactoryTargetService constructs the existing Chat Sessions-owned
// Factory Sessions shim (factorysessionsshim.Shim) over the given execution
// service. It is a stateless, exactly-once-forwarding adapter: this
// constructor performs no I/O and adds no behavior beyond what
// factorysessionsshim.New itself already does. pkg/wire is the only intended
// caller (chat_sessions/internal/factorysessionsshim cannot be imported
// directly outside this service's own tree).
func NewFactoryTargetService(service FactoryTargetExecutionService) FactoryTargetService {
	return factorysessionsshim.New(service)
}

// FactoryResponseEventSubscriber is the Factory Sessions response-event
// subscription dependency RunWithResponseBridge subscribes through,
// re-published here (an alias for factorysessionsshim.ResponseEventSubscriber)
// exclusively for pkg/wire's use, matching FactoryTargetService's own
// re-publishing convention.
type FactoryResponseEventSubscriber = factorysessionsshim.ResponseEventSubscriber

// RunWithResponseBridge starts subscribing to one Factory Session's
// response-event stream (through subscriber) and sequencing every event it
// observes onto one Chat Session's aggregate stream (through chatSessions),
// concurrently with invoke; it also runs liveDrain (the ACP transport's own
// genuine mid-generation consumer loop) concurrently with the same invoke
// call. It returns invoke's own result and error unchanged once invoke
// itself returns. Re-published here (delegating to
// factorysessionsshim.RunWithResponseBridge, this service's own internal
// implementation, which is also the one place that owns both goroutines and
// their join channels) exclusively for pkg/wire's use, the same reason
// NewFactoryTargetService is: a caller outside this service's own tree (in
// particular the ACP transport) only ever holds plain function values of
// this shape, never a raw concurrency primitive of its own.
func RunWithResponseBridge(
	ctx context.Context,
	chatSessions chatsessions.Service,
	subscriber FactoryResponseEventSubscriber,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error) {
	return factorysessionsshim.RunWithResponseBridge(ctx, chatSessions, subscriber, chatSessionID, sessionVersion, factorySessionID, liveDrain, invoke)
}
