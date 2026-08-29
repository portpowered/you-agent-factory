package models_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

const (
	story001ProcessTimeout = 45 * time.Second
	story001ServerTimeout  = 30 * time.Second
)

type builtProcessResult struct {
	exitCode      int
	stdout        []byte
	stderr        []byte
	runError      string
	processExited bool
	timedOut      bool
}

type startedBuiltProcess struct {
	command *exec.Cmd
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	cancel  context.CancelFunc
	waited  bool
}

func story001Environment(home, cache, endpoint string) []string {
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	environment = withoutEnvironmentKeys(environment,
		"HF_HOME", "HUGGINGFACE_HUB_CACHE", run.ModelCacheDirEnvironment,
		"YOU_MODELS_BACKEND_ENDPOINT", "NO_PROXY", "no_proxy", "LOCALAPPDATA", "APPDATA",
		"XDG_CACHE_HOME", "XDG_CONFIG_HOME",
	)
	return append(environment,
		"HF_HOME="+home,
		"HUGGINGFACE_HUB_CACHE="+cache,
		run.ModelCacheDirEnvironment+"="+cache,
		"YOU_MODELS_BACKEND_ENDPOINT="+endpoint,
		"LOCALAPPDATA="+home,
		"APPDATA="+home,
		"XDG_CACHE_HOME="+home,
		"XDG_CONFIG_HOME="+home,
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
	)
}

func withoutEnvironmentKeys(environment []string, keys ...string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || containsString(keys, key) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func runStory001Command(
	t testing.TB,
	parent context.Context,
	binaryPath, workDir string,
	environment []string,
	args ...string,
) builtProcessResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, story001ProcessTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = workDir
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := builtProcessResult{
		exitCode: -1,
		stdout:   append([]byte(nil), stdout.Bytes()...),
		stderr:   append([]byte(nil), stderr.Bytes()...),
		timedOut: ctx.Err() != nil,
	}
	if err == nil {
		result.exitCode = 0
		result.processExited = true
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		result.processExited = true
	}
	result.runError = err.Error()
	return result
}

func startStory001Command(
	t testing.TB,
	parent context.Context,
	binaryPath, workDir string,
	environment []string,
	args ...string,
) *startedBuiltProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, story001ProcessTimeout)
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = workDir
	command.Env = environment
	process := &startedBuiltProcess{command: command, cancel: cancel}
	command.Stdout = &process.stdout
	command.Stderr = &process.stderr
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start delivered you command %q: %v", strings.Join(args, " "), err)
	}
	return process
}

func (process *startedBuiltProcess) wait() builtProcessResult {
	if process == nil || process.waited {
		return builtProcessResult{exitCode: -1, runError: "process was already waited"}
	}
	process.waited = true
	err := process.command.Wait()
	process.cancel()
	result := builtProcessResult{
		exitCode: -1,
		stdout:   append([]byte(nil), process.stdout.Bytes()...),
		stderr:   append([]byte(nil), process.stderr.Bytes()...),
	}
	if err == nil {
		result.exitCode = 0
		result.processExited = true
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		result.processExited = true
	}
	result.runError = err.Error()
	return result
}

func (process *startedBuiltProcess) stop() {
	if process == nil || process.waited {
		return
	}
	if process.command.Process != nil && process.command.ProcessState == nil {
		_ = process.command.Process.Kill()
	}
	_ = process.command.Wait()
	process.cancel()
	process.waited = true
}

type httpObservation struct {
	method string
	path   string
	status int
	body   []byte
	err    string
}

func callStory001HTTP(
	t testing.TB,
	parent context.Context,
	method, endpoint string,
	body io.Reader,
) httpObservation {
	t.Helper()
	request, err := http.NewRequestWithContext(parent, method, endpoint, body)
	if err != nil {
		return httpObservation{method: method, path: endpoint, err: err.Error()}
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return httpObservation{method: method, path: endpoint, err: err.Error()}
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(response.Body)
	observation := httpObservation{
		method: method, path: endpoint, status: response.StatusCode,
		body: append([]byte(nil), responseBody...),
	}
	if readErr != nil {
		observation.err = readErr.Error()
	}
	return observation
}

func waitForStory001HTTP200(t testing.TB, parent context.Context, endpoint string) httpObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, story001ServerTimeout)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last httpObservation
	for {
		last = callStory001HTTP(t, ctx, http.MethodGet, endpoint, nil)
		if last.status == http.StatusOK {
			return last
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			last.err = ctx.Err().Error()
			return last
		}
	}
}

func reserveStory001Loopback(t testing.TB) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve story-001 loopback port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release story-001 loopback port: %v", err)
	}
	return address
}

func waitForStory001ModelStarts(t testing.TB, origin *characterizationOrigin, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), story001ServerTimeout)
	defer cancel()
	for observed := 0; observed < count; observed++ {
		select {
		case <-origin.modelStarted:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %d model download starts; observed=%d total=%d exchanges=%s", count, observed, origin.modelStartCount(), compactJSON(origin.exchangesSnapshot()))
		}
	}
}

func summarizeHTTP(observation httpObservation) string {
	return fmt.Sprintf(
		"method=%s status=%d bodyBytes=%d bodySHA256=%s error=%s",
		observation.method, observation.status, len(observation.body), sha256Hex(observation.body), observation.err,
	)
}

func summarizeProcess(result builtProcessResult) string {
	return fmt.Sprintf(
		"exit=%d processExited=%t stdoutBytes=%d stdoutSHA256=%s stderrBytes=%d stderrSHA256=%s sharingViolation=%t cacheCollision=%t timedOut=%t runError=%s",
		result.exitCode, result.processExited, len(result.stdout), sha256Hex(result.stdout), len(result.stderr), sha256Hex(result.stderr), processHasSharingViolation(result), processHasCacheCollision(result), result.timedOut, result.runError,
	)
}

func processHasSharingViolation(result builtProcessResult) bool {
	return containsDiagnosticMarker(processDiagnosticStream(result),
		"sharing violation", "error_sharing_violation", "being used by another process",
		"used by another process", "errno=32", "error 32",
	)
}

func processHasCacheCollision(result builtProcessResult) bool {
	return processHasSharingViolation(result) || containsDiagnosticMarker(processDiagnosticStream(result),
		"lock violation", "file is locked", "resource busy", "file exists",
	)
}

func processDiagnosticStream(result builtProcessResult) []byte {
	stream := make([]byte, 0, len(result.stdout)+len(result.stderr))
	stream = append(stream, result.stdout...)
	return append(stream, result.stderr...)
}

func containsDiagnosticMarker(stream []byte, markers ...string) bool {
	value := strings.ToLower(string(stream))
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

type cacheSnapshot struct {
	Entries      []string `json:"entries"`
	Partial      []string `json:"partial"`
	RegularBytes int64    `json:"regularBytes"`
}

func inspectStory001Cache(t testing.TB, root string) cacheSnapshot {
	t.Helper()
	snapshot := cacheSnapshot{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		snapshot.Entries = append(snapshot.Entries, relative)
		if strings.Contains(relative, ".partial") {
			snapshot.Partial = append(snapshot.Partial, relative)
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			snapshot.RegularBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect story-001 cache: %v", err)
	}
	return snapshot
}
