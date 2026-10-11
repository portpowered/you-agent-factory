package subagent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

// One compiled host and real executable locator reject both missing-path and
// unresolved-name routes. PATH, profile, port and Factory are test-owned; no
// executable is installed and no inference is permitted. Functional tests own
// the exhaustive transport/error matrix.
func TestSubagentUnavailableExecutableContract(t *testing.T) {
	binary := prebuiltCLI(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	directory, home, privateBin := t.TempDir(), t.TempDir(), t.TempDir()
	writeCurrentFactory(t, directory)
	missing := filepath.Join(privateBin, "absent agent.exe")
	configPath := writeUnavailableProfile(t, home, missing)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	for i, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			env[i] = "PATH=" + privateBin
		}
	}
	// Bootstrap first-run local assets before measuring host readiness.
	runCLI(t, ctx, binary, directory, env, "init", "--provider", "codex", "--model", "unused-fallback")
	host := startUnavailableArtifactHost(t, ctx, binary, directory, env)
	factoriesBefore := factorySessionIDs(t, ctx, host)
	workersBefore := unavailableWorkerListing(t, ctx, host)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"missing-path", "missing-name"} {
		assertUnavailableArtifactCLI(t, ctx, binary, directory, env, host, route, missing)
	}
	if after := factorySessionIDs(t, ctx, host); !slices.Equal(after, factoriesBefore) {
		t.Fatal("unavailable START created Factory Session")
	}
	if after := unavailableWorkerListing(t, ctx, host); !bytes.Equal(after, workersBefore) {
		t.Fatal("unavailable START admitted Worker Session")
	}
	if after, err := os.ReadFile(configPath); err != nil || !bytes.Equal(after, before) {
		t.Fatal("START rewrote configuration")
	}
}

func startUnavailableArtifactHost(t *testing.T, ctx context.Context, binary, directory string, env []string) string {
	t.Helper()
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	host := fmt.Sprintf("http://127.0.0.1:%d", port)
	command := exec.CommandContext(ctx, binary, "run", "--dir", filepath.Join(directory, "factory"), "--continuously", "--with-server", "--listen", strings.TrimPrefix(host, "http://"), "--quiet", "--no-record")
	command.Dir, command.Env, command.Stdout = directory, env, io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() { _ = command.Process.Kill(); <-done })
	// A real separate daemon has no injected readiness channel. Observe only
	// its public status until ready or its bounded process lifetime ends.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			var status struct {
				FactoryState string `json:"factoryState"`
			}
			err = json.NewDecoder(response.Body).Decode(&status)
			_ = response.Body.Close()
			if err == nil && response.StatusCode == http.StatusOK && status.FactoryState == "RUNNING" {
				return host
			}
		}
		select {
		case err := <-done:
			done <- err
			t.Fatalf("compiled host exited before readiness: %v %s", err, stderr.String())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func unavailableWorkerListing(t *testing.T, ctx context.Context, host string) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/worker-sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Worker Session listing = %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeUnavailableProfile(t *testing.T, home, missing string) string {
	t.Helper()
	profile := map[string]any{"defaults": map[string]any{"workerModelProvider": "codex", "workerModel": "unused-fallback", "subagent": map[string]any{"provider": "missing-path", "model": "controlled-model"}},
		"workers": map[string]any{"acp": map[string]any{"integrations": []any{
			map[string]any{"id": "missing-path", "name": "missing-path", "transport": "stdio", "command": `"` + filepath.ToSlash(missing) + `" acp`},
			map[string]any{"id": "missing-name", "name": "missing-name", "transport": "stdio", "command": "unresolved-subagent-contract-agent acp"},
		}}}}
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func assertUnavailableArtifactCLI(t *testing.T, ctx context.Context, binary, directory string, env []string, host, route, missing string) {
	t.Helper()
	args := []string{"--server", host, "--json", "worker-sessions", "start", "--request-id", route + "-request", "--worker-session-id", route, "--working-root", directory}
	if route == "missing-name" {
		args = append(args, "--provider", route, "--model", "controlled-model")
	}
	args = append(args, "private synthetic contract input")
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err == nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "PROVIDER_UNAVAILABLE") {
		t.Fatalf("%s must reject before acknowledgment (exit=%v)", route, err)
	}
	if strings.Contains(stderr.String(), missing) || strings.Contains(stderr.String(), "private synthetic") {
		t.Fatal("unavailable error exposed private input")
	}
	t.Logf("%s: compiled CLI rejected with PROVIDER_UNAVAILABLE and no acknowledgment", route)
}
