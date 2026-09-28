package service

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

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
