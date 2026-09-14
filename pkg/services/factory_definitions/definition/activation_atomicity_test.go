package factorydefinition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	platformsnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorysnapshot "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorysnapshot"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type resultActivationGateway struct {
	*trackingActivationGateway
}

func (g *resultActivationGateway) ActivateSessionEditableFactoryWithResult(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) (factorydefinitions.DefinitionActivationResult, error) {
	if err := g.trackingActivationGateway.ActivateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		name,
		runtimeName,
	); err != nil {
		return factorydefinitions.DefinitionActivationResult{}, err
	}
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		return factorydefinitions.DefinitionActivationResult{}, err
	}
	return factorydefinitions.DefinitionActivationResult{
		LoadedSource: loaded,
		Available:    true,
	}, nil
}

type resultAtomicSaveHost struct {
	*splitLayoutSaveHost
	captureErr       error
	loadErr          error
	failLoadAfter    int32
	loadCalls        atomic.Int32
	runtimeReadCalls atomic.Int32
	runtimeReadErr   error
}

func (h *resultAtomicSaveHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return snapshotsportabilityprepare.PrepareConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h *resultAtomicSaveHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	if h.captureErr != nil {
		return nil, h.captureErr
	}
	return platformsnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(factoryDir, factoryConfig, runtimeConfig, sourceDirectory, metadata)
}

func (h *resultAtomicSaveHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	call := h.loadCalls.Add(1)
	if h.failLoadAfter > 0 && call > h.failLoadAfter {
		return nil, h.loadErr
	}
	return h.splitLayoutSaveHost.LoadFactory(factoryDir, loader)
}

func (h *resultAtomicSaveHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	h.runtimeReadCalls.Add(1)
	return nil, h.runtimeReadErr
}

