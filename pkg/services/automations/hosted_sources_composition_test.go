package automations_test

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	"go.uber.org/zap"
)

func TestAutomationsRootReExportsHostedLinearEffects(t *testing.T) {
	t.Parallel()

	store, err := automations.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	if store == nil {
		t.Fatal("NewHostedLinearCheckpointStore() returned nil")
	}
	if automations.HostedLinearDefaultRequestTimeout != 30*time.Second {
		t.Fatalf(
			"HostedLinearDefaultRequestTimeout = %s, want %s",
			automations.HostedLinearDefaultRequestTimeout,
			30*time.Second,
		)
	}
}

func TestAutomationsHostedSourcesConstructsInertly(t *testing.T) {
	t.Parallel()

	service := hostedsourceswire.NewHostedPollers(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		nil,
		"",
	)
	if service == nil {
		t.Fatal("NewHostedPollers returned nil")
	}
}

func TestAutomationsServiceConstructsHostedSourcesOwnerInertly(t *testing.T) {
	t.Parallel()

	service := automationservice.NewService(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"hosted-sources-composition",
		"",
		nil,
		nil,
		nil,
	)
	if service == nil {
		t.Fatal("NewService returned nil")
	}

	store, err := automations.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	factory := automations.NewHostedSourcesFactory(store)
	pollers := factory(zap.NewNop(), clockwork.NewFakeClock(), nil, nil, "")
	if pollers == nil {
		t.Fatal("hosted-sources factory returned nil HostedPollers")
	}
}
