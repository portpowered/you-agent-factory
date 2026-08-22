package process_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

const previousDefaultFixtureCatalogError = "fixture catalog not found; run from the repository root or set --fixture-catalog to pkg/transports/http/testdata/durable-session-contract-fixtures.json"

// TestInstalledBinaryFixtureCatalogSupportsMCPAndSessionInspection proves the
// default fixture-backed surface at the OS process boundary. The in-process
// root.BuildProcess tests cannot prove that a copied installed binary has no
// repository-relative fixture dependency.
func TestInstalledBinaryFixtureCatalogSupportsMCPAndSessionInspection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the successful file-read audit uses Linux strace")
	}
	if _, err := exec.LookPath("strace"); err != nil {
		t.Fatalf("the successful file-read audit requires strace: %v", err)
	}

	repoRoot := canonicalTestPath(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(context.Background(), 5*time.Minute)
	builtBinary := buildYouBinary(t, buildContext, repoRoot)
	cancelBuild()
	installedBinary := copyInstalledYouBinary(t, builtBinary)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	isolationRoot := t.TempDir()
	if pathWithin(repoRoot, isolationRoot) {
		t.Fatalf("isolated process root %q is beneath repository root %q", isolationRoot, repoRoot)
	}
	homeDir := filepath.Join(isolationRoot, "home")
	userProfileDir := filepath.Join(isolationRoot, "user-profile")
	workingDir := filepath.Join(isolationRoot, "customer-project")
	for _, path := range []string{homeDir, userProfileDir, workingDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create isolated process directory %q: %v", path, err)
		}
		if pathWithin(repoRoot, path) {
			t.Fatalf("isolated process directory %q is beneath repository root %q", path, repoRoot)
		}
	}

	stdout, stderr, trace, err := runInstalledMCPProcess(
		t,
		ctx,
		installedBinary,
		workingDir,
		homeDir,
		userProfileDir,
	)
	if err != nil {
		t.Fatalf("installed default MCP process: %v\nstdout:\n%s\nstderr:\n%s\ntrace:\n%s", err, stdout, stderr, trace)
	}
	if strings.Contains(stdout+stderr, previousDefaultFixtureCatalogError) {
		t.Fatalf("installed default MCP process emitted the retired repository fixture error:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	responses := decodeMCPResponses(t, stdout)
	initialize := requireMCPResponse(t, responses, "1")
	assertInitializeResponse(t, initialize)

	toolsList := requireMCPResponse(t, responses, "2")
	assertToolsListResponse(t, toolsList)

	startSyncResponse := requireMCPResponse(t, responses, "3")
	startSyncText := requireSuccessfulToolText(t, startSyncResponse, mcpfactorysession.ToolStartSync)
	assertStartSyncResult(t, startSyncText)

	listSessionsResponse := requireMCPResponse(t, responses, "4")
	listSessionsText := requireSuccessfulToolText(t, listSessionsResponse, mcpfactorysession.ToolListSessions)
	assertPersistedSessionList(t, listSessionsText)

	dispatchesResponse := requireMCPResponse(t, responses, "5")
	dispatchesText := requireSuccessfulToolText(t, dispatchesResponse, mcpfactorysession.ToolListDispatches)
	assertDispatchList(t, dispatchesText)

	successfulReads := successfulReadPaths(trace, workingDir)
	if len(successfulReads) == 0 {
		t.Fatalf("file-read audit recorded no successful reads; trace:\n%s", trace)
	}
	assertNoRepositoryPathAccess(t, trace, workingDir, repoRoot)

	t.Logf("before-change default installed invocation error: %s", previousDefaultFixtureCatalogError)
	t.Logf("after-change initialize response: %s", initialize.raw)
	t.Logf("after-change tools/list response: %s", toolsList.raw)
	t.Logf("after-change start_sync tool output: %s", startSyncText)
	t.Logf("after-change persisted session list tool output: %s", listSessionsText)
	t.Logf("after-change list_dispatches tool output: %s", dispatchesText)
	t.Logf("file-read audit recorded %d successful reads outside repository root", len(successfulReads))
}

type installedMCPResponse struct {
	raw    string
	result json.RawMessage
	errors *jsonRPCError
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func runInstalledMCPProcess(
	t testing.TB,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	homeDir string,
	userProfileDir string,
) (string, string, string, error) {
	t.Helper()

	tracePath := filepath.Join(t.TempDir(), "file-access.strace")
	args := []string{
		"-f", "-qq", "-yy", "-s", "4096",
		"-e", "trace=%file",
		"-o", tracePath,
		binaryPath,
		"mcp", "serve",
	}
	command := exec.CommandContext(ctx, "strace", args...)
	command.Dir = workingDir
	command.Env = installedProcessEnvironment(homeDir, userProfileDir, workingDir)
	command.Stdin = strings.NewReader(installedMCPInput())
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	trace, readErr := os.ReadFile(tracePath)
	if readErr != nil {
		return stdout.String(), stderr.String(), string(trace), fmt.Errorf("read strace output: %w (process error: %v)", readErr, err)
	}
	return stdout.String(), stderr.String(), string(trace), err
}

func installedMCPInput() string {
	return strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"fixture-installed-binary","version":"test"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"you.factory_session.start_sync","arguments":{"requestId":"req-petri-success-001","source":{"kind":"FACTORY_ID","factoryId":"customer-support-triage"},"args":{"ticketId":"TKT-2002"}}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"you.factory_session.list","arguments":{"scope":"persisted"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"you.factory_session.list_dispatches","arguments":{"sessionId":"dur-sess-petri-success-001"}}}`,
	}, "\n") + "\n"
}

func installedProcessEnvironment(homeDir, userProfileDir, workingDir string) []string {
	env := builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	return replaceEnvironmentValues(env, map[string]string{
		"GOWORK":      "off",
		"PWD":         workingDir,
		"USERPROFILE": userProfileDir,
	})
}

func replaceEnvironmentValues(environment []string, replacements map[string]string) []string {
	result := make([]string, 0, len(environment)+len(replacements))
	seen := make(map[string]struct{}, len(replacements))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if replacement, exists := replacements[key]; exists {
			result = append(result, key+"="+replacement)
			seen[key] = struct{}{}
			continue
		}
		if ok {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		if _, exists := seen[key]; !exists {
			result = append(result, key+"="+value)
		}
	}
	return result
}

func decodeMCPResponses(t testing.TB, stdout string) map[string]installedMCPResponse {
	t.Helper()

	responses := make(map[string]installedMCPResponse)
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *jsonRPCError   `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode MCP response line %q: %v", line, err)
		}
		key := string(bytes.TrimSpace(envelope.ID))
		if key == "" || key == "null" {
			continue
		}
		responses[key] = installedMCPResponse{raw: line, result: envelope.Result, errors: envelope.Error}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan MCP responses: %v", err)
	}
	return responses
}

