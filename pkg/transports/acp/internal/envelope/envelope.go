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
// method, an unsupported id shape, an id-bearing notification, or a
// request-shaped method with no id. It is the sentinel every Decode failure
// wraps, so a caller can classify a malformed envelope distinctly from a
// method-specific validation failure.
var ErrMalformedEnvelope = errors.New("acp: malformed json-rpc envelope")

// jsonrpcVersion is the only JSON-RPC protocol version this boundary
// accepts on the wire.
const jsonrpcVersion = "2.0"

// NotificationMethods is the closed set of L1 V0 JSON-RPC methods that are
// notifications rather than requests. JSON-RPC 2.0 defines a notification
// as "a Request object without an 'id' member," and a server must never
// send a response -- success or error -- for one. session/cancel (client to
// agent) and session/update (agent to client) are both notifications in the
// ACP spec: neither has a response payload. Every other method this
// transport supports is a request that requires a response, and therefore
// requires an id.
var NotificationMethods = map[string]bool{
	"session/cancel": true,
	"session/update": true,
}

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

// Envelope is one inbound JSON-RPC message, already bound to the connection
// it arrived on: an unambiguous RequestIdentity, the method name, the raw,
// not-yet-validated params, and whether this message is a notification. It
// is produced only by Decode, which rejects a malformed wire envelope
// before an Envelope value ever exists, so an Envelope always carries a
// real identity, a non-empty method, and syntactically valid (if not yet
// semantically validated) params.
//
// IsNotification is always false for a decode failure's zero-value
// Envelope: an unparseable message still owes an ordinary JSON-RPC error
// response, matching common JSON-RPC server behavior for input the server
// cannot even classify. For a successfully decoded Envelope, IsNotification
// true means the sender never expects any response for this message --
// success or error -- and a caller must never serialize Guard's or
// GuardEnvelope's returned error back to the connection for it.
type Envelope struct {
	Identity       identity.RequestIdentity `json:"identity"`
	Method         string                   `json:"method"`
	Params         json.RawMessage          `json:"params,omitempty"`
	IsNotification bool                     `json:"isNotification"`
}

// Decode parses raw JSON-RPC 2.0 message bytes received on connectionID
// into an Envelope. It rejects -- wrapping ErrMalformedEnvelope -- invalid
// JSON, a wrong or missing "jsonrpc" version, a missing method, a blank
// connection id, any id shape identity.NewJSONRPCID does not accept, an
// id-bearing NotificationMethods message, and a non-notification method
// with no id, before ever producing an Envelope value. A notification
// carries no JSON-RPC id to correlate by, so its identity is instead minted
// from the connection, the method, and notificationSeq -- a value the
// connection/framing layer must supply as unique per notification received
// on this connection (for example a connection-local monotonic counter),
// since Decode itself has no state across calls and cannot otherwise tell
// two same-method notifications on one connection apart. Decode performs no
// IO and invokes no downstream validator or effect: a rejection here can
// never have a side effect to undo.
func Decode(connectionID identity.ConnectionID, notificationSeq uint64, raw json.RawMessage) (Envelope, error) {
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
	if strings.TrimSpace(string(connectionID)) == "" {
		return Envelope{}, fmt.Errorf("%w: connection id must not be empty", ErrMalformedEnvelope)
	}

	idPresent := w.ID != nil

	if NotificationMethods[w.Method] {
		if idPresent {
			return Envelope{}, fmt.Errorf("%w: %s is a notification and must not carry an id", ErrMalformedEnvelope, w.Method)
		}
		// connectionID and w.Method are already validated non-blank above,
		// so this formatted string can never be empty and NewMinted can
		// never fail here.
		mintedIdentity, _ := identity.NewMinted(fmt.Sprintf("%s|%s|%d", connectionID, w.Method, notificationSeq))
		return Envelope{Identity: mintedIdentity, Method: w.Method, Params: w.Params, IsNotification: true}, nil
	}

	if !idPresent {
		return Envelope{}, fmt.Errorf("%w: id is required for a request method", ErrMalformedEnvelope)
	}
	id, err := identity.NewJSONRPCID(w.ID)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformedEnvelope, err)
	}
	// connectionID is already validated non-blank above, and id is already
	// validated non-zero by NewJSONRPCID, so NewCorrelated can never fail
	// here.
	reqIdentity, _ := identity.NewCorrelated(connectionID, id)
	return Envelope{Identity: reqIdentity, Method: w.Method, Params: w.Params}, nil
}
