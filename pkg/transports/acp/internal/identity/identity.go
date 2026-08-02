// Package identity gives every inbound ACP JSON-RPC request an unambiguous
// identity. JSON-RPC ids are unique per connection only: two different
// stdio connections can both send an "id": 1 request. RequestIdentity binds
// a JSON-RPC id to the connection that sent it, or mints a transport-owned
// identity for correlation that has no JSON-RPC id of its own (for example
// a permission request the transport initiates toward the client). It is
// internal to pkg/transports/acp; callers use the package root's exported
// operations instead of this package directly.
package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ConnectionID identifies one JSON-RPC connection. The transport mints a
// distinct ConnectionID per connection; it is never reused within a
// process lifetime.
type ConnectionID string

// JSONRPCID is a JSON-RPC 2.0 request id: a JSON string or JSON number
// token. ACP requests always carry a non-null id, so the null variant JSON-
// RPC otherwise allows is not represented here.
type JSONRPCID struct {
	raw json.RawMessage
}

// NewJSONRPCID validates a raw JSON-RPC id value and wraps it. Only string
// and number JSON tokens are accepted; objects, arrays, booleans, and null
// are rejected, matching the JSON-RPC 2.0 id contract.
func NewJSONRPCID(raw json.RawMessage) (JSONRPCID, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return JSONRPCID{}, errors.New("acp: json-rpc id must not be empty")
	}

	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return JSONRPCID{}, fmt.Errorf("acp: invalid string json-rpc id: %w", err)
		}
	case '{', '[', 't', 'f', 'n':
		return JSONRPCID{}, errors.New("acp: json-rpc id must be a string or number")
	default:
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			return JSONRPCID{}, fmt.Errorf("acp: invalid numeric json-rpc id: %w", err)
		}
		if dec.More() {
			return JSONRPCID{}, errors.New("acp: json-rpc id contains trailing content")
		}
	}

	canonical := make(json.RawMessage, len(trimmed))
	copy(canonical, trimmed)
	return JSONRPCID{raw: canonical}, nil
}

// NewStringJSONRPCID wraps a string request id.
func NewStringJSONRPCID(s string) JSONRPCID {
	raw, _ := json.Marshal(s)
	return JSONRPCID{raw: raw}
}

// NewNumberJSONRPCID wraps an integer request id.
func NewNumberJSONRPCID(n int64) JSONRPCID {
	raw, _ := json.Marshal(n)
	return JSONRPCID{raw: raw}
}

func (id JSONRPCID) isZero() bool {
	return len(id.raw) == 0
}

func (id JSONRPCID) equal(other JSONRPCID) bool {
	return bytes.Equal(id.raw, other.raw)
}

// MarshalJSON writes the id back out as the JSON token it was parsed from.
func (id JSONRPCID) MarshalJSON() ([]byte, error) {
	if id.isZero() {
		return nil, errors.New("acp: json-rpc id has no value")
	}
	out := make(json.RawMessage, len(id.raw))
	copy(out, id.raw)
	return out, nil
}

// UnmarshalJSON parses and validates a JSON-RPC id token.
func (id *JSONRPCID) UnmarshalJSON(b []byte) error {
	parsed, err := NewJSONRPCID(b)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

type requestIdentityKind int

const (
	requestIdentityKindUnset requestIdentityKind = iota
	requestIdentityKindCorrelated
	requestIdentityKindMinted
)

// RequestIdentity distinguishes one inbound request from every other
// request the transport has ever seen, including requests that carry an
// equal JSON-RPC id from a different connection. It is either a
// (ConnectionID, JSONRPCID) pair or a transport-minted identity.
type RequestIdentity struct {
	kind         requestIdentityKind
	connectionID ConnectionID
	jsonrpcID    JSONRPCID
	minted       string
}

// NewCorrelated builds a RequestIdentity from a connection and the JSON-RPC
// id the client sent on that connection.
func NewCorrelated(connectionID ConnectionID, id JSONRPCID) (RequestIdentity, error) {
	if strings.TrimSpace(string(connectionID)) == "" {
		return RequestIdentity{}, errors.New("acp: connection id must not be empty")
	}
	if id.isZero() {
		return RequestIdentity{}, errors.New("acp: json-rpc id must not be empty")
	}
	return RequestIdentity{kind: requestIdentityKindCorrelated, connectionID: connectionID, jsonrpcID: id}, nil
}

// NewMinted builds a RequestIdentity from a transport-owned identifier for
// correlation that has no inbound JSON-RPC id of its own.
func NewMinted(id string) (RequestIdentity, error) {
	if strings.TrimSpace(id) == "" {
		return RequestIdentity{}, errors.New("acp: minted request identity must not be empty")
	}
	return RequestIdentity{kind: requestIdentityKindMinted, minted: id}, nil
}

// Equal reports whether two request identities refer to the same request.
// Two correlated identities are equal only when both the connection and the
// JSON-RPC id match, so the same JSON-RPC id received on different
// connections is never equal.
func (r RequestIdentity) Equal(other RequestIdentity) bool {
	if r.kind != other.kind {
		return false
	}
	switch r.kind {
	case requestIdentityKindCorrelated:
		return r.connectionID == other.connectionID && r.jsonrpcID.equal(other.jsonrpcID)
	case requestIdentityKindMinted:
		return r.minted == other.minted
	default:
		return false
	}
}

// ConnectionID returns the identity's connection id and true when the
// identity is correlated to a connection and JSON-RPC id.
func (r RequestIdentity) ConnectionID() (ConnectionID, bool) {
	if r.kind != requestIdentityKindCorrelated {
		return "", false
	}
	return r.connectionID, true
}

// IsMinted reports whether the identity is a transport-minted identifier
// rather than a connection-correlated one.
func (r RequestIdentity) IsMinted() bool {
	return r.kind == requestIdentityKindMinted
}

type requestIdentityWire struct {
	Kind         string          `json:"kind"`
	ConnectionID string          `json:"connectionId,omitempty"`
	JSONRPCID    json.RawMessage `json:"jsonrpcId,omitempty"`
	Minted       string          `json:"minted,omitempty"`
}

// MarshalJSON encodes the identity as a tagged wire value, letting a
// RequestIdentity round-trip through JSON without losing which variant it
// is or collapsing distinct connections onto the same encoded id.
func (r RequestIdentity) MarshalJSON() ([]byte, error) {
	switch r.kind {
	case requestIdentityKindCorrelated:
		return json.Marshal(requestIdentityWire{
			Kind:         "correlated",
			ConnectionID: string(r.connectionID),
			JSONRPCID:    r.jsonrpcID.raw,
		})
	case requestIdentityKindMinted:
		return json.Marshal(requestIdentityWire{Kind: "minted", Minted: r.minted})
	default:
		return nil, errors.New("acp: request identity has no variant set")
	}
}

// UnmarshalJSON decodes a tagged wire value produced by MarshalJSON,
// re-validating it through the same constructors used to build a
// RequestIdentity directly.
func (r *RequestIdentity) UnmarshalJSON(b []byte) error {
	var wire requestIdentityWire
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}

	switch wire.Kind {
	case "correlated":
		id, err := NewJSONRPCID(wire.JSONRPCID)
		if err != nil {
			return err
		}
		parsed, err := NewCorrelated(ConnectionID(wire.ConnectionID), id)
		if err != nil {
			return err
		}
		*r = parsed
	case "minted":
		parsed, err := NewMinted(wire.Minted)
		if err != nil {
			return err
		}
		*r = parsed
	default:
		return fmt.Errorf("acp: unsupported request identity kind %q", wire.Kind)
	}
	return nil
}
