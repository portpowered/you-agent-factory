package factorysessions

import "github.com/portpowered/infinite-you/pkg/services/events"

const (
	// CurrentFactoryName is the domain identifier for the current Factory selector.
	CurrentFactoryName = "UNDEFINED"

	// SessionEventStreamRetainedCountHeader identifies the response header that
	// bounds the committed retained-history prefix of a Factory Event stream.
	// The spelling is owned by the shared Events vocabulary so producers and
	// consumers of retained history cannot drift apart.
	SessionEventStreamRetainedCountHeader = events.RetainedEventCountHeader
)

// OpenRequest is the transport-independent request to discover, validate, or
// open a Factory Session from a folder.
type OpenRequest struct {
	FolderPath     string
	Target         *TargetRef
	ValidateOnly   bool
	InitNewFactory bool
}
