package acp_test

// This file independently verifies the shared ACP L1 V0 conformance corpus
// (internal/testutil/acpconformance) against the real pkg/transports/acp
// outbound compatibility boundary. It imports no Providers package and calls
// none of the Providers-owned inbound mapper: the transport and provider
// directions are proven against the same corpus cases independently, per
// ACP-L1-V0-protocol-conformance-004.

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpconformance"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

// wireEnvelope decodes the transport-facing fields a corpus case's payload
// may carry beyond the bare ACP-SDK wire shape: connectionId and mintedId
// are this repo's transport-local correlation metadata, never part of the
// ACP JSON-RPC envelope itself.
type wireEnvelope struct {
	Method       string            `json:"method"`
	ID           *acpsdk.RequestId `json:"id"`
	ConnectionID string            `json:"connectionId"`
	MintedID     string            `json:"mintedId"`
	Params       json.RawMessage   `json:"params"`
}

func TestConformanceInitializeCasesMatchNegotiateProtocolVersion(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleInitialize)
	if len(cases) == 0 {
		t.Fatal("expected at least one initialize case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var req acpsdk.InitializeRequest
			if err := json.Unmarshal(c.Payload, &req); err != nil {
				t.Fatalf("payload does not parse as acpsdk.InitializeRequest: %v", err)
			}
			negotiated, err := acp.NegotiateProtocolVersion(req.ProtocolVersion)
			switch c.Facts.Outcome {
			case "accepted":
				if err != nil {
					t.Fatalf("NegotiateProtocolVersion(%d) unexpected error: %v", req.ProtocolVersion, err)
				}
				if fmt.Sprint(int(negotiated)) != c.Facts.Metadata["negotiated_version"] {
					t.Errorf("negotiated version = %d, want %s", negotiated, c.Facts.Metadata["negotiated_version"])
				}
			case "incompatible_version":
				if err == nil {
					t.Fatalf("NegotiateProtocolVersion(%d) expected error, got nil", req.ProtocolVersion)
				}
				var compatErr *acp.CompatibilityError
				if !errors.As(err, &compatErr) {
					t.Fatalf("NegotiateProtocolVersion(%d) error type = %T, want *acp.CompatibilityError", req.ProtocolVersion, err)
				}
				if fmt.Sprint(int(compatErr.RequestedVersion)) != c.Facts.Metadata["requested_version"] {
					t.Errorf("RequestedVersion = %d, want %s", compatErr.RequestedVersion, c.Facts.Metadata["requested_version"])
				}
				if fmt.Sprint(int(compatErr.SupportedVersion)) != c.Facts.Metadata["supported_version"] {
					t.Errorf("SupportedVersion = %d, want %s", compatErr.SupportedVersion, c.Facts.Metadata["supported_version"])
				}
			default:
				t.Fatalf("case %q has unhandled initialize outcome %q", c.ID, c.Facts.Outcome)
			}
		})
	}
}

func TestConformanceCapabilitiesCaseMatchesTextFirstAgentCapabilities(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleCapabilities)
	if len(cases) == 0 {
		t.Fatal("expected at least one capabilities case")
	}
	data, err := json.Marshal(acp.TextFirstAgentCapabilities())
	if err != nil {
		t.Fatalf("json.Marshal(TextFirstAgentCapabilities()) unexpected error: %v", err)
	}
	var got any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal actual capabilities: %v", err)
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var want any
			if err := json.Unmarshal(c.Payload, &want); err != nil {
				t.Fatalf("unmarshal case payload: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("TextFirstAgentCapabilities() serialized = %v, want case payload %v", got, want)
			}
		})
	}
}

