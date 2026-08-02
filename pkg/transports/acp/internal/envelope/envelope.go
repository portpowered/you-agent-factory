// Package envelope binds an inbound JSON-RPC request to the connection it
// arrived on before any method-specific validation or dispatch runs. A bare
// method name and a bare params blob are not enough to safely route or
// correlate a request: the request also needs an unambiguous identity (see
// pkg/transports/acp/internal/identity), and a malformed wire envelope
// (invalid JSON, a missing method, or an unsupported id shape) must be
// rejected before any downstream validator or effect ever sees it. Envelope
// is the single decode step that produces all three -- identity, method,
// and params -- together, so no caller can validate or dispatch against a
// method/params pair that was never actually bound to a real request
// identity. It is internal to pkg/transports/acp; callers use the package
// root's exported operations instead of this package directly.
package envelope

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// ErrMalformedEnvelope marks a JSON-RPC request that fails wire-level
// validation: invalid JSON, a wrong or missing "jsonrpc" version, a missing
// method, or an unsupported id shape. It is the sentinel every Decode
// failure wraps, so a caller can classify a malformed envelope distinctly
// from a method-specific validation failure.
var ErrMalformedEnvelope = errors.New("acp: malformed json-rpc envelope")

// jsonrpcVersion is the only JSON-RPC protocol version this boundary
// accepts on the wire.
const jsonrpcVersion = "2.0"

// wireRequest is the raw JSON-RPC 2.0 request shape, decoded before any
// semantic validation. Params is left as json.RawMessage because its
// method-specific shape is validated later, in pkg/transports/acp/internal/
// session, not here.
type wireRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Envelope is one inbound JSON-RPC request, already bound to the connection
// it arrived on: an unambiguous RequestIdentity, the method name, and the
// raw, not-yet-validated params. It is produced only by Decode, which
// rejects a malformed wire envelope before an Envelope value ever exists,
// so an Envelope always carries a real identity, a non-empty method, and
// syntactically valid (if not yet semantically validated) params.
type Envelope struct {
	Identity identity.RequestIdentity `json:"identity"`
	Method   string                  `json:"method"`
	Params   json.RawMessage         `json:"params,omitempty"`
}

// Decode parses raw JSON-RPC 2.0 request bytes received on connectionID
// into an Envelope. It rejects -- wrapping ErrMalformedEnvelope -- invalid
// JSON, a wrong or missing "jsonrpc" version, a missing method, and any id
// shape identity.NewJSONRPCID does not accept, before ever producing an
// Envelope value. Decode performs no IO and invokes no downstream
// validator or effect: a rejection here can never have a side effect to
// undo.
func Decode(connectionID identity.ConnectionID, raw json.RawMessage) (Envelope, error) {
	var w wireRequest
	if err := json.Unmarshal(raw, &w); err != nil {
		return Envelope{}, fmt.Errorf("%w: invalid JSON: %v", ErrMalformedEnvelope, err)
	}
	if w.JSONRPC != jsonrpcVersion {
		return Envelope{}, fmt.Errorf("%w: jsonrpc must be %q", ErrMalformedEnvelope, jsonrpcVersion)
	}
	if strings.TrimSpace(w.Method) == "" {
		return Envelope{}, fmt.Errorf("%w: method is required", ErrMalformedEnvelope)
	}
	id, err := identity.NewJSONRPCID(w.ID)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformedEnvelope, err)
	}
	reqIdentity, err := identity.NewCorrelated(connectionID, id)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformedEnvelope, err)
	}
	return Envelope{Identity: reqIdentity, Method: w.Method, Params: w.Params}, nil
}
