package acp_test

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestNegotiateProtocolVersionAcceptsSupportedVersion(t *testing.T) {
	got, err := acp.NegotiateProtocolVersion(acp.SupportedProtocolVersion)
	if err != nil {
		t.Fatalf("NegotiateProtocolVersion() unexpected error: %v", err)
	}
	if got != acp.SupportedProtocolVersion {
		t.Fatalf("NegotiateProtocolVersion() = %d, want %d", got, acp.SupportedProtocolVersion)
	}
	if acp.SupportedProtocolVersion != acpsdk.ProtocolVersionNumber {
		t.Fatalf("SupportedProtocolVersion = %d, want acp-go-sdk pinned version %d", acp.SupportedProtocolVersion, acpsdk.ProtocolVersionNumber)
	}
}

func TestNegotiateProtocolVersionRejectsUnsupportedVersions(t *testing.T) {
	cases := []acpsdk.ProtocolVersion{0, 2, 99, -1}
	for _, requested := range cases {
		_, err := acp.NegotiateProtocolVersion(requested)
		if err == nil {
			t.Fatalf("NegotiateProtocolVersion(%d) expected error, got nil", requested)
		}
		var compatErr *acp.CompatibilityError
		if !errors.As(err, &compatErr) {
			t.Fatalf("NegotiateProtocolVersion(%d) error type = %T, want *acp.CompatibilityError", requested, err)
		}
		if compatErr.Code != acp.CompatibilityErrorUnsupportedProtocolVersion {
			t.Fatalf("NegotiateProtocolVersion(%d) code = %q, want %q", requested, compatErr.Code, acp.CompatibilityErrorUnsupportedProtocolVersion)
		}
		if compatErr.RequestedVersion != requested {
			t.Fatalf("NegotiateProtocolVersion(%d) RequestedVersion = %d, want %d", requested, compatErr.RequestedVersion, requested)
		}
		if compatErr.SupportedVersion != acp.SupportedProtocolVersion {
			t.Fatalf("NegotiateProtocolVersion(%d) SupportedVersion = %d, want %d", requested, compatErr.SupportedVersion, acp.SupportedProtocolVersion)
		}
	}
}

func TestBuildProfileRejectsUnsupportedVersion(t *testing.T) {
	_, err := acp.BuildProfile(acpsdk.ProtocolVersion(2))
	if err == nil {
		t.Fatal("BuildProfile(2) expected error, got nil")
	}
	var compatErr *acp.CompatibilityError
	if !errors.As(err, &compatErr) {
		t.Fatalf("BuildProfile(2) error type = %T, want *acp.CompatibilityError", err)
	}
}

func TestBuildProfileAcceptsSupportedVersion(t *testing.T) {
	profile, err := acp.BuildProfile(acp.SupportedProtocolVersion)
	if err != nil {
		t.Fatalf("BuildProfile() unexpected error: %v", err)
	}
	if profile.ProtocolVersion != acp.SupportedProtocolVersion {
		t.Fatalf("BuildProfile() ProtocolVersion = %d, want %d", profile.ProtocolVersion, acp.SupportedProtocolVersion)
	}
}

// TestTextFirstAgentCapabilitiesSerializedShape locks the exact serialized
// capability claims so a zero-value SDK field change or an accidental
// capability addition cannot silently widen what V0 advertises.
func TestTextFirstAgentCapabilitiesSerializedShape(t *testing.T) {
	capabilities := acp.TextFirstAgentCapabilities()

	data, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatalf("json.Marshal(capabilities) unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(capabilities) unexpected error: %v", err)
	}

	prompt, ok := decoded["promptCapabilities"].(map[string]any)
	if ok {
		for _, key := range []string{"audio", "embeddedContext", "image"} {
			if v, present := prompt[key]; present && v != false {
				t.Fatalf("promptCapabilities.%s = %v, want false or absent", key, v)
			}
		}
	}

	if mcp, ok := decoded["mcpCapabilities"].(map[string]any); ok {
		for _, key := range []string{"acp", "http", "sse"} {
			if v, present := mcp[key]; present && v != false {
				t.Fatalf("mcpCapabilities.%s = %v, want false or absent", key, v)
			}
		}
	}

	if _, present := decoded["loadSession"]; present {
		t.Fatalf("loadSession must be absent (zero value), got %v", decoded["loadSession"])
	}

	if session, ok := decoded["sessionCapabilities"].(map[string]any); ok {
		for _, key := range []string{"fork", "close", "delete", "list", "resume", "additionalDirectories"} {
			if v, present := session[key]; present {
				t.Fatalf("sessionCapabilities.%s must be absent, got %v", key, v)
			}
		}
	}

	if auth, ok := decoded["auth"].(map[string]any); ok {
		if v, present := auth["logout"]; present {
			t.Fatalf("auth.logout must be absent, got %v", v)
		}
	}

	if _, present := decoded["providers"]; present {
		t.Fatalf("providers capability must be absent, got %v", decoded["providers"])
	}
	if _, present := decoded["nes"]; present {
		t.Fatalf("nes capability must be absent, got %v", decoded["nes"])
	}
}

func TestTextFirstAgentCapabilitiesRoundTripsThroughSDKType(t *testing.T) {
	capabilities := acp.TextFirstAgentCapabilities()

	data, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatalf("json.Marshal(capabilities) unexpected error: %v", err)
	}

	var decoded acpsdk.AgentCapabilities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(capabilities) unexpected error: %v", err)
	}

	if decoded.PromptCapabilities.Image || decoded.PromptCapabilities.Audio || decoded.PromptCapabilities.EmbeddedContext {
		t.Fatalf("decoded PromptCapabilities = %+v, want all false", decoded.PromptCapabilities)
	}
	if decoded.LoadSession {
		t.Fatal("decoded LoadSession = true, want false")
	}
	if decoded.SessionCapabilities.Fork != nil {
		t.Fatal("decoded SessionCapabilities.Fork is set, want nil")
	}
	if decoded.McpCapabilities.Acp || decoded.McpCapabilities.Http || decoded.McpCapabilities.Sse {
		t.Fatalf("decoded McpCapabilities = %+v, want all false", decoded.McpCapabilities)
	}
}
