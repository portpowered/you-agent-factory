// Package negotiation implements L1 V0 ACP initialize negotiation: pinned
// protocol version matching and the honest P0 text-first agent capability
// profile. It is internal to pkg/transports/acp; callers use the package
// root's exported operation instead of this package directly.
package negotiation

import (
	acpsdk "github.com/coder/acp-go-sdk"
)

// SupportedProtocolVersion is the single ACP protocol version this L1 V0
// transport negotiates. There is no supported range: any other requested
// version is rejected outright rather than downgraded or upgraded.
const SupportedProtocolVersion = acpsdk.ProtocolVersion(acpsdk.ProtocolVersionNumber)

// Negotiate evaluates a client's initialize request against the pinned
// protocol version and the transport's implemented P0 text-first capability
// profile. It is a pure function: it performs no I/O and calls no
// downstream service or effect, so a rejection here never has a side
// effect to undo.
func Negotiate(req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	if req.ProtocolVersion != SupportedProtocolVersion {
		return acpsdk.InitializeResponse{}, acpsdk.NewInvalidParams(map[string]any{
			"reason":           "unsupported_protocol_version",
			"requestedVersion": int(req.ProtocolVersion),
			"supportedVersion": int(SupportedProtocolVersion),
		})
	}

	return acpsdk.InitializeResponse{
		ProtocolVersion:   SupportedProtocolVersion,
		AgentCapabilities: p0AgentCapabilities(),
		AuthMethods:       []acpsdk.AuthMethod{},
	}, nil
}

// p0AgentCapabilities is the exhaustive, honest set of agent capabilities
// this transport implements. Every field left at its zero value is a
// deliberate L1 V0 scope cut (deferred: plans, filesystem callbacks,
// terminals, fork, authentication, client-supplied MCP servers, and
// image/audio/embedded-context prompt content), not an oversight, and it
// does not vary with what the client advertises: unsupported capabilities
// are never optimistically claimed just because a client could use them.
func p0AgentCapabilities() acpsdk.AgentCapabilities {
	return acpsdk.AgentCapabilities{
		LoadSession: true,
		SessionCapabilities: acpsdk.SessionCapabilities{
			Close:  &acpsdk.SessionCloseCapabilities{},
			Resume: &acpsdk.SessionResumeCapabilities{},
		},
	}
}
