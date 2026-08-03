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

// ErrInvalidJSON marks a Decode failure caused by input that never parsed
// as JSON at all. It wraps ErrMalformedEnvelope, so a caller checking only
// for ErrMalformedEnvelope still matches, while a caller that needs the
// JSON-RPC 2.0 parse-error-vs-invalid-request distinction can check for
// this sentinel specifically.
var ErrInvalidJSON = fmt.Errorf("%w: invalid JSON", ErrMalformedEnvelope)

// ErrInvalidRequestShape marks a Decode failure caused by input that parsed
// as JSON but violates the JSON-RPC 2.0 request shape this transport
// requires: a wrong or missing "jsonrpc" version, a missing or blank
// method, an unsupported id shape, an id-bearing notification, or a
// request-shaped method with no id. It wraps ErrMalformedEnvelope for the
// same reason ErrInvalidJSON does.
var ErrInvalidRequestShape = fmt.Errorf("%w: invalid JSON-RPC request shape", ErrMalformedEnvelope)

// DecodeError classifies a Decode failure and, when possible, carries the
// JSON-RPC id token a caller may still correlate a rejection response to.
// Per JSON-RPC 2.0, a request that is otherwise malformed should still
// receive a response correlated to its id when that id token was itself
// syntactically valid; ID reports false when the inbound message had no id,
// or its id was itself malformed, or the failure was ErrInvalidJSON (input
// unparseable enough that no id could ever be recovered from it).
type DecodeError struct {
	cause error
	id    identity.JSONRPCID
	hasID bool
}

// Error returns the underlying cause's message.
func (e *DecodeError) Error() string { return e.cause.Error() }

// Unwrap exposes the underlying cause so errors.Is/errors.As can match
// ErrMalformedEnvelope, ErrInvalidJSON, or ErrInvalidRequestShape.
func (e *DecodeError) Unwrap() error { return e.cause }

// ID returns the JSON-RPC id to correlate a rejection response to, and true
// only when the inbound message's id token was itself syntactically valid.
func (e *DecodeError) ID() (identity.JSONRPCID, bool) { return e.id, e.hasID }

