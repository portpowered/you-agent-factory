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

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

type stubDefinitionHost struct {
	persistRootDir     string
	workstationLoader  factorydefinitions.WorkstationLoader
	currentRuntime     loadedFactorySource
	workflowID         string
	session            *interfaces.DefinitionSession
	sessionRuntime     loadedFactorySource
	sessionPersistRoot string
	requireSessionErr  error
	sessionRuntimeErr  error
}

func (h stubDefinitionHost) PersistRootDir() string { return h.persistRootDir }
func (h stubDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return h.workstationLoader
}
func (h stubDefinitionHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}
func (h stubDefinitionHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return definitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (h stubDefinitionHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return definitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}
func (h stubDefinitionHost) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return prepareFactoryLayoutForDefinitionTest(
		context.Background(),
		segment,
		payload,
		factoryvalidation.New(nil),
	)
}
func (h stubDefinitionHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}
func (h stubDefinitionHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	return definitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}
func (h stubDefinitionHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return portableconfig.Prepare(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}
func (h stubDefinitionHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorysnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}
func (h stubDefinitionHost) CurrentRuntimeConfig() loadedFactorySource {
	return h.currentRuntime
}
func (h stubDefinitionHost) WorkflowID() string { return h.workflowID }
func (h stubDefinitionHost) RequireSession(string) (*interfaces.DefinitionSession, error) {
	if h.requireSessionErr != nil {
		return nil, h.requireSessionErr
	}
	return h.session, nil
}
func (h stubDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	if h.sessionRuntimeErr != nil {
		return nil, h.sessionRuntimeErr
	}
	return h.sessionRuntime, nil
}
func (h stubDefinitionHost) SessionFactoryPersistRoot(*interfaces.DefinitionSession) string {
	return h.sessionPersistRoot
}
func (h stubDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *interfaces.FactorySnapshot) error {
	return validateDefinitionSnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (h stubDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*interfaces.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (h stubDefinitionHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (h stubDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h stubDefinitionHost) ActivateSessionEditableFactory(context.Context, *interfaces.DefinitionSession, string, string, string, string, string) error {
	return nil
}

func mustFactorySnapshot(factory factoryapi.Factory) *interfaces.FactorySnapshot {
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		panic(err)
	}
	return snapshot
}

func (h stubDefinitionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*interfaces.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (h stubDefinitionHost) SaveNow() time.Time {
	return time.Time{}
}

func (h stubDefinitionHost) RunSessionID() string { return "" }

func (h stubDefinitionHost) SessionForActivation(string) *interfaces.DefinitionSession {
	return nil
}

func (h stubDefinitionHost) NamedFactoryActivationPaths(*interfaces.DefinitionSession) (string, string) {
	return "", ""
}

func (h stubDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *interfaces.DefinitionSession) error {
	return nil
}

func (h stubDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *interfaces.DefinitionSession, string, string, string, string) error {
	return nil
}

func namedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}

func TestService_GetCurrentNamedFactory_ReadsPersistedPointerAndPayload(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", namedFactoryPayload(t, "alpha"), factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc := New(stubDefinitionHost{persistRootDir: rootDir})
	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	mapped, err := factorySnapshotForCompatibilityTest(current)
	if err != nil {
		t.Fatalf("map GetCurrentNamedFactory result: %v", err)
	}
	if mapped.Name != "alpha" {
		t.Fatalf("current factory name = %q, want alpha", mapped.Name)
	}
	if mapped.Id == nil || *mapped.Id != "alpha" {
		t.Fatalf("current factory id = %#v, want alpha", mapped.Id)
	}
}

func TestService_GetCurrentNamedFactory_ReturnsNotFoundWithoutRuntimeFallback(t *testing.T) {
	t.Parallel()

	svc := New(stubDefinitionHost{persistRootDir: t.TempDir()})
	_, err := svc.GetCurrentNamedFactory(context.Background())
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("GetCurrentNamedFactory error = %v, want %v", err, ErrCurrentFactoryNotFound)
	}
}

func TestService_GetCurrentNamedFactory_FallsBackToLiveRuntimeWhenPointerMissing(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	runtimeCfg, err := factorydefinitioncomposition.LoadCurrent(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	svc := New(stubDefinitionHost{
		persistRootDir: rootDir,
		currentRuntime: runtimeCfg,
	})
	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	mapped, err := factorySnapshotForCompatibilityTest(current)
	if err != nil {
		t.Fatalf("map GetCurrentNamedFactory result: %v", err)
	}
	if mapped.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", mapped.Name, apisurface.DefaultCurrentFactoryName)
	}
}

func TestService_CurrentFactoryDefinitionVersionAtRoot_UsesConfigVersion(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	payload := namedFactoryPayload(t, "alpha")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(23),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", versioned, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	got, err := New(stubDefinitionHost{}).CurrentFactoryDefinitionVersionAtRoot(rootDir, "alpha")
	if err != nil {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot: %v", err)
	}
	if got.Logical != 23 || !got.Physical.Equal(versionTime) {
		t.Fatalf("version = %#v, want logical=23 physical=%s", got, versionTime)
	}
}

func TestService_GetCurrentFactoryForSession_IncludesPersistedVersionForNamedPointer(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, time.June, 1, 10, 0, 0, 0, time.UTC)
	payload := namedFactoryPayload(t, "alpha")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(17),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", versioned, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	factoryDir, err := definitionTestNamedPaths.ResolveExistingDir(rootDir, "alpha")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDir(alpha): %v", err)
	}
	runtimeCfg, err := factorydefinitioncomposition.LoadCurrent(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(alpha): %v", err)
	}

	const sessionID = "session-alpha"
	host := stubDefinitionHost{
		persistRootDir: rootDir,
		session: &interfaces.DefinitionSession{
			ID:         sessionID,
			FactoryDir: rootDir,
			FolderPath: rootDir,
			IsDefault:  true,
		},
		sessionRuntime:     runtimeCfg,
		sessionPersistRoot: rootDir,
	}
	got, err := New(host).GetCurrentFactoryForSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("factory name = %q, want alpha", got.Name)
	}
	if got.Version == nil || got.Version.Logical != 17 || !got.Version.Physical.Equal(versionTime) {
		t.Fatalf("factory version = %#v, want logical=17 physical=%s", got.Version, versionTime)
	}
}

