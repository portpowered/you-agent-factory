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
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/responsebridge"
	internalservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/events"
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

// NewService constructs the singular in-memory Chat Sessions root from direct,
// required dependencies. Quiet callers explicitly pass logging.NoopLogger{}.
// Production callers supply effects normalized at root.BuildProcess.
func NewService(newID IDGenerator, now Clock, eventsAppender EventsAppender, eventsReader EventsReader, logger logging.Logger) chatsessions.Service {
	return internalservice.NewStore(newID, now, eventsAppender, eventsReader, logger)
}

// NewFactoryTargetCatalogService constructs the Chat Sessions Factory
// target-catalog root from direct, required peers and operation logger.
// Quiet callers explicitly pass logging.NoopLogger{}.
func NewFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.CatalogPathsService,
	logger logging.Logger,
) chatsessions.FactoryTargetCatalogService {
	return internalservice.New(operatorSettings, factoryDefinitions, logger)
}

// ResponseBridge is the Chat Sessions-owned producer bridge that sequences
// Factory Session response events onto a Chat Session aggregate stream.
type ResponseBridge = responsebridge.Service

// NewResponseBridge constructs the response-event bridge over the canonical
// Chat Sessions sequence/head-advance and Factory Sessions target-execution
// capabilities. Its logger is the direct, required operation-logging
// abstraction; callers that do not need output pass logging.NoopLogger{}.
func NewResponseBridge(
	sequencer responsebridge.Sequencer,
	factorySessions factorysessions.Service,
	workerEvents events.Service,
	logger logging.Logger,
) *ResponseBridge {
	return responsebridge.New(sequencer, factorySessions, workerEvents, logger)
}
