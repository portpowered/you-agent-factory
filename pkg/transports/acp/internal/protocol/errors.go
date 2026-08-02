package protocol

import (
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

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
// unsupported method. It carries only the method name the client sent in
// its own request — never request parameters or internal state — matching
// the vendored SDK's own method-not-found shape.
func MethodNotFound(method string) *acpsdk.RequestError {
	return acpsdk.NewMethodNotFound(method)
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
