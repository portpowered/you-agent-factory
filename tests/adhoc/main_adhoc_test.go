package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestAdHocPrepare runs the customer command through the canonical process.
// Set AGENT_FACTORY_ADHOC_RUN=1 and run:
//
//	go test -v ./tests/adhoc -run TestAdHocPrepare
func TestAdHocPrepare(t *testing.T) {
	if os.Getenv("AGENT_FACTORY_ADHOC_RUN") != "1" {
		t.Skip("adhoc test - set AGENT_FACTORY_ADHOC_RUN=1 to run manually")
	}

	factoryDir := getenv("AGENT_FACTORY_ADHOC_DIR", adhocFixtureDir(t))
	artifactPath := getenv("AGENT_FACTORY_ADHOC_RECORD", "./adhoc-recording-batch-2.json")
	args := []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--record", artifactPath,
		"--with-mock-workers",
	}
	if replayPath := os.Getenv("AGENT_FACTORY_ADHOC_REPLAY"); replayPath != "" {
		args = append(args, "--replay", replayPath)
	}

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	input := root.Input{
		Args:             args,
		Env:              os.Environ(),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}
	if err := process.Execute(input); err != nil {
		t.Fatalf("Process.Execute() error = %v\nstderr: %s", err, stderr.String())
	}
	t.Logf("stdout:\n%s", stdout.String())
	if stderr.Len() > 0 {
		t.Logf("stderr:\n%s", stderr.String())
	}
}

func adhocFixtureDir(t *testing.T) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(t, 0, "tests", "adhoc", "factory")
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
