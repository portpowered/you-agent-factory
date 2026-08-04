// Package wire is the Chat Sessions service composition boundary.
//
// Wire performs construction only, starts no lifecycle components, and
// composes both canonical Chat Sessions roots this package publishes:
// chatsessions.Service (the in-memory session-state engine, backed by the
// chat_sessions-private Store) and chatsessions.FactoryTargetCatalogService
// (the Factory target-catalog operation, composed by direct single
// injection of the singular Operator Settings public service root and
// Factory Definitions' narrow, read-only CatalogPathsService capability).
// Neither constructor is a dependency bag, service locator, or alternate
// construction path for the other's root.
package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/factorysessionsshim"
	internalservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
// target-catalog root from the singular Operator Settings public service
// root and Factory Definitions' narrow, read-only catalog/path capability.
// logger is the direct, required operation-logging abstraction; callers with
// no operation logging pass logging.NoopLogger{}.
func NewFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.CatalogPathsService,
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
