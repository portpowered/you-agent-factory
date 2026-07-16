package release_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

const compiledBinaryCommandTimeout = 30 * time.Second

// TestRootProcessCompiledBinaryModeMatrix proves the process-root migration at
// the installed-binary boundary instead of only through Cobra or root fakes.
func TestRootProcessCompiledBinaryModeMatrix(t *testing.T) {
	binaryPath := buildReleaseSmokeBinary(t)
	home := t.TempDir()
	environment := append(os.Environ(), "HOME="+home)

	t.Run("help succeeds", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "--help")
		if err != nil || !strings.Contains(output, "Usage:") {
			t.Fatalf("you --help = (%v, %q), want successful usage output", err, output)
		}
	})

	t.Run("non-startup docs command succeeds", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "docs", "config")
		if err != nil || !strings.Contains(output, "# Config") {
			t.Fatalf("you docs config = (%v, %q), want packaged docs", err, output)
		}
	})

	t.Run("invalid command fails once", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "not-a-command")
		if err == nil || strings.Count(output, `unknown command "not-a-command"`) != 1 {
			t.Fatalf("invalid command = (%v, %q), want one diagnostic and non-zero exit", err, output)
		}
	})

	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	t.Run("explicit local run succeeds", func(t *testing.T) {
		serverURL := reserveRootProcessSmokeURL(t)
		output, err := runBoundedBinary(
			t, binaryPath, environment,
			"run", "--dir", factoryDir, "--server", serverURL,
			"--with-mock-workers", "--quiet", "--no-record",
		)
		if err != nil {
			t.Fatalf("local run failed: %v\n%s", err, output)
		}
	})

	t.Run("default startup serves until cancellation", func(t *testing.T) {
		assertCompiledServiceMode(t, binaryPath, environment, t.TempDir(), []string{
			"--server", reserveRootProcessSmokeURL(t),
		})
	})

	t.Run("API service serves until cancellation", func(t *testing.T) {
		assertCompiledServiceMode(t, binaryPath, environment, factoryDir, []string{
			"run", "--dir", factoryDir, "--continuously", "--with-mock-workers",
			"--quiet", "--no-record", "--server", reserveRootProcessSmokeURL(t),
		})
	})

	t.Run("MCP serve processes stdio and exits", func(t *testing.T) {
		fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binaryPath, "mcp", "serve", "--fixture-catalog", fixturePath)
		cmd.Env = environment
		cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"root-smoke","version":"test"}}}` + "\n")
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), `"protocolVersion":"2024-11-05"`) {
			t.Fatalf("MCP serve = (%v, %q), want initialize response and clean EOF", err, string(output))
		}
	})
}

func runBoundedBinary(t *testing.T, binaryPath string, environment []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), compiledBinaryCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = environment
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("you %s exceeded %s bound", strings.Join(args, " "), compiledBinaryCommandTimeout)
	}
	return string(output), err
}

func reserveRootProcessSmokeURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve smoke port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release smoke port: %v", err)
	}
	return "http://" + address
}

func assertCompiledServiceMode(t *testing.T, binaryPath string, environment []string, workingDir string, args []string) {
	t.Helper()
	serverURL := args[len(args)-1]
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = workingDir
	cmd.Env = environment
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start service mode: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	client := &http.Client{Timeout: time.Second}
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/status", nil)
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				cancel()
				<-done
				return
			}
		}
		select {
		case err := <-done:
			cancel()
			t.Fatalf("service mode exited before readiness: %v\n%s", err, output.String())
		case <-ctx.Done():
			<-done
			t.Fatalf("service mode did not become ready at %s: %v\n%s", serverURL, ctx.Err(), output.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}
