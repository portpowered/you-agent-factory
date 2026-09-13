package factorydefinition

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSaveUpsertNamedAndActivate_RollsBackCreatedCandidateOnActivationFailure(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if _, err := persistNamedFactoryForTest(
		rootDir,
		"alpha",
		versionedNamedFactoryPayload(t, "alpha", 1, versionTime),
		factoryvalidation.New(nil),
	); err != nil {
		t.Fatalf("persist alpha: %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("write alpha pointer: %v", err)
	}

	activationErr := errors.New("controlled activation failure")
	host := &atomicUpsertSaveHost{splitLayoutSaveHost: &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name:    "alpha",
			Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
		},
		activateErr: activationErr,
	}}
	gateway := host.activationGateway()
	svc := newTestService(host, gateway)
	candidate := factoryFromPayload(t, namedFactoryPayload(t, "beta"))

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		mustEditableFactoryForTest(t, candidate),
	)
	if !errors.Is(err, activationErr) {
		t.Fatalf("upsert error = %v, want controlled activation failure", err)
	}
	if !host.discardCalled {
		t.Fatal("expected newly-created candidate to be discarded")
	}
	if got, readErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); readErr != nil || got != "alpha" {
		t.Fatalf("current pointer after failed candidate = %q, error=%v, want alpha", got, readErr)
	}
	if _, resolveErr := definitionTestNamedPaths.ResolveExistingDir(rootDir, "beta"); !errors.Is(resolveErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("failed candidate resolution error = %v, want named Factory not found", resolveErr)
	}
}

func TestSaveUpsertNamedAndActivate_RestoresExistingLayoutOnActivationFailure(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	initial := versionedNamedFactoryPayload(t, "alpha", 1, versionTime)
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", initial, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("persist alpha: %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("write alpha pointer: %v", err)
	}

	activationErr := errors.New("controlled activation failure")
	host := &atomicUpsertSaveHost{splitLayoutSaveHost: &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name:    "alpha",
			Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
		},
		activateErr: activationErr,
	}}
	gateway := host.activationGateway()
	svc := newTestService(host, gateway)
	candidate := factoryFromPayload(t, namedFactoryPayload(t, "alpha-updated"))
	candidate.Name = "alpha"
	candidate.Version = &factoryapi.HybridLogicalTimestamp{Logical: 2, Physical: versionTime.Add(time.Second)}

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		mustEditableFactoryForTest(t, candidate),
	)
	if !errors.Is(err, activationErr) {
		t.Fatalf("upsert error = %v, want controlled activation failure", err)
	}
	if !host.restoreCalled {
		t.Fatal("expected existing named layout to be restored")
	}
	if host.discardCalled {
		t.Fatal("existing named layout must not use candidate deletion")
	}
	got, readErr := os.ReadFile(filepath.Join(rootDir, "alpha", factorydefinitions.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("read restored alpha: %v", readErr)
	}
	if string(got) != string(initial) && !strings.Contains(string(got), `"logical": "1"`) {
		t.Fatalf("restored alpha payload = %s, want original version 1", got)
	}
	if pointer, readPointerErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); readPointerErr != nil || pointer != "alpha" {
		t.Fatalf("current pointer after failed replacement = %q, error=%v, want alpha", pointer, readPointerErr)
	}
}

func TestSaveUpsertNamedAndActivate_PersistenceFailurePreservesCurrentPointer(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", versionedNamedFactoryPayload(t, "alpha", 1, versionTime), factoryvalidation.New(nil)); err != nil {
		t.Fatalf("persist alpha: %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("write alpha pointer: %v", err)
	}

	persistErr := errors.New("controlled persistence failure")
	host := &atomicUpsertSaveHost{
		splitLayoutSaveHost: &splitLayoutSaveHost{
			sessionRootDir: rootDir,
			current: factoryapi.Factory{
				Name:    "alpha",
				Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
			},
		},
		persistErr: persistErr,
	}
	svc := newTestService(host, host.activationGateway())
	candidate := factoryFromPayload(t, namedFactoryPayload(t, "beta"))

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		mustEditableFactoryForTest(t, candidate),
	)
	if !errors.Is(err, persistErr) {
		t.Fatalf("upsert error = %v, want controlled persistence failure", err)
	}
	if got, readErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); readErr != nil || got != "alpha" {
		t.Fatalf("current pointer after persistence failure = %q, error=%v, want alpha", got, readErr)
	}
	if _, resolveErr := definitionTestNamedPaths.ResolveExistingDir(rootDir, "beta"); !errors.Is(resolveErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("candidate resolution after persistence failure = %v, want named Factory not found", resolveErr)
	}
}

const factoryapiSessionIDForTest = "~default"

type atomicUpsertSaveHost struct {
	*splitLayoutSaveHost
	persistErr    error
	discardCalled bool
}

func (h *atomicUpsertSaveHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	if h.persistErr != nil {
		return "", h.persistErr
	}
	return h.splitLayoutSaveHost.PersistNamedFactoryWithPrepared(rootDir, name, prepared)
}

func (h *atomicUpsertSaveHost) DiscardNamedFactory(rootDir, name string) error {
	h.discardCalled = true
	factoryDir := filepath.Join(rootDir, name)
	return os.RemoveAll(factoryDir)
}

func (h *atomicUpsertSaveHost) RemoveCurrentFactoryPointer(rootDir string) error {
	return definitionTestNamedPaths.RemoveCurrentPointer(rootDir)
}

func versionedNamedFactoryPayload(t *testing.T, name string, logical int64, physical time.Time) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(namedFactoryPayload(t, name), &document); err != nil {
		t.Fatalf("decode named Factory payload: %v", err)
	}
	document["version"] = map[string]any{
		"logical":  logical,
		"physical": physical.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode versioned named Factory payload: %v", err)
	}
	return payload
}

func factoryFromPayload(t *testing.T, payload []byte) factoryapi.Factory {
	t.Helper()
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode Factory payload: %v", err)
	}
	return factory
}
