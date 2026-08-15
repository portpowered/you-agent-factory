package testkit

import "github.com/portpowered/infinite-you/pkg/services/providers"

// ContinuationForSession adapts fixture-only session identity into the opaque
// continuation carried by the Workers execution contract.
func ContinuationForSession(session *providers.SessionMetadata) *providers.ContinuationRef {
	return (session).ContinuationRef()
}

func sessionMetadataFromContinuation(reference *providers.ContinuationRef) *providers.SessionMetadata {
	return (reference).SessionMetadata()
}
