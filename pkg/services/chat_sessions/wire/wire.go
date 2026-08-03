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
//
// It also re-publishes the package-private factorysessionsshim's narrow
// Factory Session start/invoke/cancel/close contract as FactoryTargetService
// so a consumer outside the chat_sessions tree (the ACP prompt-delegation
// transport and its own wire graph) can construct and hold that
// collaborator without importing the internal shim package itself, and
// without the chat_sessions public root (chatsessions.Service) taking on a
// Factory Sessions dependency.
package wire

import (
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

// NewService constructs the singular in-memory Chat Sessions root from
// explicit construction ports. newID and now are required; logger is
// optional and defaults to a no-op logger when omitted. This is the one
// canonical constructor for chatsessions.Service: production code has no
// alternate path to a Service value.
func NewService(newID IDGenerator, now Clock, logger ...logging.Logger) (chatsessions.Service, error) {
	if newID == nil {
		return nil, fmt.Errorf("construct chat sessions: id generator is required")
	}
	if now == nil {
		return nil, fmt.Errorf("construct chat sessions: clock is required")
	}
	return internalservice.NewStore(newID, now, logger...), nil
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

// FactoryTargetService is factorysessionsshim.FactoryTargetService,
// re-published here so a caller outside the chat_sessions tree can declare a
// field or parameter of this type without importing the internal shim
// package directly.
type FactoryTargetService = factorysessionsshim.FactoryTargetService

// NewFactoryTargetService constructs the Factory Sessions shim over the
// given public factory_sessions.Service root -- the one consumer-owned
// bridge from Chat Sessions-adjacent transports to Factory Sessions'
// start/invoke/cancel/close operations. It forwards the published
// factory_sessions vocabulary unmodified; this package does not reinterpret
// results or reach into Factory Sessions internals.
func NewFactoryTargetService(service factorysessions.Service) FactoryTargetService {
	return factorysessionsshim.New(service)
}
