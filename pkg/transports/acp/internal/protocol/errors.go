package protocol

import (
	"errors"
	"regexp"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// safeMethodNamePattern matches the shape of every JSON-RPC method name this
// protocol or its SDK could ever legitimately propose: an initial word,
// optionally followed by a single "/"-separated word (e.g. "initialize",
// "session/prompt"). A client-controlled method value that does not match
// this shape -- for example one carrying a credential, a filesystem path, a
// shell command, or another adversarial payload -- can never survive into
// MethodNotFound's serialized output.
var safeMethodNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(/[A-Za-z_][A-Za-z0-9_]*)?$`)

// redactedMethodPlaceholder replaces a method value that fails
// safeMethodNamePattern, so MethodNotFound always returns a bounded,
// sensitive-safe payload regardless of what a client sends as "method".
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
// unsupported method. It carries the client-sent method name only when that
// name matches safeMethodNamePattern; any other value -- including one
// carrying a credential, a filesystem path, a shell command, a tool/prompt
// payload fragment, or internal topology -- is replaced with
// redactedMethodPlaceholder before it ever reaches acpsdk.NewMethodNotFound,
// so the emitted error can never disclose more than a plausible method
// name.
func MethodNotFound(method string) *acpsdk.RequestError {
	safeMethod := method
	if !safeMethodNamePattern.MatchString(method) {
		safeMethod = redactedMethodPlaceholder
	}
	return acpsdk.NewMethodNotFound(safeMethod)
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