func requireMCPResponse(t testing.TB, responses map[string]installedMCPResponse, id string) installedMCPResponse {
	t.Helper()
	response, ok := responses[id]
	if !ok {
		t.Fatalf("MCP response id %s missing; responses=%#v", id, responses)
	}
	if response.errors != nil {
		t.Fatalf("MCP response id %s returned JSON-RPC error %#v: %s", id, response.errors, response.raw)
	}
	return response
}

func assertInitializeResponse(t testing.TB, response installedMCPResponse) {
	t.Helper()
	var result struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(response.result, &result); err != nil {
		t.Fatalf("decode initialize result: %v; response=%s", err, response.raw)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Fatalf("initialize protocolVersion = %q, want 2024-11-05", result.ProtocolVersion)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Fatalf("initialize capabilities = %#v, want tools capability", result.Capabilities)
	}
	if result.ServerInfo.Name == "" || result.ServerInfo.Version == "" {
		t.Fatalf("initialize serverInfo = %#v, want name and version", result.ServerInfo)
	}
}

func assertToolsListResponse(t testing.TB, response installedMCPResponse) {
	t.Helper()
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(response.result, &result); err != nil {
		t.Fatalf("decode tools/list result: %v; response=%s", err, response.raw)
	}
	if len(result.Tools) != len(mcpfactorysession.ToolNames()) {
		t.Fatalf("tools/list count = %d, want %d; response=%s", len(result.Tools), len(mcpfactorysession.ToolNames()), response.raw)
	}
	listed := make(map[string]struct{}, len(result.Tools))
	for _, tool := range result.Tools {
		listed[tool.Name] = struct{}{}
	}
	for _, want := range mcpfactorysession.ToolNames() {
		if _, ok := listed[want]; !ok {
			t.Fatalf("tools/list missing %q; response=%s", want, response.raw)
		}
	}
}

func requireSuccessfulToolText(t testing.TB, response installedMCPResponse, toolName string) string {
	t.Helper()
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(response.result, &result); err != nil {
		t.Fatalf("decode %s MCP result: %v; response=%s", toolName, err, response.raw)
	}
	if result.IsError {
		t.Fatalf("%s MCP result isError=true; response=%s", toolName, response.raw)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("%s MCP content = %#v, want one text content; response=%s", toolName, result.Content, response.raw)
	}
	return result.Content[0].Text
}

func assertStartSyncResult(t testing.TB, text string) {
	t.Helper()
	var result struct {
		SessionID   string `json:"sessionId"`
		Status      string `json:"status"`
		SyncOutcome string `json:"syncOutcome"`
	}
	decodeToolResult(t, mcpfactorysession.ToolStartSync, text, &result)
	if result.SessionID != "dur-sess-petri-success-001" || result.Status != "SUCCEEDED" || result.SyncOutcome != "COMPLETED" {
		t.Fatalf("start_sync result = %#v, want known completed fixture session", result)
	}
}

