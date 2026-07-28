package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestDefinitionsActivateNamedFactoryUsesSessionsGatewayIdleGate(t *testing.T) {
	t.Parallel()

	gateway := newActivationGatewayTestGateway(t)
	rootDir := t.TempDir()
	persistPeerNamedFactory(t, rootDir, "alpha", namedFactoryPeerPayload(t, "alpha"))

	svc := factorydefinition.New(peerIntegrationDefinitionHost{rootDir: rootDir}, gateway)
	err := svc.ActivateNamedFactory(context.Background(), "alpha")
	if err == nil {
		t.Fatal("ActivateNamedFactory() error = nil, want Sessions gateway idle rejection")
	}
	if gateway.RunSessionID() != factorysessions.DefaultSessionID {
		t.Fatalf("gateway RunSessionID() = %q, want %q", gateway.RunSessionID(), factorysessions.DefaultSessionID)
	}
}

type peerIntegrationDefinitionHost struct {
	rootDir string
}

func (h peerIntegrationDefinitionHost) PersistRootDir() string { return h.rootDir }
func (h peerIntegrationDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (h peerIntegrationDefinitionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}
func (h peerIntegrationDefinitionHost) WorkflowID() string { return "" }
func (h peerIntegrationDefinitionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{ID: "session-alpha", FactoryDir: h.rootDir, FolderPath: h.rootDir}, nil
}
func (h peerIntegrationDefinitionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, nil
}
func (h peerIntegrationDefinitionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return h.rootDir
}
func (h peerIntegrationDefinitionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (h peerIntegrationDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, errors.New("not implemented")
}
func (h peerIntegrationDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}
func (h peerIntegrationDefinitionHost) ResolveExistingFactoryDir(_ string, name string) (string, error) {
	factoryDir := filepath.Join(h.rootDir, name)
	if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
		return "", factorydefinitions.ErrNamedFactoryNotFound
	}
	return factoryDir, nil
}

func persistPeerNamedFactory(t *testing.T, rootDir, name string, payload []byte) {
	t.Helper()
	factoryDir := filepath.Join(rootDir, name)
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}

func namedFactoryPeerPayload(t *testing.T, name string) []byte {
	t.Helper()
	return []byte(`{"name":"` + name + `","id":"` + name + `-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
}
