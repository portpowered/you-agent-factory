package factorysessions

const (
	// CurrentFactoryName is the domain identifier for the current Factory selector.
	CurrentFactoryName = "UNDEFINED"

	// SessionEventStreamRetainedCountHeader identifies the response header that
	// bounds the committed retained-history prefix of a Factory Event stream.
	SessionEventStreamRetainedCountHeader = "X-Factory-Session-Retained-Event-Count"
)

// OpenRequest is the transport-independent request to discover, validate, or
// open a Factory Session from a folder.
type OpenRequest struct {
	FolderPath     string
	Target         *TargetRef
	ValidateOnly   bool
	InitNewFactory bool
}
