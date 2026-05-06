package main

import "testing"

func TestMainExecutesFunctionalLane(t *testing.T) {
	original := executeFunctionalLane
	t.Cleanup(func() {
		executeFunctionalLane = original
	})

	called := false
	executeFunctionalLane = func() error {
		called = true
		return nil
	}

	main()

	if !called {
		t.Fatal("main() did not execute the functional lane entrypoint")
	}
}