func TestConformanceFactoryOptionCaseMatchesFactoryTargetOption(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleFactoryOption)
	if len(cases) == 0 {
		t.Fatal("expected at least one factory_option case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var wire acpsdk.SessionConfigOption
			if err := json.Unmarshal(c.Payload, &wire); err != nil {
				t.Fatalf("payload does not parse as acpsdk.SessionConfigOption: %v", err)
			}
			domain, err := acp.FactoryTargetOptionFromSessionConfigOption(wire)
			if err != nil {
				t.Fatalf("FactoryTargetOptionFromSessionConfigOption() unexpected error: %v", err)
			}
			if string(domain.CurrentValue) != c.Facts.Metadata["current_value"] {
				t.Errorf("CurrentValue = %q, want %q", domain.CurrentValue, c.Facts.Metadata["current_value"])
			}
			if fmt.Sprint(len(domain.Choices)) != c.Facts.Metadata["choice_count"] {
				t.Errorf("choice count = %d, want %s", len(domain.Choices), c.Facts.Metadata["choice_count"])
			}

			reencoded, err := domain.ToSessionConfigOption()
			if err != nil {
				t.Fatalf("ToSessionConfigOption() unexpected error: %v", err)
			}
			data, err := json.Marshal(reencoded)
			if err != nil {
				t.Fatalf("marshal re-encoded option: %v", err)
			}
			var got, want any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal re-encoded option: %v", err)
			}
			if err := json.Unmarshal(c.Payload, &want); err != nil {
				t.Fatalf("unmarshal case payload: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round-tripped FactoryTargetOption = %v, want case payload %v", got, want)
			}
		})
	}
}

func TestConformanceRequestIdentityCasesMatchConstructors(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleRequestIdentity)
	if len(cases) == 0 {
		t.Fatal("expected at least one request_identity case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var envelope wireEnvelope
			if err := json.Unmarshal(c.Payload, &envelope); err != nil {
				t.Fatalf("payload does not parse: %v", err)
			}
			wantConnectionID := c.Facts.Metadata["connection_id"]
			switch c.Facts.Outcome {
			case "string_id":
				if envelope.ID == nil || envelope.ID.Str == nil {
					t.Fatalf("case %q declares string_id but decoded id = %+v", c.ID, envelope.ID)
				}
				identity, err := acp.NewRequestIdentity(envelope.ConnectionID, *envelope.ID)
				if err != nil {
					t.Fatalf("NewRequestIdentity() unexpected error: %v", err)
				}
				if identity.Kind != acp.WireIDKindString || identity.StringID != string(*envelope.ID.Str) {
					t.Errorf("identity = %+v, want Kind=string StringID=%q", identity, *envelope.ID.Str)
				}
				if identity.ConnectionID != wantConnectionID {
					t.Errorf("identity.ConnectionID = %q, want %q", identity.ConnectionID, wantConnectionID)
				}
			case "number_id":
				if envelope.ID == nil || envelope.ID.Number == nil {
					t.Fatalf("case %q declares number_id but decoded id = %+v", c.ID, envelope.ID)
				}
				identity, err := acp.NewRequestIdentity(envelope.ConnectionID, *envelope.ID)
				if err != nil {
					t.Fatalf("NewRequestIdentity() unexpected error: %v", err)
				}
				if identity.Kind != acp.WireIDKindNumber || identity.NumberID != int64(*envelope.ID.Number) {
					t.Errorf("identity = %+v, want Kind=number NumberID=%d", identity, *envelope.ID.Number)
				}
				if identity.ConnectionID != wantConnectionID {
					t.Errorf("identity.ConnectionID = %q, want %q", identity.ConnectionID, wantConnectionID)
				}
			case "minted":
				if envelope.ID != nil {
					t.Fatalf("case %q declares minted (no wire id) but decoded id = %+v", c.ID, envelope.ID)
				}
				identity, err := acp.NewMintedRequestIdentity(envelope.ConnectionID, envelope.MintedID)
				if err != nil {
					t.Fatalf("NewMintedRequestIdentity() unexpected error: %v", err)
				}
				if identity.Kind != acp.WireIDKindMinted || identity.MintedID != envelope.MintedID {
					t.Errorf("identity = %+v, want Kind=minted MintedID=%q", identity, envelope.MintedID)
				}
				if identity.ConnectionID != wantConnectionID {
					t.Errorf("identity.ConnectionID = %q, want %q", identity.ConnectionID, wantConnectionID)
				}
			default:
				t.Fatalf("case %q has unhandled request_identity outcome %q", c.ID, c.Facts.Outcome)
			}
		})
	}
}

