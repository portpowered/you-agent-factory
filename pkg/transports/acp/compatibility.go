package acp

import (
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"
)

// SupportedProtocolVersion is the only ACP protocol version this V0
// compatibility boundary accepts. It matches the version implemented by the
// pinned github.com/coder/acp-go-sdk v0.13.5 dependency.
const SupportedProtocolVersion acpsdk.ProtocolVersion = acpsdk.ProtocolVersionNumber

// Profile is the deterministic V0 ACP agent-side compatibility result: the
// negotiated protocol version and the exact capabilities You implements.
type Profile struct {
	ProtocolVersion   acpsdk.ProtocolVersion
	AgentCapabilities acpsdk.AgentCapabilities
}

// NegotiateProtocolVersion accepts only SupportedProtocolVersion and returns
// a typed *CompatibilityError for any other requested version. It performs
// no IO and has no side effect.
func NegotiateProtocolVersion(requested acpsdk.ProtocolVersion) (acpsdk.ProtocolVersion, error) {
	if requested != SupportedProtocolVersion {
		return 0, &CompatibilityError{
			Code:             CompatibilityErrorUnsupportedProtocolVersion,
			Message:          fmt.Sprintf("acp: unsupported protocol version %d, only version %d is supported", int(requested), int(SupportedProtocolVersion)),
			RequestedVersion: requested,
			SupportedVersion: SupportedProtocolVersion,
		}
	}
	return SupportedProtocolVersion, nil
}

// TextFirstAgentCapabilities returns the exact V0 agent capability claims:
// text prompt content only. Every other advertised capability is left at
// its explicit false/absent value because V0 implements none of them:
//
//   - PromptCapabilities.Image/Audio/EmbeddedContext: no non-text prompt
//     content is accepted.
//   - McpCapabilities.Acp/Http/Sse: no client MCP server passthrough.
//   - Auth.Logout and InitializeResponse.AuthMethods (set by the caller):
//     no authentication capability.
//   - SessionCapabilities.Fork: session/fork is not implemented.
//   - LoadSession: session/load is not implemented in V0.
//
// Filesystem, terminal, and permission behavior are client-advertised or
// request-driven rather than agent capability flags; V0 never issues
// fs/*, terminal/*, or session/request_permission calls, so it advertises
// no capability that would invite a client to expect them.
func TextFirstAgentCapabilities() acpsdk.AgentCapabilities {
	return acpsdk.AgentCapabilities{
		Auth:        acpsdk.AgentAuthCapabilities{Logout: nil},
		LoadSession: false,
		McpCapabilities: acpsdk.McpCapabilities{
			Acp:  false,
			Http: false,
			Sse:  false,
		},
		PromptCapabilities: acpsdk.PromptCapabilities{
			Audio:           false,
			EmbeddedContext: false,
			Image:           false,
		},
		SessionCapabilities: acpsdk.SessionCapabilities{
			AdditionalDirectories: nil,
			Close:                 nil,
			Delete:                nil,
			Fork:                  nil,
			List:                  nil,
			Resume:                nil,
		},
	}
}

// BuildProfile constructs the inert V0 compatibility profile for a
// requested protocol version. Construction performs no IO, starts no
// goroutine or process, binds no endpoint, and creates or invokes no Chat or
// Factory Session.
func BuildProfile(requested acpsdk.ProtocolVersion) (Profile, error) {
	version, err := NegotiateProtocolVersion(requested)
	if err != nil {
		return Profile{}, err
	}
	return Profile{
		ProtocolVersion:   version,
		AgentCapabilities: TextFirstAgentCapabilities(),
	}, nil
}
