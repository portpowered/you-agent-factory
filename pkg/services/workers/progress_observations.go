package workers

import providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"

const (
	ProgressFragmentKind  = "PROGRESS_FRAGMENT"
	ResponseFragmentKind  = "RESPONSE_FRAGMENT"
	CompletedFragmentKind = "STREAM_COMPLETED"
	FailedFragmentKind    = "STREAM_FAILED"
)

// ProgressFragment is the provider-neutral transient observation emitted by
// Workers and accepted by a Factory Session response stream.
type ProgressFragment struct {
	DispatchID                     string
	Kind                           string
	Type                           string
	Payload                        string
	ProviderSessionRef             *providersessions.Metadata
	ExternalEventType              string
	Metadata                       map[string]string
	CanonicalDraft                 any
	CanonicalEventAlreadyPublished bool
}

// ProgressPublisher receives transient Worker observations.
type ProgressPublisher func(ProgressFragment)
