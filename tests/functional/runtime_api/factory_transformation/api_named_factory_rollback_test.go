package factory_transformation

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// rollbackLoadingFileSystem is a construction-time filesystem edge used by
// this functional scenario to make the candidate runtime load fail after the
// public upsert has persisted and selected the candidate. The production
// process remains unchanged; the failure is injected at the same replaceable
// edge used by root.BuildProcess.
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

func TestFactoryTransformation_UpsertRollbackRemovesCandidateAndAbsentSelector(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write root Factory config: %v", err)
	}
	server := startDocumentTransformationServer(t, rootDir, "")

	const candidateName = "rollback-candidate"
	releaseLoadFailure := factoryTransformationFixture.loadingFileSystem.FailFactoryLoad(candidateName)
	defer releaseLoadFailure()

	response := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		server.URL(),
		factorySessionPath(server.SessionID()),
		upsertNamedFactoryRequestBody(functionalNamedFactoryBody(candidateName, "candidate-task")),
		http.StatusBadRequest,
	)
	var failure factoryapi.ErrorResponse
	decodeJSONResponse(t, response, &failure, "decode rollback response")
	if failure.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("rollback error code = %q, want INVALID_FACTORY", failure.Code)
	}

	if _, err := os.Stat(filepath.Join(rootDir, ".current-factory")); !os.IsNotExist(err) {
		t.Fatalf("current Factory selector after rollback: err=%v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, candidateName)); !os.IsNotExist(err) {
		t.Fatalf("candidate Factory directory after rollback: err=%v, want absent", err)
	}
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if current.Name != factoryapi.FactoryName("UNDEFINED") {
		t.Fatalf("current Factory name after rollback = %q, want UNDEFINED", current.Name)
	}
	assertFactoryWorkType(t, current, "root-task", "current Factory after rollback")
}
