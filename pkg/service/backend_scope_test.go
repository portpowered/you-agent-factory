package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestEnsureServiceBackendScope_GeneratesAndPersistsBeforeServiceBuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())

	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if !systemconfig.IsLocalBackendScopeID(svc.cfg.BackendScopeID) {
		t.Fatalf("BackendScopeID = %q, want local-<uuid>", svc.cfg.BackendScopeID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != svc.cfg.BackendScopeID {
		t.Fatalf("persisted backendScopeID = %q, want %q", persisted.BackendScopeID, svc.cfg.BackendScopeID)
	}
}

func TestEnsureServiceBackendScope_ExplicitScopeSkipsPersistence(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)
	existing := "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	cfg := &FactoryServiceConfig{
		BackendScopeID:      existing,
		SystemConfigHomeDir: homeDir,
		Logger:              zap.NewNop(),
	}
	if err := ensureServiceBackendScope(cfg, cfg.Logger); err != nil {
		t.Fatalf("ensureServiceBackendScope: %v", err)
	}
	if cfg.BackendScopeID != existing {
		t.Fatalf("BackendScopeID = %q, want %q", cfg.BackendScopeID, existing)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatalf("expected no system config write when backend scope is explicit")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat: %v", err)
	}
}

func TestEnsureServiceBackendScope_ReplayModeSkipsPersistence(t *testing.T) {
	t.Parallel()

	cfg := &FactoryServiceConfig{
		ReplayPath: filepath.Join(t.TempDir(), "replay.json"),
		Logger:     zap.NewNop(),
	}
	if err := ensureServiceBackendScope(cfg, cfg.Logger); err != nil {
		t.Fatalf("ensureServiceBackendScope: %v", err)
	}
	if cfg.BackendScopeID != "" {
		t.Fatalf("BackendScopeID = %q, want empty in replay mode", cfg.BackendScopeID)
	}
}

func TestBuildFactoryService_ResolvesBackendScopeBeforeSessionIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    1,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.cfg.BackendScopeID == "" {
		t.Fatal("expected backend scope to be resolved before service build completes")
	}
	if got := factorySessionBackendScopeID(svc, nil); got != svc.cfg.BackendScopeID {
		t.Fatalf("factorySessionBackendScopeID = %q, want %q", got, svc.cfg.BackendScopeID)
	}
}

