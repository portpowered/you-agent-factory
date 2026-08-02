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
