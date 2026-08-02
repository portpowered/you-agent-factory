package acp_test

import (
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestNegotiateInitializationSupportedVersion(t *testing.T) {
	resp, err := acp.NegotiateInitialization(acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
	})
	if err != nil {
		t.Fatalf("NegotiateInitialization() unexpected error = %v", err)
	}
	if resp.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		t.Errorf("ProtocolVersion = %v, want %v", resp.ProtocolVersion, acpsdk.ProtocolVersionNumber)
	}
	if !resp.AgentCapabilities.LoadSession {
		t.Error("AgentCapabilities.LoadSession = false, want true")
	}
}

func TestNegotiateInitializationUnsupportedVersion(t *testing.T) {
	_, err := acp.NegotiateInitialization(acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber + 1,
	})
	if err == nil {
		t.Fatal("NegotiateInitialization() error = nil, want rejection for unsupported protocol version")
	}
}