func TestConformanceMalformedInputCasesProduceTypedOutcomes(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleMalformedInput)
	if len(cases) == 0 {
		t.Fatal("expected at least one malformed_input case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			switch c.Facts.Outcome {
			case "invalid_params":
				var envelope wireEnvelope
				if err := json.Unmarshal(c.Payload, &envelope); err != nil {
					t.Fatalf("payload does not parse: %v", err)
				}
				var decodeErr *acpsdk.RequestError
				if c.Facts.Metadata["reason"] == "semantically_invalid_params" {
					// A syntactically valid params object that fails the real
					// acp-go-sdk request type's own Validate() (e.g. a
					// session/prompt request missing the required prompt
					// field) must decode through the actual supported
					// request type, not a generic map, to prove
					// DecodeMethodParams enforces semantic validity and not
					// only JSON structure.
					_, decodeErr = acp.DecodeMethodParams[acpsdk.PromptRequest](envelope.Params)
				} else {
					_, decodeErr = acp.DecodeMethodParams[map[string]any](envelope.Params)
				}
				if decodeErr == nil {
					t.Fatal("DecodeMethodParams() expected an invalid-params error, got nil")
				}
				if decodeErr.Code != -32602 {
					t.Errorf("DecodeMethodParams().Code = %d, want -32602", decodeErr.Code)
				}
			case "invalid_request":
				var envelope struct {
					ID acpsdk.RequestId `json:"id"`
				}
				if err := json.Unmarshal(c.Payload, &envelope); err == nil {
					t.Fatal("expected the case's request id to fail acpsdk.RequestId decoding, got nil error")
				}
			default:
				t.Fatalf("case %q has unhandled malformed_input outcome %q", c.ID, c.Facts.Outcome)
			}
		})
	}
}

func TestConformanceUnsupportedMethodCasesMatchDispatch(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleUnsupportedMethod)
	if len(cases) == 0 {
		t.Fatal("expected at least one unsupported_method case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var envelope wireEnvelope
			if err := json.Unmarshal(c.Payload, &envelope); err != nil {
				t.Fatalf("payload does not parse: %v", err)
			}
			if disposition, ok := c.Facts.Metadata["disposition"]; ok {
				got := acp.ClassifyMethod(envelope.Method)
				if string(got) != disposition {
					t.Errorf("ClassifyMethod(%q) = %q, want %q", envelope.Method, got, disposition)
				}
			}
			outcome := acp.UnsupportedMethodResponse(envelope.Method, envelope.ID)
			switch c.Facts.Outcome {
			case "method_not_found":
				if outcome == nil {
					t.Fatal("UnsupportedMethodResponse() = nil, want a method-not-found outcome")
				}
				if outcome.Error == nil || fmt.Sprint(outcome.Error.Code) != c.Facts.Metadata["error_code"] {
					t.Errorf("outcome.Error = %+v, want code %s", outcome.Error, c.Facts.Metadata["error_code"])
				}
				if envelope.ID == nil || outcome.ResponseID != *envelope.ID {
					t.Errorf("outcome.ResponseID = %+v, want %+v", outcome.ResponseID, envelope.ID)
				}
			case "no_response_emitted":
				if outcome != nil {
					t.Errorf("UnsupportedMethodResponse() = %+v, want nil (no response emitted)", outcome)
				}
			default:
				t.Fatalf("case %q has unhandled unsupported_method outcome %q", c.ID, c.Facts.Outcome)
			}
		})
	}
}