func assertPersistedSessionList(t testing.TB, text string) {
	t.Helper()
	var result struct {
		Scope           string `json:"scope"`
		DurableSessions []struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		} `json:"durableSessions"`
	}
	decodeToolResult(t, mcpfactorysession.ToolListSessions, text, &result)
	if result.Scope != "persisted" {
		t.Fatalf("persisted session list scope = %q, want persisted", result.Scope)
	}
	for _, session := range result.DurableSessions {
		if session.SessionID == "dur-sess-petri-success-001" && session.Status == "SUCCEEDED" {
			return
		}
	}
	t.Fatalf("persisted session list = %#v, want completed known fixture session", result.DurableSessions)
}

func assertDispatchList(t testing.TB, text string) {
	t.Helper()
	var result struct {
		SessionID  string `json:"sessionId"`
		Dispatches []struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			DispatchKind string `json:"dispatchKind"`
		} `json:"dispatches"`
	}
	decodeToolResult(t, mcpfactorysession.ToolListDispatches, text, &result)
	if result.SessionID != "dur-sess-petri-success-001" {
		t.Fatalf("dispatch list sessionId = %q, want known fixture session", result.SessionID)
	}
	for _, dispatch := range result.Dispatches {
		if dispatch.ID == "disp-petri-success-001" && dispatch.Status == "COMPLETED" && dispatch.DispatchKind == "PETRI_TRANSITION" {
			return
		}
	}
	t.Fatalf("dispatch list = %#v, want completed known fixture dispatch", result.Dispatches)
}

func decodeToolResult(t testing.TB, toolName, text string, target any) {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *jsonRPCError   `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		t.Fatalf("decode %s tool text: %v; text=%s", toolName, err, text)
	}
	if envelope.Error != nil {
		t.Fatalf("%s tool error = %#v; text=%s", toolName, envelope.Error, text)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		t.Fatalf("%s tool result is empty; text=%s", toolName, text)
	}
	if err := json.Unmarshal(envelope.Result, target); err != nil {
		t.Fatalf("decode %s tool result: %v; text=%s", toolName, err, text)
	}
}

func copyInstalledYouBinary(t testing.TB, sourcePath string) string {
	t.Helper()
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat built you binary: %v", err)
	}
	installDir := filepath.Join(t.TempDir(), "installed-bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("create installed binary directory: %v", err)
	}
	name := "you"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destination := filepath.Join(installDir, name)
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read built you binary: %v", err)
	}
	if err := os.WriteFile(destination, contents, info.Mode().Perm()); err != nil {
		t.Fatalf("copy installed you binary: %v", err)
	}
	if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
		t.Fatalf("preserve installed you binary permissions: %v", err)
	}
	return destination
}

func canonicalTestPath(t testing.TB, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve test path %q: %v", path, err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatalf("make test path absolute %q: %v", resolved, err)
	}
	return filepath.Clean(absolute)
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

var stracePathPattern = regexp.MustCompile(`\b(?:open|openat|openat2|stat|statx|lstat|newfstatat|access|readlink|readlinkat)\([^\"]*\"((?:\\.|[^\"])*)\"`)
var straceResultPattern = regexp.MustCompile(`\)\s*=\s*(-?\d+)`)
var straceFDDirectoryPattern = regexp.MustCompile(`\((?:[^,]*<([^>]+)>|[^,]*),\s*\"`)

func successfulReadPaths(trace, workingDir string) []string {
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(trace, "\n") {
		if !isSuccessfulSyscall(line) || !strings.Contains(line, "open") {
			continue
		}
		if path, ok := resolveTracedPath(line, workingDir); ok {
			if _, exists := seen[path]; !exists {
				seen[path] = struct{}{}
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func assertNoRepositoryPathAccess(t testing.TB, trace, workingDir, repoRoot string) {
	t.Helper()
	for _, line := range strings.Split(trace, "\n") {
		path, ok := resolveTracedPath(line, workingDir)
		if !ok {
			continue
		}
		if pathWithin(repoRoot, path) {
			t.Fatalf("installed process accessed repository path %q; trace line: %s", path, line)
		}
	}
}

func isSuccessfulSyscall(line string) bool {
	match := straceResultPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return false
	}
	value, err := strconv.ParseInt(match[1], 10, 64)
	return err == nil && value >= 0
}

func resolveTracedPath(line, workingDir string) (string, bool) {
	match := stracePathPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return "", false
	}
	quoted := `"` + match[1] + `"`
	path, err := strconv.Unquote(quoted)
	if err != nil || path == "" || strings.HasPrefix(path, "<") {
		return "", false
	}
	if !filepath.IsAbs(path) {
		if directory := straceFDDirectoryPattern.FindStringSubmatch(line); len(directory) == 2 {
			path = filepath.Join(directory[1], path)
		} else {
			path = filepath.Join(workingDir, path)
		}
	}
	return filepath.Clean(path), true
}
