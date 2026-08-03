// Package wire is the Chat Sessions service composition boundary.
//
// Wire performs construction only, returns the singular chatsessions.Service
// root interface, and starts no lifecycle components. The in-memory Store
// implementation stays a chat_sessions-private detail; peers depend on
// Service rather than Store or its construction ports.
package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
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
	return internalservice.New(newID, now, logger...), nil
}
