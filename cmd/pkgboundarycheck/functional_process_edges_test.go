package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsWorkersProcessPortsInFunctionalTests(t *testing.T) {
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/process/process.go", "package process\n")
	writeGoSourceFile(t, repoRoot, "tests/functional/runtime_api/edge_test.go", `package runtime_api

import workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

var runner workerexecution.CommandRunner
var request workerexecution.CommandRequest
var result workerexecution.CommandResult
`)

	var stdout, stderr bytes.Buffer
	err := run(config{root: repoRoot, packageRoot: "pkg"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want functional Workers process port rejected")
	}
	for _, symbol := range []string{"CommandRunner", "CommandRequest", "CommandResult"} {
		if !strings.Contains(stderr.String(), "prohibited functional Workers process port: workers."+symbol) {
			t.Fatalf("run() stderr = %q, want %s diagnostic", stderr.String(), symbol)
		}
	}
}

func TestRunAllowsPlatformProcessPortsInFunctionalTests(t *testing.T) {
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/process/process.go", "package process\n")
	writeGoSourceFile(t, repoRoot, "tests/functional/runtime_api/edge_test.go", `package runtime_api

import platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

var runner platformprocess.CommandRunner
var request platformprocess.CommandRequest
var result platformprocess.CommandResult
`)

	var stdout, stderr bytes.Buffer
	if err := run(config{root: repoRoot, packageRoot: "pkg"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %q", err, stderr.String())
	}
}
