package definition_activation_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// rollbackLoadingFileSystem is owned by this scenario. It injects one
// candidate-load failure through the same replaceable edge used by
// root.BuildProcess, without sharing a mutable failure switch with the
// package's parallel functional scenarios.
type rollbackLoadingFileSystem struct {
	platformfilesystem.Local
	mu                 sync.Mutex
	blockedFactoryName string
	allowedConfigReads int
}

func newRollbackLoadingFileSystem() *rollbackLoadingFileSystem {
	return &rollbackLoadingFileSystem{}
}

func (f *rollbackLoadingFileSystem) FailFactoryLoad(name string) func() {
	f.mu.Lock()
	f.blockedFactoryName = name
	f.allowedConfigReads = 1
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.blockedFactoryName = ""
		f.allowedConfigReads = 0
		f.mu.Unlock()
	}
}

func (f *rollbackLoadingFileSystem) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	blockedName := f.blockedFactoryName
	f.mu.Unlock()
	if blockedName != "" && isFactoryConfigPath(path, blockedName) {
		f.mu.Lock()
		if f.allowedConfigReads > 0 {
			f.allowedConfigReads--
			f.mu.Unlock()
			return f.Local.ReadFile(path)
		}
		f.mu.Unlock()
		return nil, fmt.Errorf("injected candidate load failure for Factory %q", blockedName)
	}
	return f.Local.ReadFile(path)
}

func isFactoryConfigPath(path, name string) bool {
	normalized := filepath.ToSlash(filepath.Clean(path))
	segment := "/" + strings.TrimSpace(name) + "/"
	return strings.HasSuffix(normalized, "/"+interfaces.FactoryConfigFile) && strings.Contains(normalized, segment)
}

func (f *rollbackLoadingFileSystem) Stat(path string) (fs.FileInfo, error) {
	return f.Local.Stat(path)
}

func TestDefinitionActivationNamedUpsertRollbackRemovesCandidateAndRestoresAbsentSelector(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		[]byte(definitionActivationFactoryBody("root-runtime", "root-task", nil)),
		0o644,
	); err != nil {
		t.Fatalf("write root Factory config: %v", err)
	}

	loadingFileSystem := newRollbackLoadingFileSystem()
	server := startDefinitionActivationRollbackServer(t, rootDir, loadingFileSystem)

	const candidateName = "rollback-candidate"
	releaseLoadFailure := loadingFileSystem.FailFactoryLoad(candidateName)
	defer releaseLoadFailure()

	response := upsertDefinitionActivationNamedFactoryExpectStatus(
		t,
		server.URL(),
		definitionActivationFactoryBody(candidateName, "candidate-task", nil),
		http.StatusBadRequest,
	)
	if response.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("rollback error code = %q, want INVALID_FACTORY", response.Code)
	}

	if _, err := os.Stat(filepath.Join(rootDir, ".current-factory")); !os.IsNotExist(err) {
		t.Fatalf("current Factory selector after rollback: err=%v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, candidateName)); !os.IsNotExist(err) {
		t.Fatalf("candidate Factory directory after rollback: err=%v, want absent", err)
	}
	current := getDefinitionActivationCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("UNDEFINED") {
		t.Fatalf("current Factory name after rollback = %q, want UNDEFINED", current.Name)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "root-task" {
		t.Fatalf("current Factory work types after rollback = %#v, want root-task", current.WorkTypes)
	}
}

func TestDefinitionActivationNamedUpsertRollbackRestoresExistingSelector(t *testing.T) {
	rootDir := t.TempDir()
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(
		sourcePath,
		[]byte(definitionActivationFactoryBody("alpha", "alpha-task", nil)),
		0o600,
	); err != nil {
		t.Fatalf("write alpha Factory source: %v", err)
	}
	support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, "alpha", sourcePath)

	loadingFileSystem := newRollbackLoadingFileSystem()
	server := startDefinitionActivationRollbackServer(t, rootDir, loadingFileSystem)

	const candidateName = "rollback-existing-selector"
	releaseLoadFailure := loadingFileSystem.FailFactoryLoad(candidateName)
	defer releaseLoadFailure()

	response := upsertDefinitionActivationNamedFactoryExpectStatus(
		t,
		server.URL(),
		definitionActivationFactoryBody(candidateName, "candidate-task", nil),
		http.StatusBadRequest,
	)
	if response.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("rollback error code = %q, want INVALID_FACTORY", response.Code)
	}

	selector, err := os.ReadFile(filepath.Join(rootDir, ".current-factory"))
	if err != nil {
		t.Fatalf("read current Factory selector after rollback: %v", err)
	}
	if strings.TrimSpace(string(selector)) != "alpha" {
		t.Fatalf("current Factory selector after rollback = %q, want alpha", strings.TrimSpace(string(selector)))
	}
	if _, err := os.Stat(filepath.Join(rootDir, candidateName)); !os.IsNotExist(err) {
		t.Fatalf("candidate Factory directory after rollback: err=%v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("existing Factory after rollback: %v", err)
	}
	current := getDefinitionActivationCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current Factory name after rollback = %q, want alpha", current.Name)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "alpha-task" {
		t.Fatalf("current Factory work types after rollback = %#v, want alpha-task", current.WorkTypes)
	}
}

func startDefinitionActivationRollbackServer(
	t *testing.T,
	rootDir string,
	loadingFileSystem *rollbackLoadingFileSystem,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			FactoryDefinitionLoadingFileSystem:        loadingFileSystem,
			FactoryDefinitionAuthoredReaderFileSystem: loadingFileSystem,
		},
	})
}

func upsertDefinitionActivationNamedFactoryExpectStatus(
	t *testing.T,
	serverURL, body string,
	expectedStatus int,
) factoryapi.ErrorResponse {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(body), &factory); err != nil {
		t.Fatalf("decode named Factory body: %v", err)
	}
	factory.Version = nil
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	payloadBytes, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{
		Factory: factory,
		Mode:    &mode,
	})
	if err != nil {
		t.Fatalf("encode named Factory activation request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPut, serverURL+"/factory-sessions/~default/factory", strings.NewReader(string(payloadBytes)))
	if err != nil {
		t.Fatalf("build PUT named Factory request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT named Factory activation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("PUT named Factory activation status = %d, want %d", response.StatusCode, expectedStatus)
	}
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatalf("decode named Factory activation error: %v", err)
	}
	return failure
}
