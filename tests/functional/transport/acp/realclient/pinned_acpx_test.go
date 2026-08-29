// Package realclient_test proves the external ACP-client process boundary.
package realclient_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This package intentionally keeps all three real-client cases isolated: the
// two helper cases own process-tree timeout/non-zero cleanup, while the pinned
// ACPX case owns executable selection, stdio negotiation, and session teardown.
// The normal-path direct-start accounting is seven: the Node prerequisite,
// revision lookup, build, and four ACPX phases. Starts performed internally by
// Go, npx, ACPX, or the built executable are outside that count.
const (
	pinnedAcpxPackage           = "acpx@0.13.0"
	pinnedAcpxVersion           = "0.13.0"
	realClientAgentName         = "you-real-client"
	defaultFactoryBuilderTarget = "factory:@you/factory-builder"
	realClientEvidenceEnv       = "INFINITE_YOU_RUN_ACPX_REAL_CLIENT"
	realClientRequiredEnv       = "INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT"
	realClientEvidenceOutputEnv = "INFINITE_YOU_ACPX_EVIDENCE_OUTPUT"
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
	requirePinnedAcpxPrerequisites(t)

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
	scenario.assertDeterministicProviderInvocations(t)
	assertPromptEvidence(t, promptOutput)
	scenario.closeSession(t)
	scenario.assertQueueOwnerStopped(t)
	scenario.emitEvidence(t)
}

func requirePinnedAcpxPrerequisites(t *testing.T) {
	t.Helper()
	required := os.Getenv(realClientRequiredEnv) == "1"
	if testing.Short() {
		if required {
			t.Fatal("real ACP evidence prerequisite failed: required lane must run without -short")
		}
		t.Skip("real acpx client evidence builds the CLI and installs a pinned npm package")
	}
	if os.Getenv(realClientEvidenceEnv) != "1" {
		if required {
			t.Fatal("real ACP evidence prerequisite failed: required lane did not enable the pinned client")
		}
		t.Skip("set INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1 to run pinned real-client ACP evidence")
	}
	if !nodeSupportsPinnedAcpx() {
		if required {
			t.Fatal("real ACP evidence prerequisite failed: Node.js 22.13.0 or later is required")
		}
		t.Skip("pinned acpx@0.13.0 requires Node.js 22.13.0 or later")
	}
}

type pinnedAcpxScenario struct {
	home            string
	project         string
	npmCache        string
	temporary       string
	repoRoot        string
	revision        string
	serverPath      string
	providerDir     string
	providerProof   string
	sessionMayExist bool
}

type realClientEvidence struct {
	Revision            string `json:"revision"`
	AcpxVersion         string `json:"acpxVersion"`
	Initialization      string `json:"initialization"`
	Session             string `json:"session"`
	DefaultTarget       string `json:"defaultTarget"`
	AssistantResult     string `json:"assistantResult"`
	TerminalStopReason  string `json:"terminalStopReason"`
	Provider            string `json:"provider"`
	ProviderInvocations int    `json:"providerInvocations"`
	Cleanup             string `json:"cleanup"`
	Outcome             string `json:"outcome"`
}

func newPinnedAcpxScenario(t *testing.T) *pinnedAcpxScenario {
	t.Helper()

	root := t.TempDir()
	serverName := "you"
	if runtime.GOOS == "windows" {
		serverName += ".exe"
	}
	scenario := &pinnedAcpxScenario{
		home:          filepath.Join(root, "home"),
		project:       filepath.Join(root, "project"),
		npmCache:      filepath.Join(root, "npm-cache"),
		temporary:     filepath.Join(root, "temporary"),
		repoRoot:      repositoryRoot(t),
		serverPath:    filepath.Join(root, "bin", serverName),
		providerDir:   filepath.Join(root, "provider"),
		providerProof: filepath.Join(root, "provider", "invocations"),
	}
	for _, directory := range []string{
		scenario.home,
		scenario.npmCache,
		scenario.temporary,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal("real ACP evidence setup failed: create disposable runtime directory")
		}
	}
	scenario.revision = repositoryRevision(t, scenario.repoRoot)
	return scenario
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
			realClientAgentName: {Argv: []string{scenario.serverPath, "server", "acp"}},
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
	// The client can create the session before it writes an invalid or
	// incomplete machine-readable result. Mark cleanup as necessary before the
	// request so t.Cleanup still closes any session that may have been created.
	scenario.sessionMayExist = true
	output, err := scenario.run("create-session", "--format", "json", realClientAgentName, "sessions", "new")
	if err != nil {
		t.Fatal(err)
	}
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
	if closed := parseAcpxSessionResult(t, output, "session_closed"); closed.ACPSessionID == "" {
		t.Fatal("real ACP evidence cleanup failed: closed session has no ACP session identity")
	}
	scenario.sessionMayExist = false
}