func newDecodeError(cause error, id identity.JSONRPCID, hasID bool) *DecodeError {
	return &DecodeError{cause: cause, id: id, hasID: hasID}
}

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
//
// NotificationMethods only governs the reverse violation: an id-bearing
// message for one of these methods is rejected as a malformed envelope,
// since a caller that attaches an id to a notification is expecting a
// response that will never come. Decode treats absence of an id as
// notification status for every method, known or not, per JSON-RPC 2.0 --
// see Decode's doc comment.
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
// into an Envelope. It rejects -- wrapping ErrMalformedEnvelope -- input
// that never parses as JSON at all, valid JSON that is not a JSON-RPC
// request object, a wrong or missing "jsonrpc" version, a missing method, a
// blank connection id, any id shape identity.NewJSONRPCID does not accept,
// and an id-bearing NotificationMethods message, before ever producing an
// Envelope value. Every rejection is returned as a *DecodeError:
// unparseable JSON wraps ErrInvalidJSON and never carries a recoverable id,
// while every other rejection wraps ErrInvalidRequestShape and carries the
// message's id when that id token was itself syntactically valid, even
// though some other part of the message was rejected -- per JSON-RPC 2.0's
// parse-error-vs-invalid-request distinction, only a message that never
// parsed as JSON at all is ErrInvalidJSON; syntactically valid JSON that is
// merely the wrong top-level shape (a scalar, an array, or an object with a
// field of the wrong type) is ErrInvalidRequestShape instead.
//
// A message with no "id" member is a notification per JSON-RPC 2.0,
// regardless of whether its method is one of NotificationMethods: Decode
// never rejects an unrecognized or not-yet-implemented method for want of
// an id, since JSON-RPC 2.0 defines notification status solely by id
// absence and a server must never send any response for one.
// NotificationMethods instead governs the one id-bearing case Decode does
// reject: an id attached to a method this transport already knows is a
// notification, which can never receive the response its id implies.
// A notification carries no JSON-RPC id to correlate by, so its identity is
// instead minted from the connection, the method, and notificationSeq -- a
// value the connection/framing layer must supply as unique per
// notification received on this connection (for example a connection-local
// monotonic counter), since Decode itself has no state across calls and
// cannot otherwise tell two same-method notifications on one connection
// apart. Decode performs no IO and invokes no downstream validator or
// effect: a rejection here can never have a side effect to undo.
func Decode(connectionID identity.ConnectionID, notificationSeq uint64, raw json.RawMessage) (Envelope, error) {
	var w wireRequest
	if err := json.Unmarshal(raw, &w); err != nil {
		if !json.Valid(raw) {
			return Envelope{}, newDecodeError(fmt.Errorf("%w: %v", ErrInvalidJSON, err), identity.JSONRPCID{}, false)
		}
		candidateID, candidateOK := recoverCandidateID(raw)
		return Envelope{}, newDecodeError(fmt.Errorf("%w: %v", ErrInvalidRequestShape, err), candidateID, candidateOK)
	}

	// candidateID is a best-effort recovery of the message's id, used to
	// correlate a rejection response for every failure below that isn't
	// ErrInvalidJSON. It is populated only when the id token itself is a
	// syntactically valid JSON-RPC id, regardless of what else about the
	// message is rejected.
	var candidateID identity.JSONRPCID
	candidateOK := false
	if w.ID != nil {
		if parsed, err := identity.NewJSONRPCID(w.ID); err == nil {
			candidateID, candidateOK = parsed, true
		}
	}

	if w.JSONRPC != jsonrpcVersion {
		return Envelope{}, newDecodeError(fmt.Errorf("%w: jsonrpc must be %q", ErrInvalidRequestShape, jsonrpcVersion), candidateID, candidateOK)
	}
	if strings.TrimSpace(w.Method) == "" {
		return Envelope{}, newDecodeError(fmt.Errorf("%w: method is required", ErrInvalidRequestShape), candidateID, candidateOK)
	}
	if strings.TrimSpace(string(connectionID)) == "" {
		return Envelope{}, newDecodeError(fmt.Errorf("%w: connection id must not be empty", ErrInvalidRequestShape), candidateID, candidateOK)
	}

	idPresent := w.ID != nil

	if !idPresent {
		// JSON-RPC 2.0 defines notification status solely by the absence of
		// an id: every method without one -- known, unrecognized, or not
		// yet implemented -- is a notification that must never receive a
		// response. connectionID and w.Method are already validated
		// non-blank above, so this formatted string can never be empty and
		// NewMinted can never fail here.
		mintedIdentity, _ := identity.NewMinted(fmt.Sprintf("%s|%s|%d", connectionID, w.Method, notificationSeq))
		return Envelope{Identity: mintedIdentity, Method: w.Method, Params: w.Params, IsNotification: true}, nil
	}

	if NotificationMethods[w.Method] {
		return Envelope{}, newDecodeError(fmt.Errorf("%w: %s is a notification and must not carry an id", ErrInvalidRequestShape, w.Method), candidateID, candidateOK)
	}

	id, err := identity.NewJSONRPCID(w.ID)
	if err != nil {
		return Envelope{}, newDecodeError(fmt.Errorf("%w: %v", ErrInvalidRequestShape, err), identity.JSONRPCID{}, false)
	}
	// connectionID is already validated non-blank above, and id is already
	// validated non-zero by NewJSONRPCID, so NewCorrelated can never fail
	// here.
	reqIdentity, _ := identity.NewCorrelated(connectionID, id)
	return Envelope{Identity: reqIdentity, Method: w.Method, Params: w.Params}, nil
}

// recoverCandidateID best-effort-recovers a JSON-RPC id from raw bytes that
// parsed as valid JSON but failed to unmarshal into wireRequest -- for
// example a top-level scalar or array, which has no "id" field at all, or
// an object whose "id" field is valid but some other field (such as
// "method") has the wrong type. It reports false whenever raw is not a JSON
// object, has no "id" member, or that member is not itself a syntactically
// valid JSON-RPC id.
func recoverCandidateID(raw json.RawMessage) (identity.JSONRPCID, bool) {
	var partial struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(raw, &partial); err != nil || partial.ID == nil {
		return identity.JSONRPCID{}, false
	}
	id, err := identity.NewJSONRPCID(partial.ID)
	if err != nil {
		return identity.JSONRPCID{}, false
	}
	return id, true
}
