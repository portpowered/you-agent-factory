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
	"github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/responsebridge"
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

// ResponseBridge is the Chat Sessions-owned producer bridge that sequences
// Factory Session response events onto a Chat Session aggregate stream.
type ResponseBridge = responsebridge.Service

// NewResponseBridge constructs the response-event bridge over the canonical
// Chat Sessions sequence/head-advance and Factory Sessions target-execution
// capabilities. Its logger is the direct, required operation-logging
// abstraction; callers that do not need output pass logging.NoopLogger{}.
func NewResponseBridge(
	sequencer responsebridge.Sequencer,
	factoryTarget factorysessions.TargetExecutionService,
	logger logging.Logger,
) *ResponseBridge {
	return responsebridge.New(sequencer, factoryTarget, logger)
}
