package acp_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

// TestPrebuiltACPDeliveredResultsSurvivePeerExit exercises the provider's actual
// stdio and process-wait boundary through a compiled CLI. The external peer
// flushes its response and exits immediately; no keepalive or timing padding
// separates the response from EOF. This package never builds the deliverable.
func TestPrebuiltACPDeliveredResultsSurvivePeerExit(t *testing.T) {
	binary := prebuiltCLI(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the external ACP peer fixture")
	}
	peer, err := filepath.Abs(filepath.Join("testdata", "delivered-peer.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"initialize-version", "prompt-result"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			directory := t.TempDir()
			env := builtcliacceptance.ProcessEnvForIsolatedHome(directory)
			launch := fmt.Sprintf("%q %q %s", node, peer, mode)
			_, diagnostic, err := invokeCLI(ctx, binary, directory, env,
				"workers", "acp", "add", "--name", "eof-peer", "--transport", "stdio", "--argument", launch)
			if err != nil {
				t.Fatalf("register external peer: %v\n%s", err, diagnostic)
			}
			stdout, stderr, err := invokeCLI(ctx, binary, directory, env,
				"run", "--named", "@you/subagent", "--worker-provider", "eof-peer",
				"--worker-model", "fixture", "--working-root", directory, "Return the fixture result")
			if mode == "initialize-version" {
				assertDeliveredUnsupportedVersion(t, stdout, stderr, err)
				return
			}
			if err != nil {
				t.Fatalf("delivered Prompt failed after peer exit: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if !strings.Contains(stdout, "delivered EOF primary result") {
				t.Fatalf("primary result was lost after peer exit: stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
}

func prebuiltCLI(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("INFINITE_YOU_INTEGRATION_BINARY"))
	if path == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("INFINITE_YOU_INTEGRATION_BINARY is required; the test never builds the CLI")
		}
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to a previously compiled CLI")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Fatalf("invalid compiled CLI artifact %q: %v", path, err)
	}
	return path
}

func invokeCLI(ctx context.Context, binary, directory string, env []string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func assertDeliveredUnsupportedVersion(t *testing.T, stdout, stderr string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("unsupported initialize version succeeded: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "unsupported protocol version 999") {
		t.Fatalf("delivered unsupported-version failure was lost: error=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if strings.Contains(stderr, "disconnected before responding") || strings.TrimSpace(stdout) != "" {
		t.Fatalf("unsupported version became disconnect or success: stdout=%q stderr=%q", stdout, stderr)
	}
}