func (scenario *pinnedAcpxScenario) registerSessionCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if scenario.sessionMayExist {
			if _, err := scenario.run("cleanup-session", "--format", "json", realClientAgentName, "sessions", "close"); err != nil {
				t.Error("real ACP evidence cleanup failed: close disposable session")
			} else {
				scenario.sessionMayExist = false
			}
		}
		if err := scenario.queueOwnerStopped(); err != nil {
			t.Error(err)
		}
	})
}

func (scenario *pinnedAcpxScenario) emitEvidence(t *testing.T) {
	t.Helper()
	evidence := realClientEvidence{
		Revision:            scenario.revision,
		AcpxVersion:         pinnedAcpxVersion,
		Initialization:      "negotiated",
		Session:             "created",
		DefaultTarget:       defaultFactoryBuilderTarget,
		AssistantResult:     "non-empty",
		TerminalStopReason:  "end_turn",
		Provider:            deterministicProviderName,
		ProviderInvocations: 1,
		Cleanup:             "complete",
		Outcome:             "passed",
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal("real ACP evidence failed: encode sanitized terminal facts")
	}
	if outputPath := strings.TrimSpace(os.Getenv(realClientEvidenceOutputEnv)); outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			t.Fatal("real ACP evidence failed: create sanitized evidence directory")
		}
		if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
			t.Fatal("real ACP evidence failed: retain sanitized terminal facts")
		}
	}
	t.Logf(
		"real ACP evidence: revision=%s acpx=%s initialization=negotiated session=created target=%s assistant_result=non-empty terminal=end_turn provider=%s provider_invocations=1 cleanup=complete outcome=passed",
		scenario.revision,
		pinnedAcpxVersion,
		defaultFactoryBuilderTarget,
		deterministicProviderName,
	)
}

func (scenario *pinnedAcpxScenario) run(phase string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--yes", "--package", pinnedAcpxPackage, "acpx"}, args...)
	return runBoundedCommand(scenario.project, scenario.environment(), phase, "npx", commandArgs...)
}

