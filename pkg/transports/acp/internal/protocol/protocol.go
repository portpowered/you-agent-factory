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

// GuardEnvelope decodes raw JSON-RPC request bytes received on
// connectionID into an identity-bound envelope.Envelope before Guard ever
// runs. A malformed envelope -- invalid JSON, a missing method, or an
// unsupported id shape -- is rejected here, before the method is looked up
// against SupportedMethods and before validate or effect is ever called, so
// a malformed request can never reach dispatch under a request identity
// that was never actually validated. A well-formed envelope is then
// dispatched exactly like Guard: an unsupported method never calls
// validate or effect, and a validate failure never calls effect.
func GuardEnvelope(connectionID identity.ConnectionID, raw json.RawMessage, validate func(envelope.Envelope) error, effect func() error) error {
	env, err := envelope.Decode(connectionID, raw)
	if err != nil {
		return SafeReject(err)
	}
	return Guard(env.Method, func() error { return validate(env) }, effect)
}
