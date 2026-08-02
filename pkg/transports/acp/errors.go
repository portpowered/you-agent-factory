package acp

import acpsdk "github.com/coder/acp-go-sdk"

// CompatibilityErrorCode is the stable machine-readable failure code for ACP
// compatibility negotiation.
type CompatibilityErrorCode string

const (
	// CompatibilityErrorUnsupportedProtocolVersion reports that a requested
	// ACP protocol version is not SupportedProtocolVersion.
	CompatibilityErrorUnsupportedProtocolVersion CompatibilityErrorCode = "ACP_UNSUPPORTED_PROTOCOL_VERSION"
)

// CompatibilityError describes a sensitive-safe ACP compatibility failure.
// Its Message never includes anything beyond the numeric protocol versions
// already public in the ACP wire protocol.
type CompatibilityError struct {
	Code             CompatibilityErrorCode
	Message          string
	RequestedVersion acpsdk.ProtocolVersion
	SupportedVersion acpsdk.ProtocolVersion
}

func (e *CompatibilityError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// RequestIdentityErrorCode is the stable machine-readable failure code for
// ACP request-identity construction.
type RequestIdentityErrorCode string

const (
	// RequestIdentityErrorBlankConnectionID reports that a RequestIdentity
	// was requested with an empty or whitespace-only connection identity.
	RequestIdentityErrorBlankConnectionID RequestIdentityErrorCode = "ACP_BLANK_CONNECTION_ID"
	// RequestIdentityErrorInvalidWireID reports that a decoded JSON-RPC
	// request id was not the required string or numeric shape.
	RequestIdentityErrorInvalidWireID RequestIdentityErrorCode = "ACP_INVALID_WIRE_ID"
	// RequestIdentityErrorBlankMintedID reports that a transport-minted
	// RequestIdentity was requested with an empty or whitespace-only minted
	// id.
	RequestIdentityErrorBlankMintedID RequestIdentityErrorCode = "ACP_BLANK_MINTED_ID"
)

// RequestIdentityError describes a sensitive-safe failure to construct a
// RequestIdentity. Its Message never includes request parameter content.
type RequestIdentityError struct {
	Code    RequestIdentityErrorCode
	Message string
}

func (e *RequestIdentityError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
