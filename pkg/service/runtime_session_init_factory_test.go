package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryService_OpenFactorySessionFromFolder_ValidateOnlyReturnsInitsNewFactoryForEmptyReadableFolder(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate only, empty): %v", err)
	}
	if result == nil {
		t.Fatal("validate-only empty-folder result = nil, want initsNewFactory metadata")
	}
	if !result.InitsNewFactory {
		t.Fatalf("validate-only initsNewFactory = false, want true")
	}
	if result.FolderPath != emptyDir {
		t.Fatalf("validate-only folderPath = %q, want absolute %q", result.FolderPath, emptyDir)
	}
	if result.SessionID != "" {
		t.Fatalf("validate-only session id = %q, want none", result.SessionID)
	}
	if len(result.Targets) != 0 {
		t.Fatalf("validate-only targets = %#v, want none", result.Targets)
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("validate-only empty-folder mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySession_ValidateOnlyMapsInitsNewFactoryToAPIResponse(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	validateOnly := true
	response, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:   emptyDir,
		ValidateOnly: &validateOnly,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession(validate only, empty): %v", err)
	}
	if response.InitsNewFactory == nil || !*response.InitsNewFactory {
		t.Fatalf("response.initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
	if response.FolderPath == nil || *response.FolderPath != emptyDir {
		t.Fatalf("response.folderPath = %#v, want %q", response.FolderPath, emptyDir)
	}
	if response.Session != nil {
		t.Fatalf("response.session = %#v, want none", response.Session)
	}
	if response.Targets != nil {
		t.Fatalf("response.targets = %#v, want none", response.Targets)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ValidateOnlyRunnableFolderOmitsInitsNewFactory(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate only, runnable): %v", err)
	}
	if result == nil {
		t.Fatal("validate-only runnable result = nil, want targets")
	}
	if result.InitsNewFactory {
		t.Fatal("validate-only initsNewFactory = true, want false for runnable folder")
	}
	if len(result.Targets) == 0 {
		t.Fatalf("validate-only targets = %#v, want runnable targets", result.Targets)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryCreatesScaffoldAndOpensSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	emptyDir := filepath.Join(harness.rootDir, "new-factory")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(new-factory): %v", err)
	}

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, false, true)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(init new factory): %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("init-new-factory result = %#v, want session id", result)
	}

	factoryConfigPath := filepath.Join(emptyDir, interfaces.FactoryConfigFile)
	written, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if normalizeInitFactoryJSON(t, string(written)) != normalizeInitFactoryJSON(t, initcmd.DefaultFactoryJSON()) {
		t.Fatalf("written factory.json does not match embedded default scaffold")
	}
	processorWorkerPath := filepath.Join(emptyDir, interfaces.WorkersDir, "processor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(processorWorkerPath); err != nil {
		t.Fatalf("Stat(processor AGENTS.md): %v", err)
	}

	session := harness.requireSession(t, result.SessionID)
	if session.FolderPath != emptyDir {
		t.Fatalf("session folder path = %q, want %q", session.FolderPath, emptyDir)
	}
	if liveSessionHandle(session).runtime.dir != emptyDir {
		t.Fatalf("session runtime dir = %q, want %q", liveSessionHandle(session).runtime.dir, emptyDir)
	}
	if got := harness.svc.sessions.Count(); got != before+1 {
		t.Fatalf("live session count = %d, want %d", got, before+1)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryRejectsRunnableFolder(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, false, true); err == nil || !strings.Contains(err.Error(), "already exposes runnable factory targets") {
		t.Fatalf("OpenFactorySessionFromFolder(init on runnable folder) error = %v, want already-runnable failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, factorysessions.ValidationReasonNotRunnable, "folderPath")
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("init-new-factory rejection mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySession_InitNewFactoryMapsToAPIResponseAndListsSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "api-init")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(api-init): %v", err)
	}

	initNewFactory := true
	response, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:     emptyDir,
		InitNewFactory: &initNewFactory,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession(init new factory): %v", err)
	}
	if response.Session == nil || response.Session.Id == "" {
		t.Fatalf("response.session = %#v, want live session summary", response.Session)
	}
	if response.Session.FolderPath != emptyDir {
		t.Fatalf("response.session.folderPath = %q, want %q", response.Session.FolderPath, emptyDir)
	}

	listResponse, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	found := false
	for _, summary := range listResponse.Sessions {
		if summary.Id == response.Session.Id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListFactorySessions = %#v, want session %q", listResponse.Sessions, response.Session.Id)
	}
}

func TestFactoryService_OpenFactorySession_InitNewFactoryRejectsValidateOnlyCombination(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "conflict")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(conflict): %v", err)
	}

	validateOnly := true
	initNewFactory := true
	_, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:     emptyDir,
		ValidateOnly:   &validateOnly,
		InitNewFactory: &initNewFactory,
	})
	if err == nil || !strings.Contains(err.Error(), "initNewFactory cannot be combined with validateOnly") {
		t.Fatalf("OpenFactorySession(conflicting flags) error = %v, want mutual-exclusion failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, factorysessions.ValidationReasonRequired, "initNewFactory")
	}
}

func normalizeInitFactoryJSON(t *testing.T, raw string) string {
	t.Helper()
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("normalizeInitFactoryJSON unmarshal: %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("normalizeInitFactoryJSON marshal: %v", err)
	}
	return string(encoded)
}
