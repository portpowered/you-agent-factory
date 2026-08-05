// Package realclient_test proves the external ACP-client process boundary.
package realclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	pinnedAcpxPackage           = "acpx@0.13.0"
	pinnedAcpxVersion           = "0.13.0"
	realClientAgentName         = "you-real-client"
	defaultFactoryBuilderTarget = "factory:@you/factory-builder"
	realClientEvidenceEnv       = "INFINITE_YOU_RUN_ACPX_REAL_CLIENT"
	deterministicProviderName   = "codex"
	providerObservationEnv      = "INFINITE_YOU_ACPX_PROVIDER_OBSERVATION"
)

// TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt proves the OS-process
// boundary that root.BuildProcess tests intentionally cannot cover: a pinned,
// independently packaged ACP client builds and starts the repository binary
// through a portable structured argv configuration, then completes one prompt
// through the shipped default target. The disposable provider command is a
// Codex-shaped external process, not an in-process fake: it reports only its
// provider identity and emits the provider-native success envelope. The test
// keeps every ACPX file, npm cache, server binary, provider marker, and ACP
// session inside t.TempDir and reports only phase classifications if that
// client boundary fails.
func TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("real acpx client evidence builds the CLI and installs a pinned npm package")
	}
	if os.Getenv(realClientEvidenceEnv) != "1" {
		t.Skip("set INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1 to run pinned real-client ACP evidence")
	}
	if !nodeSupportsPinnedAcpx() {
		t.Skip("pinned acpx@0.13.0 requires Node.js 22.13.0 or later")
	}

	scenario := newPinnedAcpxScenario(t)
	buildCurrentYouBinary(t, scenario)
	scenario.writeDeterministicProvider(t)
	scenario.writeConfig(t)
	scenario.assertPinnedVersion(t)
	scenario.registerSessionCleanup(t)

	created := scenario.newSession(t)
	assertCreatedSession(t, created)
	assertNegotiatedDefaultTarget(t, scenario)
	promptOutput := scenario.prompt(t)
	scenario.assertOneDeterministicProviderInvocation(t)
	assertPromptEvidence(t, promptOutput)
	scenario.closeSession(t)
	scenario.assertQueueOwnerStopped(t)
}

type pinnedAcpxScenario struct {
	home          string
	project       string
	npmCache      string
	repoRoot      string
	serverPath    string
	providerDir   string
	providerProof string
	sessionActive bool
}

func newPinnedAcpxScenario(t *testing.T) *pinnedAcpxScenario {
	t.Helper()

	root := t.TempDir()
	serverName := "you"
	if runtime.GOOS == "windows" {
		serverName += ".exe"
	}
	return &pinnedAcpxScenario{
		home:          filepath.Join(root, "home"),
		project:       filepath.Join(root, "project"),
		npmCache:      filepath.Join(root, "npm-cache"),
		repoRoot:      repositoryRoot(t),
		serverPath:    filepath.Join(root, "bin", serverName),
		providerDir:   filepath.Join(root, "provider"),
		providerProof: filepath.Join(root, "provider", "invocations"),
	}
}

func buildCurrentYouBinary(t *testing.T, scenario *pinnedAcpxScenario) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(scenario.serverPath), 0o755); err != nil {
		t.Fatalf("real ACP evidence setup failed: create disposable binary directory")
	}
	if _, err := runBoundedCommand(scenario.repoRoot, nil, "build-current-you", "go", "build", "-o", scenario.serverPath, "./cmd/factory"); err != nil {
		t.Fatal(err)
	}
}

func (scenario *pinnedAcpxScenario) writeConfig(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(scenario.project, 0o755); err != nil {
		t.Fatalf("real ACP evidence setup failed: create disposable project directory")
	}
	config := struct {
		Agents map[string]struct {
			Argv []string `json:"argv"`
		} `json:"agents"`
	}{
		Agents: map[string]struct {
			Argv []string `json:"argv"`
		}{
			realClientAgentName: {Argv: []string{scenario.serverPath, "serve", "acp"}},
		},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("real ACP evidence setup failed: encode structured agent argv")
	}
	if err := os.WriteFile(filepath.Join(scenario.project, ".acpxrc.json"), payload, 0o600); err != nil {
		t.Fatalf("real ACP evidence setup failed: write disposable client configuration")
	}
}

func (scenario *pinnedAcpxScenario) writeDeterministicProvider(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(scenario.providerDir, 0o755); err != nil {
		t.Fatalf("real ACP evidence setup failed: create deterministic provider directory")
	}

	providerPath, providerScript := deterministicProviderScript(scenario.providerDir)
	if err := os.WriteFile(providerPath, []byte(providerScript), 0o700); err != nil {
		t.Fatalf("real ACP evidence setup failed: write deterministic provider command")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(providerPath, 0o700); err != nil {
			t.Fatalf("real ACP evidence setup failed: mark deterministic provider command executable")
		}
	}
}

