package mockworkers_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	. "github.com/portpowered/infinite-you/pkg/services/workers/internal/interface"
)

type mockWorkersConfigReader func(string) ([]byte, error)

func (read mockWorkersConfigReader) ReadFile(path string) ([]byte, error) {
	return read(path)
}

func TestParseMockWorkersConfig_ValidConfigPreservesSelectorsAndRunTypeOptions(t *testing.T) {
	cfg, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [
			{
				"id": "accept-reviewer",
				"workerName": "reviewer",
				"workstationName": "review",
				"workInputs": [
					{
						"workId": "work-1",
						"workType": "story",
						"state": "in-review",
						"inputName": "story",
						"traceId": "trace-1",
						"channel": "default",
						"payloadHash": "sha256:test"
					}
				],
				"runType": "accept"
			},
			{
				"id": "script-executor",
				"workerName": "executor",
				"workstationName": "execute",
				"runType": "script",
				"scriptConfig": {
					"command": "go",
					"args": ["test", "./..."],
					"env": {"AGENT_FACTORY_MOCK": "1"},
					"workingDirectory": "/tmp/work",
					"stdin": "script input",
					"timeout": "30s"
				}
			},
			{
				"id": "reject-reviewer",
				"workerName": "reviewer",
				"workstationName": "review",
				"runType": "reject",
				"rejectConfig": {
					"stdout": "review output",
					"stderr": "needs changes",
					"exitCode": 7
				}
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig returned error: %v", err)
	}

	if len(cfg.MockWorkers) != 3 {
		t.Fatalf("mock worker count = %d, want 3", len(cfg.MockWorkers))
	}

	assertMockWorkerAcceptEntry(t, cfg.MockWorkers[0])
	assertMockWorkerScriptEntry(t, cfg.MockWorkers[1])
	assertMockWorkerRejectEntry(t, cfg.MockWorkers[2])
}

func TestParseMockWorkersConfig_PreservesMockUsagePresenceAndExplicitZeroes(t *testing.T) {
	cfg, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [{
			"id": "priced-accept",
			"runType": "accept",
			"usage": {
				"provider": "codex",
				"model": "gpt-5-codex",
				"inputTokens": 0,
				"outputTokens": 500,
				"cachedInputTokens": 0,
				"reasoningOutputTokens": 0
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig returned error: %v", err)
	}
	usage := cfg.MockWorkers[0].Usage
	if usage == nil || usage.Provider != "codex" || usage.Model != "gpt-5-codex" {
		t.Fatalf("usage = %#v, want provider and model", usage)
	}
	for name, value := range map[string]*int64{
		"inputTokens":           usage.InputTokens,
		"outputTokens":          usage.OutputTokens,
		"cachedInputTokens":     usage.CachedInputTokens,
		"reasoningOutputTokens": usage.ReasoningOutputTokens,
	} {
		if value == nil {
			t.Fatalf("usage.%s is nil, want explicit value", name)
		}
	}
	if *usage.InputTokens != 0 || *usage.CachedInputTokens != 0 || *usage.ReasoningOutputTokens != 0 || *usage.OutputTokens != 500 {
		t.Fatalf("usage token values = %#v, want explicit zeroes and output 500", usage)
	}

	clone := cfg.Clone()
	*clone.MockWorkers[0].Usage.InputTokens = 9
	if *cfg.MockWorkers[0].Usage.InputTokens != 0 {
		t.Fatal("Clone mutated the original usage declaration")
	}
}

func TestParseMockWorkersConfig_PreservesAndClonesGate(t *testing.T) {
	dir := t.TempDir()
	arrivedFile := filepath.Join(dir, "arrived")
	releaseFile := filepath.Join(dir, "release")
	payload, err := json.Marshal(map[string]any{
		"mockWorkers": []any{map[string]any{
			"runType": "accept",
			"gateConfig": map[string]any{
				"arrivedFile": arrivedFile,
				"releaseFile": releaseFile,
				"timeout":     "15s",
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal gate config: %v", err)
	}

	cfg, err := ParseMockWorkersConfig(payload)
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig returned error: %v", err)
	}
	gate := cfg.MockWorkers[0].GateConfig
	if gate == nil || gate.ArrivedFile != arrivedFile || gate.ReleaseFile != releaseFile || gate.Timeout != "15s" {
		t.Fatalf("gateConfig = %#v, want parsed paths and timeout", gate)
	}
	clone := cfg.Clone()
	clone.MockWorkers[0].GateConfig.Timeout = "1s"
	if cfg.MockWorkers[0].GateConfig.Timeout != "15s" {
		t.Fatal("Clone mutated the original gate declaration")
	}
}

func TestParseMockWorkersConfig_RejectsInvalidGate(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "absolute")
	cases := []struct {
		name    string
		gate    MockWorkerGateConfig
		message string
	}{
		{name: "missing arrival", gate: MockWorkerGateConfig{ReleaseFile: absolute, Timeout: "1s"}, message: "arrivedFile is required"},
		{name: "relative arrival", gate: MockWorkerGateConfig{ArrivedFile: "arrived", ReleaseFile: absolute, Timeout: "1s"}, message: "arrivedFile must be absolute"},
		{name: "same files", gate: MockWorkerGateConfig{ArrivedFile: absolute, ReleaseFile: absolute, Timeout: "1s"}, message: "must be different"},
		{name: "missing timeout", gate: MockWorkerGateConfig{ArrivedFile: absolute + "-arrived", ReleaseFile: absolute, Timeout: ""}, message: "timeout must be a duration"},
		{name: "zero timeout", gate: MockWorkerGateConfig{ArrivedFile: absolute + "-arrived", ReleaseFile: absolute, Timeout: "0s"}, message: "timeout must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{RunType: MockWorkerRunTypeAccept, GateConfig: &tc.gate}}}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Validate error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}

func TestParseMockWorkersConfig_RejectsInvalidMockUsage(t *testing.T) {
	cases := []struct {
		name    string
		usage   string
		message string
	}{
		{name: "missing provider", usage: `{"provider":"","model":"gpt-5"}`, message: "usage: provider is required"},
		{name: "missing model", usage: `{"provider":"codex","model":" "}`, message: "usage: model is required"},
		{name: "negative input", usage: `{"provider":"codex","model":"gpt-5","inputTokens":-1}`, message: "inputTokens must be non-negative"},
		{name: "cached without input", usage: `{"provider":"codex","model":"gpt-5","cachedInputTokens":1}`, message: "inputTokens is required when cachedInputTokens is set"},
		{name: "cached exceeds input", usage: `{"provider":"codex","model":"gpt-5","inputTokens":1,"cachedInputTokens":2}`, message: "cachedInputTokens must not exceed inputTokens"},
		{name: "reasoning without output", usage: `{"provider":"codex","model":"gpt-5","reasoningOutputTokens":1}`, message: "outputTokens is required when reasoningOutputTokens is set"},
		{name: "reasoning exceeds output", usage: `{"provider":"codex","model":"gpt-5","outputTokens":1,"reasoningOutputTokens":2}`, message: "reasoningOutputTokens must not exceed outputTokens"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseMockWorkersConfig([]byte(`{"mockWorkers":[{"runType":"accept","usage":` + tc.usage + `}]}`))
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}

func TestParseMockWorkersConfig_OmittedMockUsageRemainsAbsent(t *testing.T) {
	cfg, err := ParseMockWorkersConfig([]byte(`{"mockWorkers":[{"runType":"accept"}]}`))
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig returned error: %v", err)
	}
	if cfg.MockWorkers[0].Usage != nil {
		t.Fatalf("usage = %#v, want nil when omitted", cfg.MockWorkers[0].Usage)
	}
}

func TestParseMockWorkersConfig_AcceptsUnmatchedDispatchPolicyValues(t *testing.T) {
	for _, policy := range []string{"", "accept", "passthrough"} {
		t.Run(policy, func(t *testing.T) {
			payload := `{"mockWorkers":[]`
			if policy != "" {
				payload += `,"unmatchedDispatchPolicy":"` + policy + `"`
			}
			payload += `}`

			cfg, err := ParseMockWorkersConfig([]byte(payload))
			if err != nil {
				t.Fatalf("ParseMockWorkersConfig returned error: %v", err)
			}
			want := MockWorkerUnmatchedDispatchPolicy(policy)
			if cfg.UnmatchedDispatchPolicy != want {
				t.Fatalf("unmatchedDispatchPolicy = %q, want %q", cfg.UnmatchedDispatchPolicy, want)
			}
		})
	}
}

func TestParseMockWorkersConfig_RejectsUnknownUnmatchedDispatchPolicy(t *testing.T) {
	_, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [],
		"unmatchedDispatchPolicy": "maybe"
	}`))
	if err == nil {
		t.Fatal("expected unknown unmatchedDispatchPolicy to fail validation")
	}
	if !strings.Contains(err.Error(), `unmatchedDispatchPolicy must be one of "accept" or "passthrough"; got "maybe"`) {
		t.Fatalf("error = %q, want actionable unmatchedDispatchPolicy message", err)
	}
}

func TestParseMockWorkersConfig_RejectsUnknownRunTypeWithActionableError(t *testing.T) {
	_, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [
			{"id": "bad", "runType": "maybe"}
		]
	}`))
	if err == nil {
		t.Fatal("expected unknown runType to fail validation")
	}
	if !strings.Contains(err.Error(), `runType must be one of "accept", "script", or "reject"; got "maybe"`) {
		t.Fatalf("error = %q, want actionable runType message", err)
	}
}

func TestParseMockWorkersConfig_RejectsScriptEntryWithoutScriptConfig(t *testing.T) {
	_, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [
			{"id": "script", "runType": "script"}
		]
	}`))
	if err == nil {
		t.Fatal("expected script runType without scriptConfig to fail validation")
	}
	if !strings.Contains(err.Error(), "scriptConfig is required") {
		t.Fatalf("error = %q, want missing scriptConfig message", err)
	}
}

func TestParseMockWorkersConfig_RejectsScriptEntryWithoutCommand(t *testing.T) {
	_, err := ParseMockWorkersConfig([]byte(`{
		"mockWorkers": [
			{"id": "script", "runType": "script", "scriptConfig": {"args": ["ok"]}}
		]
	}`))
	if err == nil {
		t.Fatal("expected script runType without scriptConfig.command to fail validation")
	}
	if !strings.Contains(err.Error(), "scriptConfig.command is required") {
		t.Fatalf("error = %q, want missing scriptConfig.command message", err)
	}
}

func TestParseMockWorkersConfig_RejectsRejectEntryWithInvalidExitCode(t *testing.T) {
	for _, exitCode := range []string{"-1", "0", "256"} {
		t.Run(exitCode, func(t *testing.T) {
			_, err := ParseMockWorkersConfig([]byte(`{
				"mockWorkers": [
					{"id": "reject", "runType": "reject", "rejectConfig": {"exitCode": ` + exitCode + `}}
				]
			}`))
			if err == nil {
				t.Fatal("expected invalid reject exit code to fail validation")
			}
			if !strings.Contains(err.Error(), "rejectConfig.exitCode must be between 1 and 255") {
				t.Fatalf("error = %q, want invalid exit-code message", err)
			}
		})
	}
}

func TestLoadMockWorkersConfig_EmptyPathReturnsEmptyDefaultAcceptConfig(t *testing.T) {
	cfg, err := localMockWorkersConfigLoader(t)("")
	if err != nil {
		t.Fatalf("LoadMockWorkersConfig empty path returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadMockWorkersConfig empty path returned nil config")
	}
	if len(cfg.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default config", len(cfg.MockWorkers))
	}
}

func TestLoadMockWorkersConfig_LoadsConfigFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(path, []byte(`{"mockWorkers":[{"id":"accepted","runType":"accept"}]}`), 0o644); err != nil {
		t.Fatalf("write mock config: %v", err)
	}

	cfg, err := localMockWorkersConfigLoader(t)(path)
	if err != nil {
		t.Fatalf("LoadMockWorkersConfig returned error: %v", err)
	}
	if len(cfg.MockWorkers) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(cfg.MockWorkers))
	}
	if cfg.MockWorkers[0].ID != "accepted" || cfg.MockWorkers[0].RunType != MockWorkerRunTypeAccept {
		t.Fatalf("mock worker = %#v, want loaded accept entry", cfg.MockWorkers[0])
	}
}

func TestMockWorkersConfigLoaderForwardsExactPathToInjectedFileSystem(t *testing.T) {
	t.Parallel()

	const selectedPath = "selected/mock-workers.json"
	var gotPath string
	load, err := NewMockWorkersConfigLoader(mockWorkersConfigReader(func(path string) ([]byte, error) {
		gotPath = path
		return []byte(`{"mockWorkers":[]}`), nil
	}))
	if err != nil {
		t.Fatalf("construct loader: %v", err)
	}
	cfg, err := load(selectedPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if gotPath != selectedPath {
		t.Fatalf("read path = %q, want %q", gotPath, selectedPath)
	}
	if cfg == nil || len(cfg.MockWorkers) != 0 {
		t.Fatalf("config = %#v, want empty normalized config", cfg)
	}
}

func TestMockWorkersConfigLoaderFailsClosedWithoutFileSystem(t *testing.T) {
	t.Parallel()

	load, err := NewMockWorkersConfigLoader(nil)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("missing filesystem error = %v", err)
	}
	if load != nil {
		t.Fatalf("missing filesystem loader = %v, want nil", load)
	}
}

func TestMockWorkersConfigLoaderPreservesInjectedReadFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("read failed")
	load, err := NewMockWorkersConfigLoader(mockWorkersConfigReader(func(string) ([]byte, error) {
		return nil, want
	}))
	if err != nil {
		t.Fatalf("construct loader: %v", err)
	}
	_, err = load("mock-workers.json")
	if !errors.Is(err, want) {
		t.Fatalf("read error = %v, want wrapped %v", err, want)
	}
}

func TestDocsExampleMockWorkersConfig_ParsesAsSupportedConfig(t *testing.T) {
	path := testpath.MustRepoPathFromCaller(t, 0, "docs", "examples", "mock-workers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs example mock workers config: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("%s is not valid JSON", path)
	}

	cfg, err := ParseMockWorkersConfig(data)
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig(%s): %v", path, err)
	}
	if len(cfg.MockWorkers) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(cfg.MockWorkers))
	}

	worker := cfg.MockWorkers[0]
	if worker.WorkerName != "reviewer" ||
		worker.WorkstationName != "review-story" ||
		worker.RunType != MockWorkerRunTypeReject ||
		len(worker.WorkInputs) != 1 ||
		worker.WorkInputs[0].WorkType != "story" ||
		worker.WorkInputs[0].State != "in-review" ||
		worker.WorkInputs[0].InputName != "work" {
		t.Fatalf("docs example worker = %#v, want targeted reject entry", worker)
	}
	if worker.RejectConfig == nil || worker.RejectConfig.ExitCode == nil || *worker.RejectConfig.ExitCode != 42 {
		t.Fatalf("docs example rejectConfig = %#v, want exit code 42", worker.RejectConfig)
	}
}

func TestDocsExampleMockWorkersScriptConfig_ParsesAsSupportedConfig(t *testing.T) {
	path := testpath.MustRepoPathFromCaller(t, 0, "docs", "examples", "mock-workers-script.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs example script mock workers config: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("%s is not valid JSON", path)
	}

	cfg, err := ParseMockWorkersConfig(data)
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig(%s): %v", path, err)
	}
	if len(cfg.MockWorkers) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(cfg.MockWorkers))
	}

	worker := cfg.MockWorkers[0]
	if worker.WorkerName != "executor" ||
		worker.WorkstationName != "execute-story" ||
		worker.RunType != MockWorkerRunTypeScript ||
		worker.ScriptConfig == nil ||
		worker.ScriptConfig.Command != "printf" ||
		worker.ScriptConfig.Timeout != "30s" {
		t.Fatalf("docs example script worker = %#v, want targeted script entry", worker)
	}
}

func TestDocsExampleMockWorkersMixedConfig_ParsesAsSupportedConfig(t *testing.T) {
	path := testpath.MustRepoPathFromCaller(t, 0, "docs", "examples", "mock-workers-mixed.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs example mixed mock workers config: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("%s is not valid JSON", path)
	}

	cfg, err := ParseMockWorkersConfig(data)
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig(%s): %v", path, err)
	}
	if cfg.UnmatchedDispatchPolicy != MockWorkerUnmatchedDispatchPolicyPassthrough {
		t.Fatalf("unmatchedDispatchPolicy = %q, want %q", cfg.UnmatchedDispatchPolicy, MockWorkerUnmatchedDispatchPolicyPassthrough)
	}
	if len(cfg.MockWorkers) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(cfg.MockWorkers))
	}

	worker := cfg.MockWorkers[0]
	if worker.WorkerName != "reviewer" ||
		worker.WorkstationName != "review-story" ||
		worker.RunType != MockWorkerRunTypeReject {
		t.Fatalf("docs example mixed worker = %#v, want targeted reject entry", worker)
	}
}

func assertMockWorkerAcceptEntry(t *testing.T, worker MockWorkerConfig) {
	t.Helper()

	if worker.ID != "accept-reviewer" ||
		worker.WorkerName != "reviewer" ||
		worker.WorkstationName != "review" ||
		worker.RunType != MockWorkerRunTypeAccept {
		t.Fatalf("accept entry = %#v, want selectors and accept run type preserved", worker)
	}
	if len(worker.WorkInputs) != 1 {
		t.Fatalf("accept work input count = %d, want 1", len(worker.WorkInputs))
	}
	input := worker.WorkInputs[0]
	if input.WorkID != "work-1" ||
		input.WorkType != "story" ||
		input.State != "in-review" ||
		input.InputName != "story" ||
		input.TraceID != "trace-1" ||
		input.Channel != "default" ||
		input.PayloadHash != "sha256:test" {
		t.Fatalf("accept work input = %#v, want all selectors preserved", input)
	}
}

func assertMockWorkerScriptEntry(t *testing.T, worker MockWorkerConfig) {
	t.Helper()

	if worker.RunType != MockWorkerRunTypeScript {
		t.Fatalf("script run type = %q, want %q", worker.RunType, MockWorkerRunTypeScript)
	}
	if worker.ScriptConfig == nil {
		t.Fatal("scriptConfig was not preserved")
	}
	if worker.ScriptConfig.Command != "go" ||
		strings.Join(worker.ScriptConfig.Args, " ") != "test ./..." ||
		worker.ScriptConfig.Env["AGENT_FACTORY_MOCK"] != "1" ||
		worker.ScriptConfig.WorkingDirectory != "/tmp/work" ||
		worker.ScriptConfig.Stdin != "script input" ||
		worker.ScriptConfig.Timeout != "30s" {
		t.Fatalf("script config = %#v, want command options preserved", worker.ScriptConfig)
	}
}

func assertMockWorkerRejectEntry(t *testing.T, worker MockWorkerConfig) {
	t.Helper()

	if worker.RunType != MockWorkerRunTypeReject {
		t.Fatalf("reject run type = %q, want %q", worker.RunType, MockWorkerRunTypeReject)
	}
	if worker.RejectConfig == nil {
		t.Fatal("rejectConfig was not preserved")
	}
	if worker.RejectConfig.Stdout != "review output" ||
		worker.RejectConfig.Stderr != "needs changes" ||
		worker.RejectConfig.ExitCode == nil ||
		*worker.RejectConfig.ExitCode != 7 {
		t.Fatalf("reject config = %#v, want stdout, stderr, and exit code preserved", worker.RejectConfig)
	}
}
