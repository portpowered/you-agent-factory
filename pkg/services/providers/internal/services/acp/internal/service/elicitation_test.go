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

func TestClientRequestPermissionGrantsAdvertisedAllowChoice(t *testing.T) {
	tests := []struct {
		name    string
		options []acpsdk.PermissionOption
		want    string
		denied  bool
	}{
		{name: "prefer allow always regardless of option order", options: []acpsdk.PermissionOption{
			{OptionId: "deny", Kind: acpsdk.PermissionOptionKindRejectOnce},
			{OptionId: "once", Kind: acpsdk.PermissionOptionKindAllowOnce},
			{OptionId: "always", Kind: acpsdk.PermissionOptionKindAllowAlways},
		}, want: "always"},
		{name: "allow once fallback", options: []acpsdk.PermissionOption{
			{OptionId: "deny", Kind: acpsdk.PermissionOptionKindRejectAlways},
			{OptionId: "once", Kind: acpsdk.PermissionOptionKindAllowOnce},
		}, want: "once"},
		{name: "reject only", options: []acpsdk.PermissionOption{
			{OptionId: "deny", Kind: acpsdk.PermissionOptionKindRejectOnce},
		}, want: "deny", denied: true},
		{name: "unknown choice cancels", options: []acpsdk.PermissionOption{
			{OptionId: "unknown", Kind: "future_kind"},
		}, denied: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &client{}
			client.reset(nil)
			response, err := client.RequestPermission(context.Background(), acpsdk.RequestPermissionRequest{Options: test.options})
			if err != nil {
				t.Fatalf("RequestPermission() error = %v", err)
			}
			if test.want == "" {
				if response.Outcome.Cancelled == nil {
					t.Fatalf("outcome = %#v, want cancelled", response.Outcome)
				}
			} else if response.Outcome.Selected == nil || string(response.Outcome.Selected.OptionId) != test.want {
				t.Fatalf("outcome = %#v, want selected %q", response.Outcome, test.want)
			}
			if client.permissionDenied() != test.denied {
				t.Fatalf("permissionDenied() = %t, want %t", client.permissionDenied(), test.denied)
			}
		})
	}
}

func TestClientRequestPermissionCancelsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &client{}
	client.reset(nil)
	response, err := client.RequestPermission(ctx, acpsdk.RequestPermissionRequest{
		Options: []acpsdk.PermissionOption{{OptionId: "always", Kind: acpsdk.PermissionOptionKindAllowAlways}},
	})
	if err != nil || response.Outcome.Cancelled == nil {
		t.Fatalf("outcome = %#v, error = %v, want cancelled", response.Outcome, err)
	}
}
