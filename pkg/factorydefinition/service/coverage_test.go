package factorydefinition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func TestService_NilReceiverReturnsRequiredErrors(t *testing.T) {
	t.Parallel()

	var svc *Service
	if _, err := svc.GetCurrentNamedFactory(context.Background()); err == nil {
		t.Fatal("GetCurrentNamedFactory: expected error for nil service")
	}
	if _, err := svc.GetCurrentFactoryForSession(context.Background(), "session"); err == nil {
		t.Fatal("GetCurrentFactoryForSession: expected error for nil service")
	}
	if _, err := svc.CurrentFactoryDefinitionVersionAtRoot(t.TempDir(), "alpha"); err == nil {
		t.Fatal("CurrentFactoryDefinitionVersionAtRoot: expected error for nil service")
	}
	if _, err := svc.SerializeNamedFactory("alpha", nil, true); err == nil {
		t.Fatal("SerializeNamedFactory: expected error for nil service")
	}
	if _, err := svc.PrepareEditableFactoryPersistView("alpha", factoryapi.Factory{}); err == nil {
		t.Fatal("PrepareEditableFactoryPersistView: expected error for nil service")
	}
	if _, err := svc.PersistPayloadFromView(nil, factoryapi.HybridLogicalTimestamp{}); err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil service")
	}
	if _, err := svc.PreparePersistedFactoryPayload("alpha", factoryapi.Factory{}, factoryapi.HybridLogicalTimestamp{}); err == nil {
		t.Fatal("PreparePersistedFactoryPayload: expected error for nil service")
	}
	if err := svc.ValidateEditableFactoryTopology(factoryapi.Factory{}); err == nil {
		t.Fatal("ValidateEditableFactoryTopology: expected error for nil service")
	}
	if _, err := svc.SaveReplaceCurrentForSession(context.Background(), "session", factoryapi.Factory{}); err == nil {
		t.Fatal("SaveReplaceCurrentForSession: expected error for nil service")
	}
	if _, err := svc.SaveUpsertNamedAndActivateForSession(context.Background(), "session", factoryapi.Factory{}); err == nil {
		t.Fatal("SaveUpsertNamedAndActivateForSession: expected error for nil service")
	}
	if err := svc.ActivateNamedFactory(context.Background(), "alpha"); err == nil {
		t.Fatal("ActivateNamedFactory: expected error for nil service")
	}
}

func TestValidateUpsertNamedFactoryRequest_RejectsInvalidFactoryName(t *testing.T) {
	t.Parallel()

	err := New(stubDefinitionHost{}).ValidateUpsertNamedFactoryRequest(factoryapi.Factory{
		Name: "bad/name",
	})
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("ValidateUpsertNamedFactoryRequest error = %v, want invalid named factory name", err)
	}
}

func TestService_SerializeNamedFactoryUpsertResponse_ReturnsThinBundledFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	runtimeCfg, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	got, err := New(stubDefinitionHost{workflowID: "workflow-1"}).SerializeNamedFactoryUpsertResponse("alpha", runtimeCfg)
	if err != nil {
		t.Fatalf("SerializeNamedFactoryUpsertResponse: %v", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("factory name = %q, want alpha", got.Name)
	}
}

func TestService_GetCurrentFactoryForSession_PropagatesSessionLookupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("session missing")
	host := stubDefinitionHost{requireSessionErr: wantErr}
	_, err := New(host).GetCurrentFactoryForSession(context.Background(), "missing")
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetCurrentFactoryForSession error = %v, want %v", err, wantErr)
	}
}

func TestService_GetCurrentNamedFactory_PropagatesPointerReadError(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	pointerPath := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(pointerPath, []byte("alpha"), 0o000); err != nil {
		t.Fatalf("WriteFile pointer: %v", err)
	}

	_, err := New(stubDefinitionHost{persistRootDir: rootDir}).GetCurrentNamedFactory(context.Background())
	if err == nil {
		t.Fatal("GetCurrentNamedFactory: expected pointer read error")
	}
}

func TestSaveReplaceCurrentForSession_RejectsInvalidWritableCurrentName(t *testing.T) {
	t.Parallel()

	host := &splitLayoutSaveHost{
		sessionRootDir: t.TempDir(),
		current: factoryapi.Factory{
			Name: "bad/name",
		},
	}
	_, err := New(host).SaveReplaceCurrentForSession(context.Background(), factorysessions.DefaultSessionID, factoryapi.Factory{})
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("SaveReplaceCurrentForSession error = %v, want invalid named factory name", err)
	}
}

func TestService_CurrentFactoryDefinitionVersionAtRoot_PropagatesNamedResolveError(t *testing.T) {
	t.Parallel()

	_, err := New(stubDefinitionHost{}).CurrentFactoryDefinitionVersionAtRoot(t.TempDir(), "missing")
	if !factoryconfig.IsNamedFactoryNotFound(err) {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot error = %v, want named factory not found", err)
	}
}

func TestService_PersistPayloadFromView_RejectsNilView(t *testing.T) {
	t.Parallel()

	_, err := New(stubDefinitionHost{}).PersistPayloadFromView(nil, factoryapi.HybridLogicalTimestamp{
		Logical:  1,
		Physical: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil view")
	}
}
