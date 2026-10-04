package settingsresolution_test

import (
	"testing"

	settingsresolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/defaults"
)

func TestProviderBackendScopeIdentity(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, provider, kind, boundary, want string
	}{
		{"canonical", "codex", "host", "local", "provider-codex-host-local"},
		{"normalized", " CODEX ", " HOST ", " Local ", "provider-codex-host-local"},
		{"path boundary", "claude", "host", `C:\Users/operator:workspace|one two`, "provider-claude-host-c--users-operator-workspace-one-two"},
		{"missing facts", "", " ", "", "provider-unknown-unknown-unknown"},
		{"other boundary", "codex", "host", "remote", "provider-codex-host-remote"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := settingsresolution.DeriveProviderBackendScopeID(test.provider, test.kind, test.boundary); got != test.want {
				t.Fatalf("backend scope = %q, want %q", got, test.want)
			}
		})
	}
}
