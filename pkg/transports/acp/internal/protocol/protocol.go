// Package protocol implements the L1 V0 ACP JSON-RPC safety boundary:
// closed method support, bounded protocol-safe error classification, and a
// total deterministic stop-reason mapping. Every function here is pure —
// none perform I/O or invoke a downstream service — so a rejection can
// never leave a partial effect to undo. It is internal to
// pkg/transports/acp; callers use the package root's exported operations
// instead of this package directly.
package protocol

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// SupportedMethods is the closed set of L1 V0 JSON-RPC methods this
// transport implements. Any other method name is rejected as
// method-not-found before any validation or dispatch is attempted.
var SupportedMethods = map[string]bool{
	"initialize":                 true,
	"session/new":                true,
	"session/load":               true,
	"session/resume":             true,
	"session/cancel":             true,
	"session/set_config_option":  true,
	"session/prompt":             true,
	"session/update":             true,
	"session/request_permission": true,
}

// Guard evaluates a JSON-RPC method call against the closed method set and
// a caller-supplied validator before allowing effect to run. effect stands
// in for the inert transport-facing operation this method would perform (or,
// in a test, a probe). An unsupported method never calls validate or
// effect; a validate failure never calls effect. Only a supported method
// whose request validates successfully reaches effect, so a rejection here
// never has a side effect to undo.
func Guard(method string, validate func() error, effect func() error) error {
	if !SupportedMethods[method] {
		return MethodNotFound(method)
	}
	if err := validate(); err != nil {
		return SafeReject(err)
	}
	return effect()
}

// GuardEnvelope decodes raw JSON-RPC message bytes received on
// connectionID into an identity-bound envelope.Envelope before Guard ever
// runs. notificationSeq is forwarded to envelope.Decode unchanged: it is
// the connection/framing layer's responsibility to supply a value unique
// per notification received on this connection (see envelope.Decode), and
// GuardEnvelope has no state of its own to derive one from. A malformed
// envelope -- invalid JSON, valid JSON that is not a request object, a
// missing method, an unsupported id shape, or an id-bearing notification --
// is rejected here, before the method is looked up against SupportedMethods
// and before validate or effect is ever called, so a malformed message can
// never reach dispatch under a request identity that was never actually
// validated. A message with no id is always well-formed (a notification,
// regardless of method; see envelope.Decode) and is then dispatched exactly
// like Guard: an unsupported method never calls validate or effect, and a
// validate failure never calls effect.
//
// GuardEnvelope returns the decoded Envelope alongside the dispatch error
// so a caller can tell whether a response is ever owed for this message:
// per JSON-RPC 2.0, a notification (Envelope.IsNotification true) never
// receives a response -- success or error -- so a caller must discard the
// returned error for a notification rather than serializing it back to the
// connection. A message that never successfully decodes returns the zero
// Envelope, whose IsNotification is false, matching ordinary JSON-RPC
// server behavior: input the server cannot even classify still owes an
// error response.
func GuardEnvelope(connectionID identity.ConnectionID, notificationSeq uint64, raw json.RawMessage, validate func(envelope.Envelope) error, effect func() error) (envelope.Envelope, error) {
	env, err := envelope.Decode(connectionID, notificationSeq, raw)
	if err != nil {
		return envelope.Envelope{}, SafeReject(err)
	}
	dispatchErr := Guard(env.Method, func() error { return validate(env) }, effect)
	return env, dispatchErr
}