func deterministicProviderScript(directory string) (string, string) {
	if runtime.GOOS == "windows" {
		return filepath.Join(directory, deterministicProviderName+".cmd"), `@echo off
>> "%INFINITE_YOU_ACPX_PROVIDER_OBSERVATION%" echo codex
echo {"type":"turn.started"}
echo {"type":"item.completed","item":{"id":"real-client-result","type":"agent_message","text":"ok"}}
echo {"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}
`
	}
	return filepath.Join(directory, deterministicProviderName), `#!/bin/sh
printf '%s\n' codex >> "$INFINITE_YOU_ACPX_PROVIDER_OBSERVATION"
printf '%s\n' '{"type":"turn.started"}' '{"type":"item.completed","item":{"id":"real-client-result","type":"agent_message","text":"ok"}}' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
`
}

func (scenario *pinnedAcpxScenario) assertPinnedVersion(t *testing.T) {
	t.Helper()
	output, err := scenario.run("verify-acpx-version", "--version")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != pinnedAcpxVersion {
		t.Fatalf("real ACP evidence version verification failed: effective acpx version %q", got)
	}
}

func (scenario *pinnedAcpxScenario) newSession(t *testing.T) acpxSessionResult {
	t.Helper()
	output, err := scenario.run("create-session", "--format", "json", realClientAgentName, "sessions", "new")
	if err != nil {
		t.Fatal(err)
	}
	scenario.sessionActive = true
	return parseAcpxSessionResult(t, output, "session_ensured")
}

func (scenario *pinnedAcpxScenario) prompt(t *testing.T) []byte {
	t.Helper()
	// Keep the input ephemeral and semantically empty. The assertion below
	// retains only result-presence and terminal facts, never prompt or result
	// payloads from the real-client output.
	output, err := scenario.run(
		"complete-prompt",
		"--format", "json",
		"--timeout", "45",
		realClientAgentName,
		"prompt",
		strings.Repeat("x", 16),
	)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func (scenario *pinnedAcpxScenario) closeSession(t *testing.T) {
	t.Helper()
	output, err := scenario.run("close-session", "--format", "json", realClientAgentName, "sessions", "close")
	if err != nil {
		t.Fatal(err)
	}
	scenario.sessionActive = false
	if closed := parseAcpxSessionResult(t, output, "session_closed"); closed.ACPSessionID == "" {
		t.Fatal("real ACP evidence cleanup failed: closed session has no ACP session identity")
	}
}

func (scenario *pinnedAcpxScenario) registerSessionCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if !scenario.sessionActive {
			return
		}
		if _, err := scenario.run("cleanup-session", "--format", "json", realClientAgentName, "sessions", "close"); err != nil {
			t.Error(err)
		}
	})
}

func (scenario *pinnedAcpxScenario) run(phase string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--yes", "--package", pinnedAcpxPackage, "acpx"}, args...)
	return runBoundedCommand(scenario.project, scenario.environment(), phase, "npx", commandArgs...)
}

func (scenario *pinnedAcpxScenario) environment() []string {
	environment := append([]string(nil), os.Environ()...)
	for name, value := range map[string]string{
		"HOME":                              scenario.home,
		"USERPROFILE":                       scenario.home,
		"APPDATA":                           filepath.Join(scenario.home, "appdata"),
		"LOCALAPPDATA":                      filepath.Join(scenario.home, "localappdata"),
		"XDG_CONFIG_HOME":                   filepath.Join(scenario.home, "config"),
		"XDG_CACHE_HOME":                    filepath.Join(scenario.home, "cache"),
		"npm_config_cache":                  scenario.npmCache,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER": deterministicProviderName,
		providerObservationEnv:              scenario.providerProof,
		"PATH":                              scenario.providerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	} {
		environment = replaceEnvironmentValue(environment, name, value)
	}
	return environment
}

func replaceEnvironmentValue(environment []string, name, value string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if found && strings.EqualFold(key, name) {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, name+"="+value)
}

func runBoundedCommand(directory string, environment []string, phase, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("real ACP evidence failed during %s: timeout", phase)
	}
	return nil, fmt.Errorf("real ACP evidence failed during %s: non-zero exit", phase)
}

func nodeSupportsPinnedAcpx() bool {
	output, err := exec.Command("node", "--version").Output()
	if err != nil {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(output)), "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	return majorErr == nil && minorErr == nil && (major > 22 || major == 22 && minor >= 13)
}

type acpxSessionResult struct {
	Action       string `json:"action"`
	Created      bool   `json:"created"`
	RecordID     string `json:"acpxRecordId"`
	ACPSessionID string `json:"acpxSessionId"`
}