func TestService_CurrentFactoryDefinitionVersionAtRoot_UsesFileModTimeForDefaultFactory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	modTime := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.Chtimes(factoryPath, modTime, modTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, err := New(stubDefinitionHost{}, platformfilesystem.Local{}).CurrentFactoryDefinitionVersionAtRoot(rootDir, apisurface.DefaultCurrentFactoryName)
	if err != nil {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot: %v", err)
	}
	if got.Logical != modTime.UnixNano() || !got.Physical.Equal(modTime) {
		t.Fatalf("version = %#v, want logical=%d physical=%s", got, modTime.UnixNano(), modTime)
	}
}

func TestService_CurrentFactoryDefinitionVersionAtRoot_FailsClosedWithoutFileSystem(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	_, err := New(stubDefinitionHost{}).CurrentFactoryDefinitionVersionAtRoot(
		rootDir,
		apisurface.DefaultCurrentFactoryName,
	)
	if err == nil || !strings.Contains(err.Error(), "version filesystem is required") {
		t.Fatalf("error = %v, want missing version filesystem", err)
	}
}

type activateTrackingHost struct {
	stubDefinitionHost
	swappedName string
	sessionID   string
}

func (h *activateTrackingHost) RunSessionID() string { return "session-alpha" }

func (h *activateTrackingHost) SessionForActivation(string) *interfaces.DefinitionSession {
	return &interfaces.DefinitionSession{
		ID:         "session-alpha",
		FactoryDir: h.persistRootDir,
		FolderPath: h.persistRootDir,
	}
}

func (h *activateTrackingHost) NamedFactoryActivationPaths(*interfaces.DefinitionSession) (string, string) {
	return h.persistRootDir, h.persistRootDir
}

func (h *activateTrackingHost) SwapPersistedNamedFactoryRuntime(
	_ context.Context,
	sessionID string,
	_ *interfaces.DefinitionSession,
	_, _, _, name string,
) error {
	h.sessionID = sessionID
	h.swappedName = name
	return nil
}

func TestService_ActivateNamedFactory_SwapsPersistedNamedFactory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", namedFactoryPayload(t, "alpha"), factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	host := &activateTrackingHost{stubDefinitionHost: stubDefinitionHost{persistRootDir: rootDir}}
	if err := New(host).ActivateNamedFactory(context.Background(), "alpha"); err != nil {
		t.Fatalf("ActivateNamedFactory: %v", err)
	}
	if host.swappedName != "alpha" {
		t.Fatalf("swapped factory name = %q, want alpha", host.swappedName)
	}
	if host.sessionID != "session-alpha" {
		t.Fatalf("swap session id = %q, want session-alpha", host.sessionID)
	}
}

func TestService_ActivateNamedFactory_ReturnsResolveErrorForMissingFactory(t *testing.T) {
	t.Parallel()

	host := &activateTrackingHost{stubDefinitionHost: stubDefinitionHost{persistRootDir: t.TempDir()}}
	err := New(host).ActivateNamedFactory(context.Background(), "missing")
	if err == nil {
		t.Fatal("ActivateNamedFactory: expected error for missing named factory")
	}
	if !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("ActivateNamedFactory error = %v, want named factory not found", err)
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

func TestService_CurrentFactoryDefinitionVersionAtRoot_PropagatesNamedResolveError(t *testing.T) {
	t.Parallel()

	_, err := New(stubDefinitionHost{}).CurrentFactoryDefinitionVersionAtRoot(t.TempDir(), "missing")
	if !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot error = %v, want named factory not found", err)
	}
}
