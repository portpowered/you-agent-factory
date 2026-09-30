package acp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	acpHelperArtifactPathEnv = "INFINITE_YOU_ACP_HELPER_ARTIFACT_PATH"
	acpHelperArtifactHashEnv = "INFINITE_YOU_ACP_HELPER_ARTIFACT_SHA256"
	acpHelperArtifactNeedEnv = "INFINITE_YOU_ACP_HELPER_ARTIFACT_REQUIRED"
	acpObservationExportEnv  = "YOU_ACP_TEST_OBSERVATION_EXPORT_DIR"
	acpHelperProcessCeiling  = 20 * time.Second
	acpObservationMaxBytes   = 64 * 1024
	acpHelperFixtureArg      = "-acp-fixture="
)

// TestACPRetryHelperProcessLifecycle exercises only the OS process and ACP
// stdio boundary. The helper is the prebuilt functional-test artifact supplied
// by Backend Integration; no binary is built from test setup.
func TestACPRetryHelperProcessLifecycle(t *testing.T) {
	t.Parallel()
	helperPath := requirePrebuiltACPHelper(t)
	observationPath := configuredObservationExportPath(t)

	root := t.TempDir()
	localTracePath := filepath.Join(root, "rpc.jsonl")
	if observationPath != "" {
		localTracePath = observationPath
	}
	fixture := acpHelperFixture{
		Kind:      "functional",
		Mode:      "retry-resume",
		SessionID: "acp-session-retry-resume",
	}
	if observationPath != "" {
		fixture.ObservationPath = observationPath
	}

	firstAttemptDir := filepath.Join(root, "attempt-1")
	secondAttemptDir := filepath.Join(root, "attempt-2")
	writeAttemptMarker(t, firstAttemptDir, "1")
	writeAttemptMarker(t, secondAttemptDir, "2")

	firstFixture := fixture
	firstFixture.RetryAttemptDirectory = firstAttemptDir
	first := runACPHelperConversation(t, helperPath, firstFixture, retryConversation(false, fixture.SessionID))
	firstSession := requireACPResponse(t, first.stdout, "2")
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(firstSession.Result, &created); err != nil {
		t.Fatalf("decode ACP session/new response: %v", err)
	}
	if created.SessionID != fixture.SessionID {
		t.Fatalf("created ACP session = %q, want %q", created.SessionID, fixture.SessionID)
	}
	firstPrompt := requireACPResponse(t, first.stdout, "3")
	if firstPrompt.Error == nil || firstPrompt.Error.Code != -32001 {
		t.Fatalf("first prompt response = %#v, want controlled -32001 error", firstPrompt)
	}

	secondFixture := fixture
	secondFixture.RetryAttemptDirectory = secondAttemptDir
	second := runACPHelperConversation(t, helperPath, secondFixture, retryConversation(true, created.SessionID))
	loadResponse := requireACPResponse(t, second.stdout, "2")
	if loadResponse.Error != nil {
		t.Fatalf("session/load response = %#v", loadResponse.Error)
	}
	secondPrompt := requireACPResponse(t, second.stdout, "3")
	if secondPrompt.Error != nil || len(secondPrompt.Result) == 0 {
		t.Fatalf("resumed prompt response = %#v, want successful result", secondPrompt)
	}
	if first.pid <= 0 || second.pid <= 0 || first.pid == second.pid {
		t.Fatalf("helper process IDs = %d and %d, want two distinct started processes", first.pid, second.pid)
	}
	if first.exitCode != 0 || second.exitCode != 0 {
		t.Fatalf("helper process exit codes = %d and %d, want 0", first.exitCode, second.exitCode)
	}

	if observationPath != "" {
		validateRetryObservationTrace(t, localTracePath)
	} else if _, err := os.Stat(localTracePath); !os.IsNotExist(err) {
		t.Fatalf("ACP trace without opt-in: stat error = %v, want not-exist", err)
	}
}

type acpHelperFixture struct {
	Kind                  string `json:"kind"`
	Mode                  string `json:"mode"`
	SessionID             string `json:"sessionId,omitempty"`
	RetryAttemptDirectory string `json:"retryAttemptDirectory,omitempty"`
	ObservationPath       string `json:"observationPath,omitempty"`
}

type acpHelperRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type acpHelperProcessResult struct {
	pid      int
	exitCode int
	stdout   []byte
}

type acpObservationRecord struct {
	PID       int                     `json:"pid"`
	Phase     string                  `json:"phase"`
	Attempt   int                     `json:"attempt"`
	Direction string                  `json:"direction,omitempty"`
	Method    string                  `json:"method,omitempty"`
	ID        json.RawMessage         `json:"id,omitempty"`
	Result    string                  `json:"result,omitempty"`
	Error     *acpObservationRPCError `json:"error,omitempty"`
	ExitCode  *int                    `json:"exitCode,omitempty"`
}

type acpObservationRPCError struct {
	Code int `json:"code"`
}

func requirePrebuiltACPHelper(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(acpHelperArtifactPathEnv))
	if path == "" {
		if os.Getenv(acpHelperArtifactNeedEnv) == "1" {
			t.Fatalf("%s is required", acpHelperArtifactPathEnv)
		}
		t.Skipf("prebuilt ACP helper unavailable; set %s", acpHelperArtifactPathEnv)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt ACP helper: %v", err)
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		t.Fatalf("inspect prebuilt ACP helper: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("prebuilt ACP helper %q is not a regular file", absPath)
	}
	wantHash := strings.TrimSpace(os.Getenv(acpHelperArtifactHashEnv))
	if wantHash == "" {
		t.Fatalf("%s is required with the prebuilt ACP helper", acpHelperArtifactHashEnv)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read prebuilt ACP helper: %v", err)
	}
	gotHash := sha256.Sum256(data)
	if got := hex.EncodeToString(gotHash[:]); !strings.EqualFold(got, wantHash) {
		t.Fatalf("prebuilt ACP helper SHA-256 = %s, want %s", got, wantHash)
	}
	t.Logf("prebuilt ACP helper sha256=%s", wantHash)
	return absPath
}