func (scenario *pinnedAcpxScenario) environment() []string {
	return allowlistedEnvironment(map[string]string{
		"HOME":                              scenario.home,
		"USERPROFILE":                       scenario.home,
		"APPDATA":                           filepath.Join(scenario.home, "appdata"),
		"LOCALAPPDATA":                      filepath.Join(scenario.home, "localappdata"),
		"XDG_CONFIG_HOME":                   filepath.Join(scenario.home, "config"),
		"XDG_CACHE_HOME":                    filepath.Join(scenario.home, "cache"),
		"npm_config_cache":                  scenario.npmCache,
		"TMP":                               scenario.temporary,
		"TEMP":                              scenario.temporary,
		"TMPDIR":                            scenario.temporary,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER": deterministicProviderName,
		providerObservationEnv:              scenario.providerProof,
		"PATH":                              scenario.providerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
}

func allowlistedEnvironment(overrides map[string]string) []string {
	environment := make([]string, 0, len(overrides)+6)
	if _, hasPathOverride := overrides["PATH"]; !hasPathOverride {
		environment = append(environment, "PATH="+os.Getenv("PATH"))
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"ComSpec", "PATHEXT", "SystemDrive", "SystemRoot", "WINDIR"} {
			if value := os.Getenv(name); value != "" {
				environment = append(environment, name+"="+value)
			}
		}
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		environment = append(environment, name+"="+overrides[name])
	}
	return environment
}

func runBoundedCommand(directory string, environment []string, phase, name string, args ...string) ([]byte, error) {
	return runBoundedCommandWithTimeout(directory, environment, phase, time.Minute, name, args...)
}

func runBoundedCommandWithTimeout(directory string, environment []string, phase string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	configureCommandProcessTree(command)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("real ACP evidence failed during %s: launch", phase)
	}
	tree, err := attachCommandProcessTree(command)
	if err != nil {
		_ = terminateCommandProcessTree(command, nil)
		_ = command.Wait()
		return nil, fmt.Errorf("real ACP evidence failed during %s: process ownership", phase)
	}
	defer closeCommandProcessTree(command, tree)
	completed := make(chan error, 1)
	go func() {
		completed <- command.Wait()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-completed:
		if err == nil {
			return output.Bytes(), nil
		}
		if cleanupErr := terminateCommandProcessTree(command, tree); cleanupErr != nil {
			return nil, fmt.Errorf("real ACP evidence failed during %s: non-zero exit cleanup", phase)
		}
		return nil, fmt.Errorf("real ACP evidence failed during %s: non-zero exit", phase)
	case <-timer.C:
		cleanupErr := terminateCommandProcessTree(command, tree)
		<-completed
		if cleanupErr != nil {
			return nil, fmt.Errorf("real ACP evidence failed during %s: timeout cleanup", phase)
		}
		return nil, fmt.Errorf("real ACP evidence failed during %s: timeout", phase)
	}
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

	// A Worker is a tool call, so everything it produces -- messages,
	// reasoning, tool activity -- must arrive as content inside that call, and
	// the assistant message must come from the Factory rather than from a
	// Worker speaking directly. Counting both proves the routing end to end
	// against a real third-party client rather than only against our own.
	var assistantResult bool
	var workerToolCalls, workerToolUpdates int
	terminalStopReasons := make([]string, 0, 1)
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		// content is deliberately raw: ACP gives it a different shape per
		// update kind -- an object for agent_message_chunk, an array for
		// tool_call_update. Decoding it eagerly into one shape makes a valid
		// frame of the other kind look like malformed output.
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Update struct {
					SessionUpdate string          `json:"sessionUpdate"`
					Content       json.RawMessage `json:"content"`
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
		if frame.Method == "session/update" && frame.Params.Update.SessionUpdate == "agent_message_chunk" {
			var content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(frame.Params.Update.Content, &content); err != nil {
				t.Fatal("real ACP evidence failed: assistant message carried an unreadable content block")
			}
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				assistantResult = true
			}
		}
		if frame.Method == "session/update" {
			switch frame.Params.Update.SessionUpdate {
			case "tool_call":
				workerToolCalls++
			case "tool_call_update":
				workerToolUpdates++
			}
		}
		if frame.Method == "" && frame.Result.StopReason != "" {
			terminalStopReasons = append(terminalStopReasons, frame.Result.StopReason)
		}
	}
	if !assistantResult {
		t.Fatal("real ACP evidence failed: prompt did not produce a non-empty assistant result")
	}
	if workerToolCalls == 0 {
		t.Fatal("real ACP evidence failed: the dispatched Worker did not open a tool call")
	}
	if workerToolUpdates == 0 {
		t.Fatal("real ACP evidence failed: the Worker produced no content inside its tool call")
	}
	if len(terminalStopReasons) != 1 || terminalStopReasons[0] != "end_turn" {
		t.Fatal("real ACP evidence failed: prompt did not return exactly one successful end_turn result")
	}
}

func (scenario *pinnedAcpxScenario) assertDeterministicProviderInvocations(t *testing.T) {
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
	// Two, not one: the default Factory classifies the request and then answers
	// it, so a single prompt legitimately makes two model calls. The count is
	// still asserted exactly, because an unbounded number would mean the
	// Factory looped rather than routed.
	if invocations != 2 {
		t.Fatalf("real ACP evidence failed: deterministic provider invocation count = %d, want 2 (classify, then answer)", invocations)
	}
}

func (scenario *pinnedAcpxScenario) assertQueueOwnerStopped(t *testing.T) {
	t.Helper()
	if err := scenario.queueOwnerStopped(); err != nil {
		t.Fatal(err)
	}
}

func (scenario *pinnedAcpxScenario) queueOwnerStopped() error {
	entries, err := os.ReadDir(filepath.Join(scenario.home, ".acpx", "queues"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("real ACP evidence cleanup failed: inspect disposable queue state")
	}
	if len(entries) != 0 {
		return errors.New("real ACP evidence cleanup failed: disposable acpx queue owner remained active")
	}
	return nil
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

func repositoryRevision(t *testing.T, root string) string {
	t.Helper()
	output, err := runBoundedCommand(root, allowlistedEnvironment(nil), "read-revision", "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal("real ACP evidence setup failed: resolve repository revision")
	}
	revision := strings.TrimSpace(string(output))
	if len(revision) != 40 {
		t.Fatal("real ACP evidence setup failed: repository revision was not a full safe identifier")
	}
	for _, character := range revision {
		if !strings.ContainsRune("0123456789abcdef", character) {
			t.Fatal("real ACP evidence setup failed: repository revision was not a full safe identifier")
		}
	}
	return revision
}
