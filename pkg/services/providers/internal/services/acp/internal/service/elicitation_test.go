package service

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientAdvertisesOnlyFormElicitation(t *testing.T) {
	capabilities := acpClientCapabilities()
	if capabilities.Elicitation == nil || capabilities.Elicitation.Form == nil || capabilities.Elicitation.Url != nil {
		t.Fatalf("elicitation capabilities = %#v, want form only", capabilities.Elicitation)
	}
}

func TestClientAutoAcceptsChoiceAndApprovalElicitation(t *testing.T) {
	request := acpsdk.UnstableCreateElicitationRequest{Form: &acpsdk.UnstableCreateElicitationForm{
		Mode: "form",
		RequestedSchema: acpsdk.UnstableElicitationSchema{
			Properties: map[string]any{
				"approved":   map[string]any{"type": "boolean", "default": false},
				"permission": map[string]any{"enum": []any{"Deny", "Approve"}},
				"strategy": map[string]any{"oneOf": []any{
					map[string]any{"const": "conservative"}, map[string]any{"const": "balanced"},
				}, "default": "balanced"},
				"comment": map[string]any{"type": "string"},
			},
			Required: []string{"approved", "permission", "strategy"},
		},
	}}
	response, err := (&client{}).UnstableCreateElicitation(context.Background(), request)
	if err != nil || response.Accept == nil || response.Accept.Action != "accept" {
		t.Fatalf("elicitation response = (%#v, %v), want accept", response, err)
	}
	content := response.Accept.Content
	if content["approved"] != true || content["permission"] != "Approve" || content["strategy"] != "balanced" {
		t.Fatalf("elicitation content = %#v, want affirmative approval and default preference", content)
	}
	if _, exists := content["comment"]; exists {
		t.Fatalf("elicitation invented free text: %#v", content)
	}
}

func TestClientCancelsUnsupportedElicitation(t *testing.T) {
	client := &client{}
	for _, test := range []struct {
		name    string
		ctx     context.Context
		request acpsdk.UnstableCreateElicitationRequest
	}{
		{name: "required free text", ctx: context.Background(), request: acpsdk.UnstableCreateElicitationRequest{Form: &acpsdk.UnstableCreateElicitationForm{
			Mode: "form", RequestedSchema: acpsdk.UnstableElicitationSchema{
				Properties: map[string]any{"credential": map[string]any{"type": "string"}}, Required: []string{"credential"},
			},
		}}},
		{name: "url", ctx: context.Background(), request: acpsdk.UnstableCreateElicitationRequest{Url: &acpsdk.UnstableCreateElicitationUrl{Mode: "url", Url: "https://example.com/auth"}}},
		{name: "cancelled context", ctx: cancelledElicitationContext(), request: acpsdk.UnstableCreateElicitationRequest{Form: &acpsdk.UnstableCreateElicitationForm{Mode: "form"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := client.UnstableCreateElicitation(test.ctx, test.request)
			if err != nil || response.Cancel == nil || response.Cancel.Action != "cancel" {
				t.Fatalf("elicitation response = (%#v, %v), want cancel", response, err)
			}
		})
	}
}

func cancelledElicitationContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
