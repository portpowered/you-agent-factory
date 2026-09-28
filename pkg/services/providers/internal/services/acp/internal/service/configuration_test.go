package service

import (
	"context"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestConfigureRetainsUnchangedDaemonAndReplacesChangedCommand(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.daemons["custom-acp"]

	if err := service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}); err != nil {
		t.Fatalf("Configure(unchanged) error = %v", err)
	}
	if service.daemons["custom-acp"] != original {
		t.Fatal("unchanged configuration replaced the live daemon")
	}

	if err := service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "replacement acp",
	}}); err != nil {
		t.Fatalf("Configure(replacement) error = %v", err)
	}
	if service.daemons["custom-acp"] == original {
		t.Fatal("changed command retained the old daemon")
	}
}

func TestOpenCodeEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		provider    providers.ID
		environment []string
		want        []string
	}{
		{
			name:        "OpenCode default preserves other entries",
			provider:    providers.IDOpenCode,
			environment: []string{"OTHER=value"},
			want:        []string{"OTHER=value", `OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:        "operator inline config is preserved case insensitively",
			provider:    providers.IDOpenCode,
			environment: []string{"OTHER=value", `opencode_config_content={"snapshots":true}`, "LAST=entry"},
			want:        []string{"OTHER=value", `opencode_config_content={"snapshots":true}`, "LAST=entry"},
		},
		{
			name:        "other provider unchanged",
			provider:    providers.IDCodex,
			environment: []string{"OTHER=value"},
			want:        []string{"OTHER=value"},
		},
		{
			name:     "nil OpenCode environment",
			provider: providers.IDOpenCode,
			want:     []string{`OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:        "empty OpenCode environment",
			provider:    providers.IDOpenCode,
			environment: []string{},
			want:        []string{`OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:     "nil other provider environment",
			provider: providers.IDCodex,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := openCodeEnvironment(test.provider, test.environment)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("openCodeEnvironment() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestConfigureRejectsMalformedReplacementWithoutChangingLiveSet(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.daemons["custom-acp"]

	err = service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "http", Command: "agent acp",
	}})
	if err == nil {
		t.Fatal("Configure(malformed) error = nil")
	}
	if service.daemons["custom-acp"] != original {
		t.Fatal("malformed replacement mutated the live daemon set")
	}
}
