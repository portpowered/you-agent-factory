package wire_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestNewHostedPollersConstructsOwner(t *testing.T) {
	t.Parallel()

	checkpoints, err := hostedsourceswire.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}

	service := hostedsourceswire.NewHostedPollers(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedsourceswire.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		checkpoints,
	)
	if service == nil {
		t.Fatal("NewHostedPollers returned nil")
	}
	var _ hostedsources.HostedPollers = service
}

func TestNewCheckpointStoreRejectsMissingFilesystem(t *testing.T) {
	t.Parallel()

	if _, err := hostedsourceswire.NewCheckpointStore(nil); err == nil {
		t.Fatal("NewCheckpointStore(nil) error = nil, want required-filesystem failure")
	}
}

func TestNewSecretResolverRejectsMissingEnvironmentReader(t *testing.T) {
	t.Parallel()

	resolver := hostedsourceswire.NewSecretResolver(nil, func(string) ([]byte, error) {
		return nil, nil
	})
	if _, err := resolver(context.Background(), nil, "linear-api-key"); err == nil {
		t.Fatal("NewSecretResolver(nil, reader) error = nil, want required-environment-reader failure")
	}
}

func TestHostedPollersValidateLinearPollerDelegatesToService(t *testing.T) {
	t.Parallel()

	checkpoints, err := hostedsourceswire.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}
	service := hostedsourceswire.NewHostedPollers(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedsourceswire.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		checkpoints,
	)

	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker := &interfaces.FactoryWorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
	workstation := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: worker.Name,
	}

	if err := service.ValidateLinearPoller(
		runtimeCfg,
		workstation,
		worker,
		func(context.Context, work.WorkRequest) error { return nil },
	); err != nil {
		t.Fatalf("ValidateLinearPoller() error = %v", err)
	}
}