func configuredObservationExportPath(t *testing.T) string {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv(acpObservationExportEnv))
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("ACP observation export directory must be absolute: %q", dir)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create private ACP observation export directory: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restrict ACP observation export directory permissions: %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("inspect ACP observation export directory: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		t.Fatalf("ACP observation export path must be a new directory: %q", dir)
	}
	return filepath.Join(dir, "rpc.jsonl")
}

func writeAttemptMarker(t *testing.T, dir, attempt string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create ACP attempt directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, attempt), []byte("started"), 0o600); err != nil {
		t.Fatalf("write ACP attempt marker: %v", err)
	}
}

func retryConversation(resume bool, sessionID string) string {
	initialization, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	var sessionRequest []byte
	if resume {
		sessionRequest, _ = json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "session/load", "params": map[string]any{"sessionId": sessionID},
		})
	} else {
		sessionRequest, _ = json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": map[string]any{},
		})
	}
	prompt, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/prompt", "params": map[string]any{
			"sessionId": sessionID,
			"prompt":    []any{map[string]any{"type": "text", "text": "complete retry"}},
		},
	})
	return string(initialization) + "\n" + string(sessionRequest) + "\n" + string(prompt) + "\n"
}

func runACPHelperConversation(
	t *testing.T,
	helperPath string,
	fixture acpHelperFixture,
	conversation string,
) acpHelperProcessResult {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode ACP helper fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), acpHelperProcessCeiling)
	defer cancel()
	command := exec.CommandContext(ctx, helperPath,
		"-test.run=^TestACPAgentHelperProcess$",
		"--",
		acpHelperFixtureArg+base64.RawURLEncoding.EncodeToString(encoded),
	)
	command.Stdin = strings.NewReader(conversation)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("run prebuilt ACP helper: %v; stderr=%q stdout=%q", err, stderr.String(), stdout.String())
	}
	if command.Process == nil || command.ProcessState == nil {
		t.Fatal("ACP helper did not expose a completed process state")
	}
	return acpHelperProcessResult{
		pid:      command.Process.Pid,
		exitCode: command.ProcessState.ExitCode(),
		stdout:   append([]byte(nil), stdout.Bytes()...),
	}
}

func requireACPResponse(t *testing.T, output []byte, requestID string) acpHelperRPCMessage {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var message acpHelperRPCMessage
		if err := json.Unmarshal(line, &message); err != nil {
			continue
		}
		if string(message.ID) == requestID {
			return message
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read ACP helper response: %v", err)
	}
	t.Fatalf("ACP helper output omitted response ID %s: %q", requestID, output)
	return acpHelperRPCMessage{}
}

func validateRetryObservationTrace(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read opted-in ACP observation trace: %v", err)
	}
	if len(data) == 0 || len(data) > acpObservationMaxBytes {
		t.Fatalf("ACP observation trace size = %d, want 1..%d bytes", len(data), acpObservationMaxBytes)
	}
	var records []acpObservationRecord
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var record acpObservationRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode ACP observation record: %v", err)
		}
		records = append(records, record)
	}
	starts := map[int]int{}
	exits := map[int]int{}
	firstFailure, secondResume, secondSuccess := false, false, false
	for _, record := range records {
		if record.PID <= 0 {
			t.Fatalf("ACP observation record omitted process ID: %#v", record)
		}
		switch record.Phase {
		case "start":
			starts[record.Attempt] = record.PID
		case "exit":
			if record.Result != "success" || record.ExitCode == nil || *record.ExitCode != 0 {
				t.Fatalf("ACP helper attempt %d lifecycle exit = %#v", record.Attempt, record)
			}
			exits[record.Attempt] = record.PID
		case "rpc":
			if record.Result != "notification" && len(record.ID) == 0 {
				t.Fatalf("ACP RPC record omitted request id: %#v", record)
			}
			switch {
			case record.Attempt == 1 && record.Direction == "peer_to_client" && record.Method == "session/prompt" && record.Error != nil && record.Error.Code == -32001:
				firstFailure = true
			case record.Attempt == 2 && record.Direction == "client_to_peer" && record.Method == "session/load":
				secondResume = true
			case record.Attempt == 2 && record.Direction == "peer_to_client" && record.Method == "session/prompt" && record.Result == "success":
				secondSuccess = true
			}
		}
	}
	if starts[1] == 0 || starts[2] == 0 || exits[1] == 0 || exits[2] == 0 {
		t.Fatalf("ACP helper lifecycle attempts = starts %#v exits %#v, want attempts 1 and 2", starts, exits)
	}
	if starts[1] != exits[1] || starts[2] != exits[2] || starts[1] == starts[2] {
		t.Fatalf("ACP helper lifecycle PIDs = starts %#v exits %#v, want paired, distinct process IDs", starts, exits)
	}
	if !firstFailure || !secondResume || !secondSuccess {
		t.Fatalf("ACP observation trace omitted failed prompt, exact-session resume, or successful retry: %#v", records)
	}
}
