package service

import (
	"context"
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
