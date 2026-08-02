package chatsessions

import "strings"

// JSONRPCIDKind distinguishes the supported JSON-RPC request-id wire shapes
// a RequestIdentity's JSON-RPC id was decoded from, so a numeric id and a
// string id with an identical printed form (e.g. 1 and "1") are never
// conflated. This is a transport-neutral counterpart to
// pkg/transports/acp's WireIDKind: the value shape mirrors it deliberately,
// but this package never imports the transport package, so decoding a wire
// id into this shape is a transport-owned boundary concern.
type JSONRPCIDKind string

const (
	// JSONRPCIDKindString marks a JSON-RPC id carried as a string.
	JSONRPCIDKindString JSONRPCIDKind = "STRING"
	// JSONRPCIDKindNumber marks a JSON-RPC id carried as a number.
	JSONRPCIDKindNumber JSONRPCIDKind = "NUMBER"
)

// RequestIdentity is the caller identity a Chat Sessions request carries so
// retries and race-safe control intents cannot rely on an ambiguous bare
// JSON-RPC id. Exactly one of two legal modes may be populated:
//
//   - connection-qualified: ConnectionID non-empty and a JSON-RPC id
//     populated through JSONRPCIDKind/JSONRPCIDString/JSONRPCIDNumber,
//     OpaqueID empty
//   - transport-minted: OpaqueID non-empty, ConnectionID and the JSON-RPC id
//     fields empty
//
// JSONRPCIDKind selects which of JSONRPCIDString and JSONRPCIDNumber is
// live, so a numeric id and a string id sharing a printed form are distinct
// identities.
type RequestIdentity struct {
	ConnectionID    string
	JSONRPCIDKind   JSONRPCIDKind
	JSONRPCIDString string
	JSONRPCIDNumber int64
	OpaqueID        string
}

// hasJSONRPCID reports whether identity carries a populated JSON-RPC id in
// exactly one of its two supported wire shapes. A numeric id is always
// considered populated once selected by JSONRPCIDKind, since 0 is a legal
// JSON-RPC numeric id with no analogous "blank" form.
func (identity RequestIdentity) hasJSONRPCID() bool {
	switch identity.JSONRPCIDKind {
	case JSONRPCIDKindString:
		return strings.TrimSpace(identity.JSONRPCIDString) != ""
	case JSONRPCIDKindNumber:
		return true
	default:
		return false
	}
}

// Validate reports whether identity matches exactly one legal identity mode.
// Bare JSON-RPC ids, bare connection ids, mixed identity modes, a populated
// field outside the active identity shape (a stray OpaqueID alongside a
// connection-qualified id, or a stray JSONRPCIDString/JSONRPCIDNumber that
// does not match the selected JSONRPCIDKind), an unrecognized JSONRPCIDKind,
// and empty identities are rejected. The returned error never carries the
// supplied identity values.
func (identity RequestIdentity) Validate() error {
	if identity.JSONRPCIDKind != "" && identity.JSONRPCIDKind != JSONRPCIDKindString && identity.JSONRPCIDKind != JSONRPCIDKindNumber {
		return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidJSONRPCIDKind}
	}

	stringPopulated := strings.TrimSpace(identity.JSONRPCIDString) != ""
	numberPopulated := identity.JSONRPCIDNumber != 0
	switch identity.JSONRPCIDKind {
	case JSONRPCIDKindString:
		if numberPopulated {
			return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidStrayField}
		}
	case JSONRPCIDKindNumber:
		if stringPopulated {
			return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidStrayField}
		}
	default:
		if stringPopulated || numberPopulated {
			return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidStrayField}
		}
	}

	hasConnection := strings.TrimSpace(identity.ConnectionID) != ""
	hasJSONRPCID := identity.hasJSONRPCID()
	hasOpaque := strings.TrimSpace(identity.OpaqueID) != ""

	switch {
	case hasOpaque && (hasConnection || hasJSONRPCID):
		return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidMixedIdentityModes}
	case hasConnection && hasJSONRPCID:
		return nil
	case hasOpaque:
		return nil
	case hasJSONRPCID:
		return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidBareJSONRPCID}
	case hasConnection:
		return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidIncompleteConnectionPair}
	default:
		return &InvalidRequestIdentityError{Reason: RequestIdentityInvalidEmpty}
	}
}
