package main

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func TestMainExecutesCLI(t *testing.T) {
	t.Parallel()

	original := executeCLI
	t.Cleanup(func() {
		executeCLI = original
	})

	called := false
	executeCLI = func() {
		called = true
	}

	main()

	if !called {
		t.Fatal("main() did not execute the CLI entrypoint")
	}
}

func TestBuildCLIRunnerComposesConfiguredRuntime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	runner, err := buildCLIRunner(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("buildCLIRunner() error = %v", err)
	}
	if runner == nil {
		t.Fatal("buildCLIRunner() returned nil runner")
	}
}

func TestMainHelpExecutesWithoutError(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"you", "--help"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	main()
}
