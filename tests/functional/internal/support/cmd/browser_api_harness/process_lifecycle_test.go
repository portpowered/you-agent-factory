package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootProcessReportsRuntimeStartFailure(t *testing.T) {
	t.Parallel()

	factoryDirectory := support.ScaffoldSingleStepFactory(t, "runtime-start-failure")
	const failureText = "injected runtime input failure"
	var walkerCalls atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		FactoryRuntimeInputDirectoryWalker: func(string, fs.WalkDirFunc) error {
			walkerCalls.Add(1)
			return errors.New(failureText)
		},
	})
	support.CleanupProcess(t, process)

	var stdout, stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "run", "--factory", filepath.Join(factoryDirectory, "factory.json"),
			"--with-mock-workers", "--no-record",
		},
		Env:              append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir()),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: factoryDirectory,
	})
	if err == nil || !strings.Contains(err.Error(), failureText) {
		t.Fatalf("Process.Execute(runtime start failure) error = %v, want injected failure; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if walkerCalls.Load() != 1 {
		t.Fatalf("injected runtime input walker calls = %d, want exactly one", walkerCalls.Load())
	}
}
