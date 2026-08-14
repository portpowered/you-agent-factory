package workers

import (
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// CanonicalDraftFragment carries one provider-native canonical response draft
// through the Workers-owned progress publication contract.
func CanonicalDraftFragment(dispatchID string, draft any) ProgressFragment {
	return ProgressFragment{DispatchID: strings.TrimSpace(dispatchID), CanonicalDraft: draft}
}

const (
	ProgressFragmentKind  = "PROGRESS_FRAGMENT"
	ResponseFragmentKind  = "RESPONSE_FRAGMENT"
	CompletedFragmentKind = "STREAM_COMPLETED"
	FailedFragmentKind    = "STREAM_FAILED"
	// ProviderSessionObservedFragmentKind is an internal hand-off of a
	// provider-authored exact session identity. The Worker Sessions bridge
	// commits it before any dependent response output and does not expose this
	// bookkeeping fragment on the response stream itself.
	ProviderSessionObservedFragmentKind = "PROVIDER_SESSION_OBSERVED"
)

// ProgressFragment is the provider-neutral transient observation emitted by
// Workers and accepted by a Factory Session response stream.
type ProgressFragment struct {
	// Correlation is the detached identity of the attempt that emitted this
	// progress fact. DispatchID remains populated for compatibility with
	// existing stream consumers.
	Correlation ExecutionCorrelation
	DispatchID  string
	Kind        string
	Type        string
	Payload     string
	// Provider is the provider identity selected for this attempt. It is
	// intentionally independent from ProviderSessionReference: a provider may
	// author progress without exposing a resumable native session identity.
	Provider string
	// ProviderSessionReference is the exact detached typed reference returned
	// by Providers for this attempt. ProviderSessionRef remains the
	// response-stream metadata projection; Worker Sessions must use this field
	// when it associates a resumable execution before forwarding output.
	ProviderSessionReference       *providers.SessionRef
	ProviderSessionRef             *providersessions.Metadata
	ExternalEventType              string
	Metadata                       map[string]string
	CanonicalDraft                 any
	CanonicalEventAlreadyPublished bool
}

// ProgressPublisher receives transient Worker observations.
type ProgressPublisher func(ProgressFragment)

// CloneProviderSessionReference returns a detached copy of the exact typed
// Provider Session reference carried by a Workers progress fragment.
func CloneProviderSessionReference(reference *providers.SessionRef) *providers.SessionRef {
	if reference == nil {
		return nil
	}
	cloned := reference.Clone()
	return &cloned
}
