package acp

import (
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
)

// WireIDKind distinguishes the supported JSON-RPC request-id wire shapes a
// RequestIdentity was built from, so a numeric id and a string id with an
// identical printed form (e.g. 1 and "1") are never conflated.
type WireIDKind string

const (
	// WireIDKindString marks a RequestIdentity built from a JSON-RPC string id.
	WireIDKindString WireIDKind = "string"
	// WireIDKindNumber marks a RequestIdentity built from a JSON-RPC numeric id.
	WireIDKindNumber WireIDKind = "number"
	// WireIDKindMinted marks a RequestIdentity built for a notification or
	// request with no usable wire id.
	WireIDKindMinted WireIDKind = "minted"
)

// RequestIdentity correlates one JSON-RPC request or notification to the
// connection it arrived on. JSON-RPC request ids are unique per connection
// only: two different connections may reuse the same id concurrently, so the
// bare wire id is never a safe correlation key by itself.
type RequestIdentity struct {
	ConnectionID string
	Kind         WireIDKind
	StringID     string
	NumberID     int64
	MintedID     string
}

// NewRequestIdentity builds a RequestIdentity from a non-empty connection
// identity and a decoded JSON-RPC request id that carries a supported string
// or numeric value. It rejects a blank connection identity and any other id
// shape -- including the JSON-RPC null id, which the protocol itself
// discourages for requests -- without returning a partially valid identity.
func NewRequestIdentity(connectionID string, wireID acpsdk.RequestId) (RequestIdentity, error) {
	if strings.TrimSpace(connectionID) == "" {
		return RequestIdentity{}, &RequestIdentityError{
			Code:    RequestIdentityErrorBlankConnectionID,
			Message: "acp: request identity requires a non-empty connection identity",
		}
	}
	switch {
	case wireID.Str != nil:
		return RequestIdentity{ConnectionID: connectionID, Kind: WireIDKindString, StringID: string(*wireID.Str)}, nil
	case wireID.Number != nil:
		return RequestIdentity{ConnectionID: connectionID, Kind: WireIDKindNumber, NumberID: int64(*wireID.Number)}, nil
	default:
		return RequestIdentity{}, &RequestIdentityError{
			Code:    RequestIdentityErrorInvalidWireID,
			Message: "acp: request identity requires a string or numeric JSON-RPC id",
		}
	}
}

// NewMintedRequestIdentity builds a transport-minted RequestIdentity for a
// notification or request with no usable wire id. mintedID must be supplied
// by the caller (e.g. a UUID) and is never derived from, nor confusable
// with, a JSON-RPC response id: a minted identity is only ever used for
// internal correlation and is never fabricated onto the wire as a response
// id.
func NewMintedRequestIdentity(connectionID, mintedID string) (RequestIdentity, error) {
	if strings.TrimSpace(connectionID) == "" {
		return RequestIdentity{}, &RequestIdentityError{
			Code:    RequestIdentityErrorBlankConnectionID,
			Message: "acp: request identity requires a non-empty connection identity",
		}
	}
	if strings.TrimSpace(mintedID) == "" {
		return RequestIdentity{}, &RequestIdentityError{
			Code:    RequestIdentityErrorBlankMintedID,
			Message: "acp: transport-minted request identity requires a non-empty minted id",
		}
	}
	return RequestIdentity{ConnectionID: connectionID, Kind: WireIDKindMinted, MintedID: mintedID}, nil
}
