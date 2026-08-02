package negotiation

import (
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

// richClientCapabilities declares every optional client capability this
// transport does not implement in L1 V0, so tests can prove the negotiated
// profile never claims them regardless of what the client offers.
func richClientCapabilities() acpsdk.ClientCapabilities {
	return acpsdk.ClientCapabilities{
		Auth:        acpsdk.AuthCapabilities{Terminal: true},
		Elicitation: &acpsdk.ElicitationCapabilities{},
		Fs: acpsdk.FileSystemCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Nes:               &acpsdk.ClientNesCapabilities{},
		PlanCapabilities:  &acpsdk.PlanCapabilities{},
		PositionEncodings: []acpsdk.PositionEncodingKind{"utf-8"},
		Terminal:          true,
	}
}

func TestNegotiateSupportedVersionAdvertisesHonestP0Profile(t *testing.T) {
	for name, clientCapabilities := range map[string]acpsdk.ClientCapabilities{
		"minimal client capabilities": {},
		"rich client capabilities":    richClientCapabilities(),
	} {
		t.Run(name, func(t *testing.T) {
			req := acpsdk.InitializeRequest{
				ProtocolVersion:    SupportedProtocolVersion,
				ClientCapabilities: clientCapabilities,
			}

			resp, err := Negotiate(req)
			if err != nil {
				t.Fatalf("Negotiate() unexpected error = %v", err)
			}

			if resp.ProtocolVersion != SupportedProtocolVersion {
				t.Errorf("ProtocolVersion = %v, want %v", resp.ProtocolVersion, SupportedProtocolVersion)
			}

			caps := resp.AgentCapabilities
			if !caps.LoadSession {
				t.Error("AgentCapabilities.LoadSession = false, want true")
			}
			if caps.SessionCapabilities.Close == nil {
				t.Error("AgentCapabilities.SessionCapabilities.Close = nil, want advertised")
			}
			if caps.SessionCapabilities.Resume == nil {
				t.Error("AgentCapabilities.SessionCapabilities.Resume = nil, want advertised")
			}

			// Deferred capabilities: never claimed, regardless of what the
			// client advertised.
			if caps.SessionCapabilities.List != nil {
				t.Error("AgentCapabilities.SessionCapabilities.List advertised, want deferred")
			}
			if caps.SessionCapabilities.Fork != nil {
				t.Error("AgentCapabilities.SessionCapabilities.Fork advertised, want deferred")
			}
			if caps.SessionCapabilities.Delete != nil {
				t.Error("AgentCapabilities.SessionCapabilities.Delete advertised, want deferred")
			}
			if caps.PromptCapabilities.Image || caps.PromptCapabilities.Audio || caps.PromptCapabilities.EmbeddedContext {
				t.Errorf("PromptCapabilities = %+v, want text-first only", caps.PromptCapabilities)
			}
			if caps.McpCapabilities.Acp || caps.McpCapabilities.Http || caps.McpCapabilities.Sse {
				t.Errorf("McpCapabilities = %+v, want none advertised", caps.McpCapabilities)
			}
			if caps.Auth.Logout != nil {
				t.Errorf("Auth = %+v, want none advertised", caps.Auth)
			}
			if len(resp.AuthMethods) != 0 {
				t.Errorf("AuthMethods = %v, want empty", resp.AuthMethods)
			}
			if caps.Providers != nil {
				t.Error("Providers advertised, want deferred")
			}
			if caps.Nes != nil {
				t.Error("Nes advertised, want deferred")
			}
		})
	}
}

func TestNegotiateRejectsUnsupportedOrMissingVersion(t *testing.T) {
	tests := map[string]acpsdk.ProtocolVersion{
		"missing version (zero value)": 0,
		"future version":               SupportedProtocolVersion + 1,
		"negative version":             -1,
	}

	for name, version := range tests {
		t.Run(name, func(t *testing.T) {
			req := acpsdk.InitializeRequest{ProtocolVersion: version}

			resp, err := Negotiate(req)
			if err == nil {
				t.Fatalf("Negotiate() error = nil, want rejection")
			}
			if resp.ProtocolVersion != 0 || resp.AgentCapabilities.LoadSession || len(resp.AuthMethods) != 0 {
				t.Errorf("Negotiate() result = %+v on rejection, want zero value", resp)
			}

			var reqErr *acpsdk.RequestError
			if !errors.As(err, &reqErr) {
				t.Fatalf("Negotiate() error type = %T, want *acpsdk.RequestError", err)
			}
			if reqErr.Code != -32602 {
				t.Errorf("RequestError.Code = %d, want -32602 (invalid params)", reqErr.Code)
			}
		})
	}
}

func TestNegotiateIsDeterministic(t *testing.T) {
	req := acpsdk.InitializeRequest{
		ProtocolVersion:    SupportedProtocolVersion,
		ClientCapabilities: richClientCapabilities(),
	}

	first, firstErr := Negotiate(req)
	second, secondErr := Negotiate(req)

	if firstErr != nil || secondErr != nil {
		t.Fatalf("Negotiate() errors = (%v, %v), want no error", firstErr, secondErr)
	}
	if first.ProtocolVersion != second.ProtocolVersion {
		t.Errorf("repeated negotiation produced different protocol versions: %v vs %v", first.ProtocolVersion, second.ProtocolVersion)
	}
	if first.AgentCapabilities.LoadSession != second.AgentCapabilities.LoadSession {
		t.Errorf("repeated negotiation produced different capability profiles")
	}
}
