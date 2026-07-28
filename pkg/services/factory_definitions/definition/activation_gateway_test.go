package factorydefinition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestEditableFactoryActivationUsesActivationGateway(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	host := &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name: apisurface.DefaultCurrentFactoryName,
			Id:   saveStringPointer("root-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	gateway := host.activationGateway()
	svc := newTestService(host, gateway)

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Name = apisurface.DefaultCurrentFactoryName
	factory.Id = saveStringPointer("root-runtime")
	factory.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  2,
		Physical: time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC),
	}

	if _, err := svc.SaveReplaceCurrentSnapshotForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		mustEditableFactoryForTest(t, factory),
	); err != nil {
		t.Fatalf("SaveReplaceCurrentSnapshotForSession: %v", err)
	}
	if gateway.activateCalls.Load() != 1 {
		t.Fatalf("ActivateSessionEditableFactory calls = %d, want 1", gateway.activateCalls.Load())
	}
	if gateway.activatedName != factorydefinitions.DefaultCurrentFactoryName {
		t.Fatalf("activated name = %q, want %q", gateway.activatedName, factorydefinitions.DefaultCurrentFactoryName)
	}
}

func TestNamedFactorySwapUsesActivationGatewayAndRejectsIdle(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if _, err := persistNamedFactoryForTest(
		rootDir,
		"named-target",
		namedFactoryPayload(t, "named-target"),
		factoryvalidation.New(nil),
	); err != nil {
		t.Fatalf("PersistNamedFactory(named-target): %v", err)
	}

	session := &factorydefinitions.DefinitionSession{
		ID:         "session-alpha",
		FactoryDir: rootDir,
		FolderPath: rootDir,
	}
	host := stubDefinitionHost{
		persistRootDir: rootDir,
		session:        session,
	}
	gateway := &trackingActivationGateway{
		runSessionID:         "session-alpha",
		sessionForActivation: session,
		persistRoot:          rootDir,
		folderPath:           rootDir,
		idleNamedErr:         errors.New("runtime busy"),
	}
	svc := newTestService(host, gateway)

	err := svc.ActivateNamedFactory(context.Background(), "named-target")
	if err == nil {
		t.Fatal("ActivateNamedFactory() error = nil, want idle rejection")
	}
	if gateway.swapCalls.Load() != 0 {
		t.Fatalf("SwapPersistedNamedFactoryRuntime calls = %d, want 0 before idle gate passes", gateway.swapCalls.Load())
	}

	gateway.idleNamedErr = nil
	if err := svc.ActivateNamedFactory(context.Background(), "named-target"); err != nil {
		t.Fatalf("ActivateNamedFactory after idle cleared: %v", err)
	}
	if gateway.swapCalls.Load() != 1 {
		t.Fatalf("SwapPersistedNamedFactoryRuntime calls = %d, want 1", gateway.swapCalls.Load())
	}
	if gateway.swappedName != "named-target" {
		t.Fatalf("swapped name = %q, want named-target", gateway.swappedName)
	}
}