func TestBuildFactoryService_ReusesBackendScopeAcrossRestarts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)

	build := func() *FactoryService {
		t.Helper()
		svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
			Dir:                                     dir,
			Logger:                                  zap.NewNop(),
			SystemConfigHomeDir:                     homeDir,
			SkipBuiltInRunnerPrerequisiteValidation: true,
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}
		return svc
	}

	first := build()
	second := build()
	if first.cfg.BackendScopeID != second.cfg.BackendScopeID {
		t.Fatalf("restart backendScopeID = %q, want %q", second.cfg.BackendScopeID, first.cfg.BackendScopeID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != first.cfg.BackendScopeID {
		t.Fatalf("persisted backendScopeID = %q, want %q", persisted.BackendScopeID, first.cfg.BackendScopeID)
	}
}

func TestBuildFactoryService_ReusesConfiguredScopeFromSystemConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)
	existing := "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"`+existing+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.cfg.BackendScopeID != existing {
		t.Fatalf("BackendScopeID = %q, want configured %q", svc.cfg.BackendScopeID, existing)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != existing {
		t.Fatalf("persisted backendScopeID = %q, want unchanged %q", persisted.BackendScopeID, existing)
	}
}

func TestBuildFactoryService_BackendScopeStableAcrossSessionOperations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	scopeBefore := svc.cfg.BackendScopeID
	if scopeBefore == "" {
		t.Fatal("expected backend scope before session operations")
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime idle")

	preflight, err := svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if preflight.BackendScopeId == nil || *preflight.BackendScopeId != scopeBefore {
		t.Fatalf("preflight backendScopeId = %#v, want %q", preflight.BackendScopeId, scopeBefore)
	}

	session, err := svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Id != defaultFactorySessionID {
		t.Fatalf("session id = %q, want %q", session.Id, defaultFactorySessionID)
	}
	liveSession := svc.sessionByID(defaultFactorySessionID)
	if liveSession == nil {
		t.Fatal("expected default live session")
	}
	if got := factorySessionBackendScopeID(svc, liveSession); got != scopeBefore {
		t.Fatalf("session identity backend scope = %q, want %q", got, scopeBefore)
	}

	alphaDir := writeNamedFactoryFixture(t, dir, "alpha")
	if _, err := svc.openFactorySession(context.Background(), alphaDir); err != nil {
		t.Fatalf("openFactorySession: %v", err)
	}
	if got := svc.cfg.BackendScopeID; got != scopeBefore {
		t.Fatalf("backend scope after open = %q, want %q", got, scopeBefore)
	}

	_, err = svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession: %v", err)
	}
	if got := svc.cfg.BackendScopeID; got != scopeBefore {
		t.Fatalf("backend scope after save = %q, want %q", got, scopeBefore)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != scopeBefore {
		t.Fatalf("persisted backendScopeID = %q, want unchanged %q", persisted.BackendScopeID, scopeBefore)
	}

	cancelRun()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestBuildFactoryService_MalformedConfiguredScopeFailsStartup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"local-bad-value"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err == nil {
		t.Fatal("expected malformed backend scope startup failure")
	}
}

func TestBuildFactoryService_RestartPreservesBackendScopeThroughSessionIdentityAPI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("expected missing system config before first startup")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat: %v", err)
	}

	build := func() *FactoryService {
		t.Helper()
		svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
			Dir:                                     dir,
			RuntimeMode:                             interfaces.RuntimeModeService,
			Logger:                                  zap.NewNop(),
			SystemConfigHomeDir:                     homeDir,
			MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
			SkipBuiltInRunnerPrerequisiteValidation: true,
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}
		return svc
	}

	first := build()
	if !systemconfig.IsLocalBackendScopeID(first.cfg.BackendScopeID) {
		t.Fatalf("first BackendScopeID = %q, want local-<uuid>", first.cfg.BackendScopeID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile after first startup: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != first.cfg.BackendScopeID {
		t.Fatalf("persisted backendScopeID = %q, want %q", persisted.BackendScopeID, first.cfg.BackendScopeID)
	}

	second := build()
	if second.cfg.BackendScopeID != first.cfg.BackendScopeID {
		t.Fatalf("restart backendScopeID = %q, want %q", second.cfg.BackendScopeID, first.cfg.BackendScopeID)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- second.Run(runCtx)
	}()
	waitForSessionRuntimeStatus(t, second, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime idle")

	server := httptest.NewServer(api.NewServer(second, 0, zap.NewNop()).Handler())
	defer server.Close()

	session := getLiveFactorySessionForBackendScopeTest(t, server.URL, defaultFactorySessionID)
	if session.Runtime.StreamIdentity == nil {
		t.Fatal("streamIdentity = nil, want persisted backend scope identity")
	}
	if session.Runtime.StreamIdentity.BackendScopeID != first.cfg.BackendScopeID {
		t.Fatalf(
			"session streamIdentity.backendScopeID = %q, want persisted %q after restart",
			session.Runtime.StreamIdentity.BackendScopeID,
			first.cfg.BackendScopeID,
		)
	}

	cancelRun()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestBuildFactoryService_ExposesPersistedBackendScopeThroughSessionIdentitySurfaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	persistedScope := svc.cfg.BackendScopeID
	if persistedScope == "" {
		t.Fatal("expected persisted backend scope before session identity reads")
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime idle")

	server := httptest.NewServer(api.NewServer(svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	session := getLiveFactorySessionForBackendScopeTest(t, server.URL, defaultFactorySessionID)
	if session.Runtime.StreamIdentity == nil {
		t.Fatal("streamIdentity = nil, want persisted backend scope identity")
	}
	if session.Runtime.StreamIdentity.BackendScopeID != persistedScope {
		t.Fatalf("session streamIdentity.backendScopeID = %q, want persisted %q", session.Runtime.StreamIdentity.BackendScopeID, persistedScope)
	}
	if strings.TrimSpace(session.Runtime.StreamIdentity.StreamGenerationID) == "" {
		t.Fatal("session streamIdentity.streamGenerationID is empty")
	}

	preflight, err := svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if preflight.BackendScopeId == nil || *preflight.BackendScopeId != persistedScope {
		t.Fatalf("preflight backendScopeId = %#v, want persisted %q", preflight.BackendScopeId, persistedScope)
	}
	if preflight.StreamGenerationId == nil || *preflight.StreamGenerationId != session.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("preflight streamGenerationId = %#v, want session read %q", preflight.StreamGenerationId, session.Runtime.StreamIdentity.StreamGenerationID)
	}

	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	eventsRecorder := httptest.NewRecorder()
	eventsRequest := httptest.NewRequest(http.MethodGet, "/factory-sessions/"+defaultFactorySessionID+"/events", nil).WithContext(eventsCtx)
	cancelEvents()
	api.NewServer(svc, 0, zap.NewNop()).Handler().ServeHTTP(eventsRecorder, eventsRequest)
	if eventsRecorder.Code != http.StatusOK {
		t.Fatalf("GET events status = %d, want 200", eventsRecorder.Code)
	}
	if got := eventsRecorder.Header().Get("X-Factory-Session-Backend-Scope-Id"); got != persistedScope {
		t.Fatalf("event handshake backend scope = %q, want persisted %q", got, persistedScope)
	}
	if got := eventsRecorder.Header().Get("X-Factory-Session-Stream-Generation-Id"); got != session.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("event handshake stream generation = %q, want session read %q", got, session.Runtime.StreamIdentity.StreamGenerationID)
	}
}

func getLiveFactorySessionForBackendScopeTest(t *testing.T, serverURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s status = %d, want 200", sessionID, resp.StatusCode)
	}
	var session factoryapi.FactorySession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode factory session: %v", err)
	}
	return session
}
