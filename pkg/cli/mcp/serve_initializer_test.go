package mcpcli_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestRunServe_MatchesServiceBuildFactoryServiceInvalidConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	errCompose := mcpcli.RunServe(ctx, mcpcli.ServeConfig{
		FactoryDir:         dir,
		FixtureCatalogPath: fixtureCatalogPathForInitializerTest(t),
		Stdout:             stdoutWrite,
		Stdin:              stdinRead,
	})
	_, errService := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{Dir: dir})

	if errCompose == nil {
		t.Fatal("expected initializer-backed RunServe to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected service.BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errCompose.Error() {
		t.Fatalf("service.BuildFactoryService error = %q, want %q", errService, errCompose)
	}
}

func TestRunServe_InitializerBackedInstallSmokeDiscovery(t *testing.T) {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- mcpcli.RunServe(ctx, mcpcli.ServeConfig{
			FixtureCatalogPath: fixtureCatalogPathForInitializerTest(t),
			Stdin:              stdinRead,
			Stdout:             stdoutWrite,
		})
	}()

	client := newStdioMCPClient(t, stdinWrite, stdoutRead)
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)

	_ = stdinWrite.Close()
	select {
	case err := <-serveErr:
		if err != nil && err != io.EOF {
			t.Fatalf("RunServe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServe did not shut down after stdin closed")
	}
}

func fixtureCatalogPathForInitializerTest(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
}
