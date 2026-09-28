package service

import (
	"slices"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestOpenCodeACPEnvironmentGrantsPermissionsByDefault(t *testing.T) {
	tests := []struct {
		name    string
		id      providers.ID
		request providers.ExecuteRequest
		want    string
	}{
		{name: "opencode default", id: providers.IDOpenCode, request: providers.ExecuteRequest{ProcessEnvironment: []string{"PATH=test-path"}}, want: `OPENCODE_PERMISSION={"*":"allow"}`},
		{name: "other ACP provider", id: "other"},
		{name: "explicit process setting", id: providers.IDOpenCode, request: providers.ExecuteRequest{ProcessEnvironment: []string{`opencode_permission={"*":"deny"}`}}, want: `opencode_permission={"*":"deny"}`},
		{name: "explicit request setting", id: providers.IDOpenCode, request: providers.ExecuteRequest{EnvVars: map[string]string{"OPENCODE_PERMISSION": `{"*":"ask"}`}}, want: `OPENCODE_PERMISSION={"*":"ask"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := requestEnvironment(test.id, test.request)
			if test.want == "" {
				if len(got) != 0 {
					t.Fatalf("environment = %#v, want no OpenCode override", got)
				}
				return
			}
			if !slices.Contains(got, test.want) {
				t.Fatalf("environment = %#v, want %q", got, test.want)
			}
			count := 0
			for _, value := range got {
				if strings.EqualFold(strings.SplitN(value, "=", 2)[0], "OPENCODE_PERMISSION") {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("environment = %#v, want one OpenCode permission setting", got)
			}
		})
	}
}

func TestOpenCodeACPEnvironmentPreservesInheritedEnvironment(t *testing.T) {
	t.Setenv("YOU_ACP_ENV_INHERIT_TEST", "inherited")
	got := requestEnvironment(providers.IDOpenCode, providers.ExecuteRequest{})
	if !slices.Contains(got, "YOU_ACP_ENV_INHERIT_TEST=inherited") {
		t.Fatalf("environment does not preserve inherited setting: %#v", got)
	}
	if !slices.Contains(got, `OPENCODE_PERMISSION={"*":"allow"}`) {
		t.Fatalf("environment does not grant OpenCode permissions: %#v", got)
	}
}