func TestSaveReplaceCurrent_PreparesResponseBeforeActivationFailure(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	initial := versionedNamedFactoryPayload(t, "root", 1, versionTime)
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("write initial factory: %v", err)
	}

	responseErr := errors.New("response snapshot unavailable")
	host := &resultAtomicSaveHost{
		splitLayoutSaveHost: &splitLayoutSaveHost{
			sessionRootDir: rootDir,
			current: factoryapi.Factory{
				Name: apisurface.DefaultCurrentFactoryName,
				Id:   saveStringPointer("root-runtime"),
				Version: &factoryapi.HybridLogicalTimestamp{
					Logical:  1,
					Physical: versionTime,
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	replacement := editableFactoryFromPayload(t, namedFactoryPayload(t, "replacement"))
	replacement.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	_, err := svc.SaveReplaceCurrentSnapshotForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		replacement,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("save error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.restoreCalled {
		t.Fatal("expected durable layout restore after response preparation failure")
	}
	got, readErr := os.ReadFile(initialPath)
	if readErr != nil {
		t.Fatalf("read restored factory: %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("restored factory = %q, want original %q", got, initial)
	}
}

func TestSaveReplaceCurrent_UsesPreparedResponseWithoutPostSwapVersionOrRuntimeReads(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if err := os.WriteFile(
		filepath.Join(rootDir, factorydefinitions.FactoryConfigFile),
		versionedNamedFactoryPayload(t, "root", 1, versionTime),
		0o644,
	); err != nil {
		t.Fatalf("write initial factory: %v", err)
	}
	host := &resultAtomicSaveHost{
		splitLayoutSaveHost: &splitLayoutSaveHost{
			sessionRootDir: rootDir,
			current: factoryapi.Factory{
				Name: apisurface.DefaultCurrentFactoryName,
				Id:   saveStringPointer("root-runtime"),
				Version: &factoryapi.HybridLogicalTimestamp{
					Logical:  1,
					Physical: versionTime,
				},
			},
		},
		failLoadAfter:  2,
		loadErr:        errors.New("post-swap version read must not happen"),
		runtimeReadErr: errors.New("post-swap runtime config read must not happen"),
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	replacement := editableFactoryFromPayload(t, namedFactoryPayload(t, "replacement"))
	replacement.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	saved, err := svc.SaveReplaceCurrentSnapshotForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		replacement,
	)
	if err != nil {
		t.Fatalf("SaveReplaceCurrentSnapshotForSession: %v", err)
	}
	if gateway.activateCalls.Load() != 1 {
		t.Fatalf("activation calls = %d, want 1", gateway.activateCalls.Load())
	}
	if host.loadCalls.Load() != 2 {
		t.Fatalf("host LoadFactory calls = %d, want current and preflight candidate only", host.loadCalls.Load())
	}
	if host.runtimeReadCalls.Load() != 0 {
		t.Fatalf("post-swap runtime config reads = %d, want 0", host.runtimeReadCalls.Load())
	}
	if saved == nil {
		t.Fatal("saved response is missing snapshot")
	}
	response, mapErr := factorySnapshotForCompatibilityTest(saved)
	if mapErr != nil || response.Version == nil {
		t.Fatalf("map saved response = %#v, %v; want version", response, mapErr)
	}
	if got := response.Version.Logical.Int64(); got != 2 {
		t.Fatalf("saved version logical = %d, want 2", got)
	}
	if !host.discardCalled {
		t.Fatal("expected successful replacement to discard its backup")
	}
}

type resultAtomicUpsertHost struct {
	*atomicUpsertSaveHost
	captureErr error
}

func (h *resultAtomicUpsertHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return snapshotsportabilityprepare.PrepareConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h *resultAtomicUpsertHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	if h.captureErr != nil {
		return nil, h.captureErr
	}
	return platformsnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(factoryDir, factoryConfig, runtimeConfig, sourceDirectory, metadata)
}

func TestSaveUpsertNamed_PreparesResponseBeforeActivationAndDiscardsCreatedCandidate(t *testing.T) {
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

	responseErr := errors.New("upsert response snapshot unavailable")
	host := &resultAtomicUpsertHost{
		atomicUpsertSaveHost: &atomicUpsertSaveHost{
			splitLayoutSaveHost: &splitLayoutSaveHost{
				sessionRootDir: rootDir,
				current: factoryapi.Factory{
					Name:    "alpha",
					Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	candidate := editableFactoryFromPayload(t, namedFactoryPayload(t, "beta"))

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		candidate,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("upsert error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.discardCalled {
		t.Fatal("expected created candidate discard after response preparation failure")
	}
	if got, readErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); readErr != nil || got != "alpha" {
		t.Fatalf("current pointer after failed upsert = %q, error=%v, want alpha", got, readErr)
	}
	if _, resolveErr := definitionTestNamedPaths.ResolveExistingDir(rootDir, "beta"); !errors.Is(resolveErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("failed candidate resolution error = %v, want named Factory not found", resolveErr)
	}
}

func TestSaveUpsertNamed_PreparesResponseBeforeActivationAndRestoresExistingLayout(t *testing.T) {
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

	responseErr := errors.New("existing upsert response unavailable")
	host := &resultAtomicUpsertHost{
		atomicUpsertSaveHost: &atomicUpsertSaveHost{
			splitLayoutSaveHost: &splitLayoutSaveHost{
				sessionRootDir: rootDir,
				current: factoryapi.Factory{
					Name:    "alpha",
					Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	candidate := editableFactoryFromPayload(t, namedFactoryPayload(t, "alpha"))
	candidate.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		candidate,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("upsert error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.restoreCalled {
		t.Fatal("expected existing layout restore after response preparation failure")
	}
	got, readErr := os.ReadFile(filepath.Join(rootDir, "alpha", factorydefinitions.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("read restored alpha: %v", readErr)
	}
	if !strings.Contains(string(got), `"logical": "1"`) && !strings.Contains(string(got), `"logical":1`) {
		t.Fatalf("restored alpha payload = %q, want version 1", got)
	}
	if pointer, pointerErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); pointerErr != nil || pointer != "alpha" {
		t.Fatalf("current pointer after failed replacement = %q, error=%v, want alpha", pointer, pointerErr)
	}
}
