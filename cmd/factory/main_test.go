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

func TestMainHelpExecutesWithoutError(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"you", "--help"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	main()
}

func TestBuildCLIRuntimeRunner_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	_, err := buildCLIRuntimeRunner(context.Background(), &service.FactoryServiceConfig{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("expected buildCLIRuntimeRunner to fail without factory.json")
	}
}

func TestBuildCLIRuntimeRunner_ReturnsInitializerRunner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	runner, err := buildCLIRuntimeRunner(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("buildCLIRuntimeRunner: %v", err)
	}
	if runner == nil {
		t.Fatal("expected initializer-produced runtime runner")
	}
}
