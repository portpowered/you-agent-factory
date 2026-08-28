package catalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const catalogSharedProcessShutdownTimeout = 15 * time.Second

// catalogSharedProcessFixture owns the one compatible root process for the
// catalog package. Its policy-free filesystem edges are swappable only
// between sequential failure rows; normal rows use the local implementation.
// API catalog discovery routes its short-lived hosted command through the same
// process and a fresh local server.
type catalogSharedProcessFixture struct {
	process      support.ApplicationProcess
	provider     *catalogSwitchingProviderRunner
	namedCatalog *catalogSwitchingNamedCatalogFileSystem
	authored     *catalogSwitchingAuthoredReaderFileSystem
	apiRouter    *catalogAPIServerRouter
}

type catalogSwitchingProviderRunner struct {
	mu       sync.Mutex
	delegate platformprocess.CommandRunner
}

func (runner *catalogSwitchingProviderRunner) setDelegate(delegate platformprocess.CommandRunner) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.delegate = delegate
}

func (runner *catalogSwitchingProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	delegate := runner.delegate
	runner.mu.Unlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, errors.New("no catalog provider command runner installed")
	}
	return delegate.Run(ctx, request)
}

type catalogSwitchingNamedCatalogFileSystem struct {
	mu       sync.Mutex
	delegate factorydefinitions.NamedFactoryCatalogFileSystem
}

func newCatalogSwitchingNamedCatalogFileSystem() *catalogSwitchingNamedCatalogFileSystem {
	return &catalogSwitchingNamedCatalogFileSystem{delegate: platformfilesystem.Local{}}
}

func (filesystem *catalogSwitchingNamedCatalogFileSystem) setDelegate(
	delegate factorydefinitions.NamedFactoryCatalogFileSystem,
) {
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	filesystem.delegate = delegate
}

func (filesystem *catalogSwitchingNamedCatalogFileSystem) current() factorydefinitions.NamedFactoryCatalogFileSystem {
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	return filesystem.delegate
}

func (filesystem *catalogSwitchingNamedCatalogFileSystem) Stat(path string) (fs.FileInfo, error) {
	return filesystem.current().Stat(path)
}

func (filesystem *catalogSwitchingNamedCatalogFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	return filesystem.current().ReadDir(path)
}

func (filesystem *catalogSwitchingNamedCatalogFileSystem) RemoveAll(path string) error {
	return filesystem.current().RemoveAll(path)
}

type catalogSwitchingAuthoredReaderFileSystem struct {
	mu       sync.Mutex
	delegate factorydefinitions.AuthoredLayoutReaderFileSystem
}

func newCatalogSwitchingAuthoredReaderFileSystem() *catalogSwitchingAuthoredReaderFileSystem {
	return &catalogSwitchingAuthoredReaderFileSystem{delegate: platformfilesystem.Local{}}
}

func (filesystem *catalogSwitchingAuthoredReaderFileSystem) setDelegate(
	delegate factorydefinitions.AuthoredLayoutReaderFileSystem,
) {
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	filesystem.delegate = delegate
}

func (filesystem *catalogSwitchingAuthoredReaderFileSystem) current() factorydefinitions.AuthoredLayoutReaderFileSystem {
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	return filesystem.delegate
}

func (filesystem *catalogSwitchingAuthoredReaderFileSystem) ReadFile(path string) ([]byte, error) {
	return filesystem.current().ReadFile(path)
}

func (filesystem *catalogSwitchingAuthoredReaderFileSystem) Stat(path string) (fs.FileInfo, error) {
	return filesystem.current().Stat(path)
}

type catalogAPIServerRouter struct {
	mu     sync.Mutex
	server *support.ProcessAPIServer
}

func (router *catalogAPIServerRouter) set(server *support.ProcessAPIServer) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.server = server
}

func (router *catalogAPIServerRouter) current() *support.ProcessAPIServer {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.server
}

func (router *catalogAPIServerRouter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	router.mu.Lock()
	server := router.server
	router.mu.Unlock()
	if server == nil {
		return errors.New("catalog API server is not selected")
	}
	return server.Start(ctx, request)
}

var (
	catalogFixtureOnce sync.Once
	catalogFixture     *catalogSharedProcessFixture
	catalogFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	var closeErr error
	if catalogFixture != nil {
		closeErr = catalogFixture.close()
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close shared catalog process: %v\n", closeErr)
			if code == 0 {
				code = 1
			}
		}
	}
	if err := writeCatalogForcedUnwindReport(catalogFixture, closeErr); err != nil {
		fmt.Fprintf(os.Stderr, "write catalog forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func sharedCatalogProcess(t *testing.T) *catalogSharedProcessFixture {
	t.Helper()
	catalogFixtureOnce.Do(func() {
		catalogFixture, catalogFixtureErr = startCatalogSharedProcess()
	})
	if catalogFixtureErr != nil {
		t.Fatalf("start shared catalog process: %v", catalogFixtureErr)
	}
	if catalogFixture == nil {
		t.Fatal("shared catalog process is unavailable")
	}
	return catalogFixture
}

func startCatalogSharedProcess() (*catalogSharedProcessFixture, error) {
	provider := &catalogSwitchingProviderRunner{}
	namedCatalog := newCatalogSwitchingNamedCatalogFileSystem()
	authored := newCatalogSwitchingAuthoredReaderFileSystem()
	apiRouter := &catalogAPIServerRouter{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner:                          provider,
		FactoryDefinitionNamedFactoryCatalogFileSystem: namedCatalog,
		FactoryDefinitionAuthoredReaderFileSystem:      authored,
		APIServerStarter:                               apiRouter.start,
	})
	if err != nil {
		return nil, err
	}
	return &catalogSharedProcessFixture{
		process: process, provider: provider, namedCatalog: namedCatalog,
		authored: authored, apiRouter: apiRouter,
	}, nil
}

func (fixture *catalogSharedProcessFixture) close() error {
	if fixture == nil || fixture.process == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), catalogSharedProcessShutdownTimeout)
	defer cancel()
	closeErr := fixture.process.Close(ctx)
	if fixture.apiRouter != nil {
		if server := fixture.apiRouter.current(); server != nil {
			if baseURL, ok := server.BaseURL(); ok {
				closeErr = errors.Join(closeErr, catalogListenerError(baseURL))
			}
		}
	}
	return closeErr
}
