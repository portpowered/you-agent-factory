package workscope_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func doPrebuiltWorkscopeGET(t *testing.T, ctx context.Context, client *http.Client, endpoint string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build prebuilt Work Session GET %s: %v", endpoint, err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET prebuilt Work Session endpoint %s: %v", endpoint, err)
	}
	return response
}

func readPrebuiltWorkscopeResponse(t *testing.T, response *http.Response) ([]byte, int) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read prebuilt Work Session response: %v", err)
	}
	return body, response.StatusCode
}

func stopPrebuiltWorkscopeDaemon(
	t *testing.T,
	ctx context.Context,
	daemon *prebuiltWorkscopeDaemon,
	binaryPath, workspace string,
	environment []string,
	serverURL string,
) {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(stopCtx, binaryPath, "--server", serverURL, "server", "stop")
	command.Dir = workspace
	command.Env = append([]string(nil), environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("stop prebuilt Work Session process through public server command: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	select {
	case err := <-daemon.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("prebuilt Work Session process exit after stop = %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
		}
	case <-stopCtx.Done():
		t.Fatalf("prebuilt Work Session process did not stop: %v\nstdout=%s\nstderr=%s", stopCtx.Err(), daemon.stdout.String(), daemon.stderr.String())
	}
	daemon.mu.Lock()
	daemon.stopped = true
	daemon.mu.Unlock()
}

func cleanupPrebuiltWorkscopeDaemon(daemon *prebuiltWorkscopeDaemon) {
	if daemon == nil {
		return
	}
	daemon.mu.Lock()
	if daemon.stopped || daemon.command == nil || daemon.command.Process == nil {
		daemon.mu.Unlock()
		return
	}
	daemon.stopped = true
	process := daemon.command.Process
	daemon.mu.Unlock()
	if daemon.memory != nil {
		_, _, _ = daemon.memory.stopAndRead()
	}
	if os.PathSeparator == '\\' {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F").Run()
	} else {
		_ = process.Kill()
	}
	select {
	case <-daemon.done:
	case <-time.After(10 * time.Second):
	}
}

func preparePrebuiltWorkscopeFixture(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read pinned prebuilt Work Session fixture %s: %v", source, err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("decode pinned prebuilt Work Session fixture: %v", err)
	}
	var events []map[string]json.RawMessage
	if err := json.Unmarshal(document["events"], &events); err != nil {
		t.Fatalf("decode pinned prebuilt Work Session events: %v", err)
	}
	continuation, err := json.Marshal(map[string]string{
		"Provider":          "codex",
		"Kind":              "session_id",
		"ProviderSessionID": prebuiltWorkscopeProviderSession,
		"ExternalRef":       prebuiltWorkscopeProviderSession,
	})
	if err != nil {
		t.Fatalf("encode controlled prebuilt Provider Session identity: %v", err)
	}
	responses := 0
	for _, event := range events {
		var eventType string
		if err := json.Unmarshal(event["type"], &eventType); err != nil {
			t.Fatalf("decode pinned Factory Event type: %v", err)
		}
		if eventType != "INFERENCE_RESPONSE" {
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event["payload"], &payload); err != nil {
			t.Fatalf("decode pinned inference response payload: %v", err)
		}
		payload["continuation"] = continuation
		event["payload"], err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode controlled inference response payload: %v", err)
		}
		responses++
	}
	if responses != 1 {
		t.Fatalf("pinned fixture contains %d inference responses, want one controlled Provider Session association", responses)
	}
	document["events"], err = json.Marshal(events)
	if err != nil {
		t.Fatalf("encode prepared prebuilt Factory Event fixture: %v", err)
	}
	prepared, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode prepared prebuilt retained-history fixture: %v", err)
	}
	prepared = append(prepared, '\n')
	if err := os.WriteFile(destination, prepared, 0o600); err != nil {
		t.Fatalf("write isolated prepared prebuilt fixture %s: %v", destination, err)
	}
}

func copyWorkscopeFile(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read Work Session witness input %s: %v", source, err)
	}
	if len(contents) == 0 {
		t.Fatalf("Work Session witness input %s is empty", source)
	}
	if err := os.WriteFile(destination, contents, 0o600); err != nil {
		t.Fatalf("copy Work Session witness input to %s: %v", destination, err)
	}
}

func copyPrebuiltWorkscopeDirectory(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse to copy symlink from isolated prebuilt profile")
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
	if err != nil {
		t.Fatalf("copy isolated prebuilt profile from %s to %s: %v", source, destination, err)
	}
}

func sha256File(t *testing.T, path string) (string, int64) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read SHA-256 input %s: %v", path, err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), int64(len(contents))
}

func sha256FileDigest(t *testing.T, path string) string {
	t.Helper()
	digest, _ := sha256File(t, path)
	return digest
}

func elapsedMilliseconds(started time.Time) float64 {
	return float64(time.Since(started).Nanoseconds()) / float64(time.Millisecond)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}
