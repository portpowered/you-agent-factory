package service

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientRequestPermissionUsesBoundBypassPolicy(t *testing.T) {
	t.Parallel()

	request := acpsdk.RequestPermissionRequest{Options: []acpsdk.PermissionOption{
		{OptionId: "allow", Kind: acpsdk.PermissionOptionKindAllowOnce},
		{OptionId: "deny", Kind: acpsdk.PermissionOptionKindRejectOnce},
	}}
	for _, test := range []struct {
		name         string
		skip         bool
		wantOptionID string
	}{
		{name: "bypass selects allow", skip: true, wantOptionID: "allow"},
		{name: "default selects deny", skip: false, wantOptionID: "deny"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &client{}
			client.reset(test.skip, nil)
			response, err := client.RequestPermission(context.Background(), request)
			if err != nil {
				t.Fatalf("RequestPermission() error = %v", err)
			}
			if response.Outcome.Selected == nil || string(response.Outcome.Selected.OptionId) != test.wantOptionID {
				t.Fatalf("selected option = %#v, want %q", response.Outcome.Selected, test.wantOptionID)
			}
		})
	}
}
