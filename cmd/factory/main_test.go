package main

import "testing"

func TestMainDelegatesExitCodeToRootProcess(t *testing.T) {
	originalRun := runProcess
	originalExit := exitProcess
	t.Cleanup(func() {
		runProcess = originalRun
		exitProcess = originalExit
	})

	runCalls := 0
	runProcess = func() int {
		runCalls++
		return 23
	}
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}

	main()

	if runCalls != 1 {
		t.Fatalf("root process calls = %d, want 1", runCalls)
	}
	if exitCode != 23 {
		t.Fatalf("exit code = %d, want 23", exitCode)
	}
}
