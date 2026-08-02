package chatsessions

import "fmt"

// RequestIdentityInvalidReason classifies why a RequestIdentity failed
// validation without exposing the supplied identity values.
type RequestIdentityInvalidReason string

const (
	// RequestIdentityInvalidEmpty reports that no identity mode was supplied.
	RequestIdentityInvalidEmpty RequestIdentityInvalidReason = "EMPTY"
	// RequestIdentityInvalidBareJSONRPCID reports a JSON-RPC id supplied
	// without its pairing connection id.
	RequestIdentityInvalidBareJSONRPCID RequestIdentityInvalidReason = "BARE_JSONRPC_ID"
	// RequestIdentityInvalidIncompleteConnectionPair reports a connection id
	// supplied without its pairing JSON-RPC id.
	RequestIdentityInvalidIncompleteConnectionPair RequestIdentityInvalidReason = "INCOMPLETE_CONNECTION_PAIR"
	// RequestIdentityInvalidMixedIdentityModes reports both a
	// connection-qualified field and a transport-minted opaque id supplied
	// together.
	RequestIdentityInvalidMixedIdentityModes RequestIdentityInvalidReason = "MIXED_IDENTITY_MODES"
)

// InvalidRequestIdentityError reports a RequestIdentity that matches neither
// legal identity mode. Peers branch on Reason; the error never carries the
// supplied identity values, so it is safe to log.
type InvalidRequestIdentityError struct {
	Reason RequestIdentityInvalidReason
}

func (e *InvalidRequestIdentityError) Error() string {
	return fmt.Sprintf("invalid chat session request identity: %s", e.Reason)
}

// InvalidChatTargetKindError reports a ChatTargetKind outside the exact
// declared value set (FACTORY, WORKER).
type InvalidChatTargetKindError struct {
	Kind ChatTargetKind
}

func (e *InvalidChatTargetKindError) Error() string {
	return fmt.Sprintf("invalid chat target kind: %q", string(e.Kind))
}

// InvalidChatTargetRefError reports a ChatTargetRef missing its canonical
// reference.
type InvalidChatTargetRefError struct {
	Kind ChatTargetKind
}

func (e *InvalidChatTargetRefError) Error() string {
	return fmt.Sprintf("invalid chat target ref: empty canonical ref for kind %q", string(e.Kind))
}

// InvalidSessionStateError reports a SessionState outside the exact declared
// value set (CREATED, ACTIVE, CLOSED).
type InvalidSessionStateError struct {
	State SessionState
}

func (e *InvalidSessionStateError) Error() string {
	return fmt.Sprintf("invalid session state: %q", string(e.State))
}

// InvalidSessionStateTransitionError reports a SessionState transition
// outside the declared legal transition table.
type InvalidSessionStateTransitionError struct {
	From SessionState
	To   SessionState
}

func (e *InvalidSessionStateTransitionError) Error() string {
	return fmt.Sprintf("invalid session state transition: %q to %q", string(e.From), string(e.To))
}

// InvalidTargetEpisodeStateError reports a TargetEpisodeState outside the
// exact declared value set (OPEN, CLOSED).
type InvalidTargetEpisodeStateError struct {
	State TargetEpisodeState
}

func (e *InvalidTargetEpisodeStateError) Error() string {
	return fmt.Sprintf("invalid target episode state: %q", string(e.State))
}

// InvalidTargetEpisodeStateTransitionError reports a TargetEpisodeState
// transition outside the declared legal transition table.
type InvalidTargetEpisodeStateTransitionError struct {
	From TargetEpisodeState
	To   TargetEpisodeState
}

func (e *InvalidTargetEpisodeStateTransitionError) Error() string {
	return fmt.Sprintf("invalid target episode state transition: %q to %q", string(e.From), string(e.To))
}

// TargetEpisodeNotClosedError reports an attempt to open the next Target
// Episode while the prior episode is not yet closed.
type TargetEpisodeNotClosedError struct {
	Number uint64
	State  TargetEpisodeState
}

func (e *TargetEpisodeNotClosedError) Error() string {
	return fmt.Sprintf("target episode %d is not closed: state %q", e.Number, string(e.State))
}
