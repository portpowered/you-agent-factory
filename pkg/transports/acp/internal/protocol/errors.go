package protocol

import (
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// redactedMethodPlaceholder is the only method value MethodNotFound ever
// discloses. A client-controlled "method" string is never echoed back --
// not even one that superficially looks like a plausible method name -- so
// no shape-matching heuristic can be bypassed by a credential, path,
// command, payload fragment, or topology sentinel crafted to look like a
// method name (e.g. "sk_live_credential_ABC123" or
// "internal_topology_node_7" are syntactically valid identifiers, so any
// regex admitting real method names would also admit them).
const redactedMethodPlaceholder = "unrecognized_method"

// RejectionKind is the closed set of bounded rejection reasons this
// transport is willing to disclose to a client. Every value is a fixed,
// static label; none of them carry request payload data, error text, or
// internal detail, so a RejectionKind can never itself leak a credential, a
// provider command, a path, or a prompt/tool payload.
type RejectionKind string

const (
	// RejectionUnsupportedContent marks a rejection caused by a non-text
	// prompt or update content variant (session.ErrUnsupportedContent).
	RejectionUnsupportedContent RejectionKind = "unsupported_content"
	// RejectionUnsupportedUpdate marks a rejection caused by a session/update
	// variant this transport declares no output for
	// (session.ErrUnsupportedUpdate).
	RejectionUnsupportedUpdate RejectionKind = "unsupported_update"
	// RejectionMalformedRequest is the default classification for every
	// other validation failure: a missing required field, an invalid shape,
	// or any other cause this package does not classify more specifically.
	RejectionMalformedRequest RejectionKind = "malformed_request"
)

// Classify maps an internal validation cause to the closed RejectionKind
// set this transport discloses to a client. It inspects only the cause's
// identity (via errors.Is against this package tree's sentinel errors),
// never its message text, so the classification itself cannot reproduce
// anything sensitive the cause's message might contain.
func Classify(cause error) RejectionKind {
	switch {
	case errors.Is(cause, session.ErrUnsupportedContent):
		return RejectionUnsupportedContent
	case errors.Is(cause, session.ErrUnsupportedUpdate):
		return RejectionUnsupportedUpdate
	default:
		return RejectionMalformedRequest
	}
}

// MethodNotFound returns the bounded JSON-RPC method-not-found error for an
// unsupported method. The client-sent method value is never echoed back --
// only the fixed redactedMethodPlaceholder ever reaches
// acpsdk.NewMethodNotFound -- so a credential, filesystem path, shell
// command, tool/prompt payload fragment, or internal topology sentinel a
// client sends as "method" can never be disclosed through this error,
// regardless of whether it happens to look like a plausible method name.
func MethodNotFound(method string) *acpsdk.RequestError {
	_ = method
	return acpsdk.NewMethodNotFound(redactedMethodPlaceholder)
}

// SafeReject converts an internal validation cause into a bounded,
// protocol-safe invalid-params error. The cause's message is never
// serialized: only its closed RejectionKind classification crosses the
// protocol boundary. Because Classify never reads the cause's message text,
// credentials, provider commands, absolute paths, and prompt/tool payloads
// that an internal cause happens to mention can never reach the client
// through this constructor, and the same cause always classifies the same
// way.
func SafeReject(cause error) *acpsdk.RequestError {
	return acpsdk.NewInvalidParams(map[string]any{"reason": string(Classify(cause))})
}

// ParseError returns the bounded JSON-RPC parse-error response for input
// that never parsed as JSON at all. It carries only a fixed, static reason
// label -- never the underlying parse cause's message -- so it can never
// disclose fragments of unparseable client input.
func ParseError() *acpsdk.RequestError {
	return acpsdk.NewParseError(map[string]any{"reason": "invalid_json"})
}

// InvalidRequest returns the bounded JSON-RPC invalid-request response for
// input that parsed as JSON but violates the JSON-RPC 2.0 request shape
// this transport requires (for example a missing method, a wrong protocol
// version token, or an id-bearing message for a notification-only method).
// It carries only a fixed, static reason label, for the same reason
// ParseError does.
func InvalidRequest() *acpsdk.RequestError {
	return acpsdk.NewInvalidRequest(map[string]any{"reason": "invalid_request_shape"})
}

// RejectEnvelope classifies an envelope.Decode failure into the bounded
// JSON-RPC error to serialize, and the JSON-RPC id, if any, to correlate
// the response to. Completely unparseable JSON classifies as ParseError and
// is never correlated, matching JSON-RPC 2.0's requirement that a parse
// error response always carries a null id. Every other envelope.Decode
// failure classifies as InvalidRequest and is correlated to the message's
// id only when envelope.DecodeError reports that id as syntactically valid
// -- an id that is itself malformed, or altogether absent, has nothing to
// correlate to. A cause that is not an *envelope.DecodeError (for example a
// method-specific params decode failure a caller passes through this same
// path) falls back to the general SafeReject classification and is never
// correlated by this function; a caller with its own recoverable id for
// that case correlates it separately.
func RejectEnvelope(cause error) (*acpsdk.RequestError, identity.JSONRPCID, bool) {
	var decodeErr *envelope.DecodeError
	if errors.As(cause, &decodeErr) {
		if errors.Is(decodeErr, envelope.ErrInvalidJSON) {
			return ParseError(), identity.JSONRPCID{}, false
		}
		id, ok := decodeErr.ID()
		return InvalidRequest(), id, ok
	}
	return SafeReject(cause), identity.JSONRPCID{}, false
}
