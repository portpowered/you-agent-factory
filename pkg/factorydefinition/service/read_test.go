package factorydefinition

import (
	"context"
	"encoding/json"
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

type stubDefinitionHost struct {
	persistRootDir      string
	workstationLoader   factoryconfig.WorkstationLoader
	currentRuntime      *factoryconfig.LoadedFactoryConfig
	workflowID          string
	session             *factorysessions.LiveSession
	sessionRuntime      *factoryconfig.LoadedFactoryConfig
	sessionPersistRoot  string
	requireSessionErr   error
	sessionRuntimeErr   error
}

func (h stubDefinitionHost) PersistRootDir() string { return h.persistRootDir }
func (h stubDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	return h.workstationLoader
}
func (h stubDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return h.currentRuntime
}
func (h stubDefinitionHost) WorkflowID() string { return h.workflowID }
func (h stubDefinitionHost) RequireSession(string) (*factorysessions.LiveSession, error) {
	if h.requireSessionErr != nil {
		return nil, h.requireSessionErr
	}
	return h.session, nil
}
func (h stubDefinitionHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	if h.sessionRuntimeErr != nil {
		return nil, h.sessionRuntimeErr
	}
	return h.sessionRuntime, nil
}
func (h stubDefinitionHost) SessionFactoryPersistRoot(*factorysessions.LiveSession) string {
	return h.sessionPersistRoot
}

func (h stubDefinitionHost) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (h stubDefinitionHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (h stubDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h stubDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, factoryapi.FactoryName, string) error {
	return nil
}

func (h stubDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (h stubDefinitionHost) SaveNow() time.Time {
	return time.Time{}
}

func (h stubDefinitionHost) RunSessionID() string { return "" }

func (h stubDefinitionHost) SessionForActivation(string) *factorysessions.LiveSession {
	return nil
}

func (h stubDefinitionHost) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return "", ""
}

func (h stubDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorysessions.LiveSession) error {
	return nil
}

func (h stubDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorysessions.LiveSession, string, string, string, string) error {
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
	if _, err := config.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc := New(stubDefinitionHost{persistRootDir: rootDir})
	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	if current.Name != "alpha" {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Id == nil || *current.Id != "alpha" {
		t.Fatalf("current factory id = %#v, want alpha", current.Id)
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
	runtimeCfg, err := config.LoadRuntimeConfig(rootDir, nil)
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
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
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
	if _, err := config.PersistNamedFactory(rootDir, "alpha", versioned); err != nil {
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

func TestService_CurrentFactoryDefinitionVersionAtRoot_UsesFileModTimeForDefaultFactory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	modTime := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.Chtimes(factoryPath, modTime, modTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, err := New(stubDefinitionHost{}).CurrentFactoryDefinitionVersionAtRoot(rootDir, apisurface.DefaultCurrentFactoryName)
	if err != nil {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot: %v", err)
	}
	if got.Logical.Int64() != modTime.UnixNano() || !got.Physical.Equal(modTime) {
		t.Fatalf("version = %#v, want logical=%d physical=%s", got, modTime.UnixNano(), modTime)
	}
}