// TestConformanceRoundTripSessionUpdateCasesPreserveFactsAcrossEncodeDecode
// independently proves the declared round-trip subset survives an
// encode/decode cycle through the pinned acp-go-sdk type, from the transport
// side, without importing Providers internals or calling the inbound
// mapper (mirrors, but does not share code with, the corpus package's own
// self-consistency proof).
func TestConformanceRoundTripSessionUpdateCasesPreserveFactsAcrossEncodeDecode(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	roundTrip := corpus.RoundTripCases()
	if len(roundTrip) == 0 {
		t.Fatal("expected at least one round_trip case")
	}
	for _, c := range roundTrip {
		t.Run(c.ID, func(t *testing.T) {
			var update acpsdk.SessionUpdate
			if err := json.Unmarshal(c.Payload, &update); err != nil {
				t.Fatalf("payload does not parse as acpsdk.SessionUpdate: %v", err)
			}
			kind, itemID, text, metadata := transportSessionUpdateFacts(t, update)
			if kind != c.Facts.Kind {
				t.Errorf("decoded kind = %q, want %q", kind, c.Facts.Kind)
			}
			if c.Facts.ItemID != "" && itemID != c.Facts.ItemID {
				t.Errorf("decoded item id = %q, want %q", itemID, c.Facts.ItemID)
			}
			if c.Facts.Text != "" && text != c.Facts.Text {
				t.Errorf("decoded text = %q, want %q", text, c.Facts.Text)
			}
			for key, want := range c.Facts.Metadata {
				if metadata[key] != want {
					t.Errorf("decoded metadata[%q] = %q, want %q", key, metadata[key], want)
				}
			}

			reencoded, err := json.Marshal(update)
			if err != nil {
				t.Fatalf("re-marshal SessionUpdate: %v", err)
			}
			var redecoded acpsdk.SessionUpdate
			if err := json.Unmarshal(reencoded, &redecoded); err != nil {
				t.Fatalf("re-unmarshal SessionUpdate: %v", err)
			}
			reKind, reItemID, reText, reMetadata := transportSessionUpdateFacts(t, redecoded)
			if reKind != kind || reItemID != itemID || reText != text {
				t.Errorf(
					"round_trip case %q lost facts across an encode/decode cycle: got (%q,%q,%q), want (%q,%q,%q)",
					c.ID, reKind, reItemID, reText, kind, itemID, text,
				)
			}
			if !reflect.DeepEqual(reMetadata, metadata) {
				t.Errorf("round_trip case %q lost metadata across an encode/decode cycle: got %v, want %v", c.ID, reMetadata, metadata)
			}
		})
	}
}

// transportSessionUpdateFacts extracts a minimal (kind, itemID, text,
// metadata) tuple directly from a decoded acpsdk.SessionUpdate for the
// round-trip-eligible variants only. It deliberately duplicates none of the
// Providers-owned mapper's policy; it exists only to prove this corpus's
// round-trip subset is stable under the pinned SDK type from the transport
// side.
func transportSessionUpdateFacts(t testing.TB, update acpsdk.SessionUpdate) (kind, itemID, text string, metadata map[string]string) {
	t.Helper()
	switch {
	case update.AgentMessageChunk != nil:
		id := ""
		if update.AgentMessageChunk.MessageId != nil {
			id = *update.AgentMessageChunk.MessageId
		}
		txt := ""
		if update.AgentMessageChunk.Content.Text != nil {
			txt = update.AgentMessageChunk.Content.Text.Text
		}
		return "message", id, txt, nil
	case update.AgentThoughtChunk != nil:
		id := ""
		if update.AgentThoughtChunk.MessageId != nil {
			id = *update.AgentThoughtChunk.MessageId
		}
		txt := ""
		if update.AgentThoughtChunk.Content.Text != nil {
			txt = update.AgentThoughtChunk.Content.Text.Text
		}
		return "reasoning", id, txt, nil
	case update.UsageUpdate != nil:
		return "usage", "usage", "", map[string]string{
			"used_tokens": fmt.Sprint(update.UsageUpdate.Used),
			"max_tokens":  fmt.Sprint(update.UsageUpdate.Size),
		}
	case update.SessionInfoUpdate != nil:
		txt := ""
		if update.SessionInfoUpdate.Title != nil {
			txt = *update.SessionInfoUpdate.Title
		}
		return "session", "session", txt, nil
	default:
		t.Fatalf("transportSessionUpdateFacts: no round-trip-eligible SessionUpdate variant is set: %+v", update)
		return "", "", "", nil
	}
}
