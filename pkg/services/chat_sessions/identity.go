package chatsessions

import "strings"

// RequestIdentity is the caller identity a Chat Sessions request carries so
// retries and race-safe control intents cannot rely on an ambiguous bare
// JSON-RPC id. Exactly one of two legal modes may be populated:
//
//   - connection-qualified: ConnectionID and JSONRPCID both non-empty,
//     OpaqueID empty
//   - transport-minted: OpaqueID non-empty, ConnectionID and JSONRPCID empty
type RequestIdentity struct {
	ConnectionID string
	JSONRPCID    string
	OpaqueID     string
}

// Validate reports whether identity matches exactly one legal identity mode.
// Bare JSON-RPC ids, bare connection ids, mixed identity modes, and empty
// identities are rejected. The returned error never carries the supplied
// identity values.
func (identity RequestIdentity) Validate() error {
	hasConnection := strings.TrimSpace(identity.ConnectionID) != ""
	hasJSONRPCID := strings.TrimSpace(identity.JSONRPCID) != ""
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
