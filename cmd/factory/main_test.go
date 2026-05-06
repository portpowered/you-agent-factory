package main

import (
	"os"
	"testing"
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
	os.Args = []string{"infinite-you", "--help"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	main()
}