func parseAcpxSessionResult(t *testing.T, output []byte, action string) acpxSessionResult {
	t.Helper()
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		var result acpxSessionResult
		if err := json.Unmarshal(line, &result); err != nil {
			t.Fatalf("real ACP evidence failed: acpx %s output was not machine-readable JSON", action)
		}
		if result.Action == action {
			return result
		}
	}
	t.Fatalf("real ACP evidence failed: acpx did not report %s", action)
	return acpxSessionResult{}
}

func assertCreatedSession(t *testing.T, created acpxSessionResult) {
	t.Helper()
	if !created.Created || created.RecordID == "" || created.ACPSessionID == "" {
		t.Fatal("real ACP evidence failed: session creation did not report both client and ACP session identities")
	}
}

func assertNegotiatedDefaultTarget(t *testing.T, scenario *pinnedAcpxScenario) {
	t.Helper()
	record := scenario.loadSessionRecord(t)
	if len(record.ProtocolVersion) == 0 {
		t.Fatal("real ACP evidence failed: initialized session has no negotiated protocol version")
	}
	for _, option := range record.ACP.ConfigOptions {
		if option.ID == "target" && option.CurrentValue == defaultFactoryBuilderTarget {
			return
		}
	}
	t.Fatalf("real ACP evidence failed: current target was not %s", defaultFactoryBuilderTarget)
}

func assertPromptEvidence(t *testing.T, output []byte) {
	t.Helper()

	var assistantResult bool
	terminalStopReasons := make([]string, 0, 1)
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Update struct {
					SessionUpdate string `json:"sessionUpdate"`
					Content       struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"update"`
			} `json:"params"`
			Result struct {
				StopReason string `json:"stopReason"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &frame); err != nil {
			t.Fatal("real ACP evidence failed: acpx prompt output was not machine-readable JSON")
		}
		if len(frame.Error) > 0 && string(frame.Error) != "null" {
			t.Fatal("real ACP evidence failed: acpx prompt reported a protocol error")
		}
		if frame.Method == "session/update" &&
			frame.Params.Update.SessionUpdate == "agent_message_chunk" &&
			frame.Params.Update.Content.Type == "text" &&
			strings.TrimSpace(frame.Params.Update.Content.Text) != "" {
			assistantResult = true
		}
		if frame.Method == "" && frame.Result.StopReason != "" {
			terminalStopReasons = append(terminalStopReasons, frame.Result.StopReason)
		}
	}
	if !assistantResult {
		t.Fatal("real ACP evidence failed: prompt did not produce a non-empty assistant result")
	}
	if len(terminalStopReasons) != 1 || terminalStopReasons[0] != "end_turn" {
		t.Fatal("real ACP evidence failed: prompt did not return exactly one successful end_turn result")
	}
}

func (scenario *pinnedAcpxScenario) assertOneDeterministicProviderInvocation(t *testing.T) {
	t.Helper()
	payload, err := os.ReadFile(scenario.providerProof)
	if err != nil {
		t.Fatal("real ACP evidence failed: read deterministic provider observation")
	}
	invocations := 0
	for _, line := range strings.Fields(string(payload)) {
		if line != deterministicProviderName {
			t.Fatal("real ACP evidence failed: provider observation did not identify the deterministic provider")
		}
		invocations++
	}
	if invocations != 1 {
		t.Fatalf("real ACP evidence failed: deterministic provider invocation count = %d, want 1", invocations)
	}
}

func (scenario *pinnedAcpxScenario) assertQueueOwnerStopped(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(scenario.home, ".acpx", "queues"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal("real ACP evidence cleanup failed: inspect disposable queue state")
	}
	if len(entries) != 0 {
		t.Fatal("real ACP evidence cleanup failed: disposable acpx queue owner remained active")
	}
}

type persistedAcpxSession struct {
	ProtocolVersion json.RawMessage `json:"protocol_version"`
	ACP             struct {
		ConfigOptions []struct {
			ID           string `json:"id"`
			CurrentValue string `json:"currentValue"`
		} `json:"config_options"`
	} `json:"acpx"`
}

func (scenario *pinnedAcpxScenario) loadSessionRecord(t *testing.T) persistedAcpxSession {
	t.Helper()
	directory := filepath.Join(scenario.home, ".acpx", "sessions")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal("real ACP evidence failed: read disposable session state")
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "index.json" || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			t.Fatal("real ACP evidence failed: read disposable session record")
		}
		var record persistedAcpxSession
		if unmarshalErr := json.Unmarshal(payload, &record); unmarshalErr != nil {
			t.Fatal("real ACP evidence failed: decode disposable session record")
		}
		return record
	}
	t.Fatal("real ACP evidence failed: acpx did not retain a disposable session record")
	return persistedAcpxSession{}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal("real ACP evidence setup failed: locate repository root")
	}
	for {
		if _, err = os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("real ACP evidence setup failed: repository root not found")
		}
		directory = parent
	}
}
