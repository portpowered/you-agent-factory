package acp_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The ACP functional peers were previously configured through process-global
// environment variables, which made every real-process cell mutually exclusive
// inside one test process invocation. This file defines the package-local,
// invocation-scoped launch contract instead: each spawned peer receives an
// unpadded base64url-encoded JSON configuration as a trailing argument, so two
// peers with different modes can run concurrently under -parallel=2 without
// sharing mutable process state.
//
// Production-environment witnesses deliberately stay on the environment path:
// ACP_TEST_API_TOKEN (workstation-configured secret redaction) and
// YOU_ACP_GOLDEN_SENTINEL (invocation environment propagation) must keep
// flowing through the real production invocation environment.

const (
	acpFixtureKindFunctional = "functional"
	acpFixtureKindGolden     = "golden"

	acpFixtureFlagPrefix = "-acp-fixture="
)

type acpFixtureConfig struct {
	Kind      string `json:"kind"`
	Mode      string `json:"mode"`
	SessionID string `json:"sessionId,omitempty"`

	RetryAttemptDirectory         string `json:"retryAttemptDirectory,omitempty"`
	RetryHoldPath                 string `json:"retryHoldPath,omitempty"`
	DisconnectMarkerPath          string `json:"disconnectMarkerPath,omitempty"`
	DisconnectReadyPath           string `json:"disconnectReadyPath,omitempty"`
	DisconnectReleasePath         string `json:"disconnectReleasePath,omitempty"`
	PackageConformanceReleasePath string `json:"packageConformanceReleasePath,omitempty"`
	CrashMarkerPath               string `json:"crashMarkerPath,omitempty"`
	PromptSignalPath              string `json:"promptSignalPath,omitempty"`
	PromptReleasePath             string `json:"promptReleasePath,omitempty"`
	ContentSentinel               string `json:"contentSentinel,omitempty"`
}

var acpFunctionalModes = map[string]bool{
	"1": true, "fail": true, "auth": true, "model": true, "package-conformance": true,
	"resource": true, "content": true, "version": true, "init-fail": true, "stderr": true,
	"malformed": true, "eof": true, "block": true, "isolate": true, "unsupported": true,
	"persistent": true, "serialize": true, "crash-once": true, "spawn": true,
	"tournament": true, "cancelled-response": true, "resume": true, "resume-not-found": true,
	"retry-resume": true, "disconnect-once": true,
}

var acpGoldenModes = map[string]bool{
	"success": true, "new-fail": true, "config-fail": true,
	"permission-reject": true, "permission-allow": true,
}

// acpAbsolutePathFields lists every optional field that must name an absolute
// path when present, because the peer resolves these locations independently
// of its working directory.
var acpAbsolutePathFields = []struct {
	name  string
	value func(config acpFixtureConfig) string
}{
	{"retryAttemptDirectory", func(c acpFixtureConfig) string { return c.RetryAttemptDirectory }},
	{"retryHoldPath", func(c acpFixtureConfig) string { return c.RetryHoldPath }},
	{"disconnectMarkerPath", func(c acpFixtureConfig) string { return c.DisconnectMarkerPath }},
	{"disconnectReadyPath", func(c acpFixtureConfig) string { return c.DisconnectReadyPath }},
	{"disconnectReleasePath", func(c acpFixtureConfig) string { return c.DisconnectReleasePath }},
	{"packageConformanceReleasePath", func(c acpFixtureConfig) string { return c.PackageConformanceReleasePath }},
	{"crashMarkerPath", func(c acpFixtureConfig) string { return c.CrashMarkerPath }},
	{"promptSignalPath", func(c acpFixtureConfig) string { return c.PromptSignalPath }},
	{"promptReleasePath", func(c acpFixtureConfig) string { return c.PromptReleasePath }},
}

func validateACPFixture(config acpFixtureConfig) error {
	switch config.Kind {
	case "":
		return fmt.Errorf("acp fixture kind is required")
	case acpFixtureKindFunctional, acpFixtureKindGolden:
	default:
		return fmt.Errorf("acp fixture kind %q is not a known peer kind", config.Kind)
	}
	if config.Mode == "" {
		return fmt.Errorf("acp fixture mode is required")
	}
	var allowed map[string]bool
	if config.Kind == acpFixtureKindFunctional {
		allowed = acpFunctionalModes
	} else {
		allowed = acpGoldenModes
	}
	if !allowed[config.Mode] {
		return fmt.Errorf("acp fixture mode %q is not valid for kind %q", config.Mode, config.Kind)
	}
	for _, field := range acpAbsolutePathFields {
		value := field.value(config)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return fmt.Errorf("acp fixture %s = %q must be an absolute path", field.name, value)
		}
	}
	return nil
}

func encodeACPFixture(config acpFixtureConfig) string {
	data, err := json.Marshal(config)
	if err != nil {
		panic(fmt.Sprintf("marshal acp fixture: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeACPFixture(encoded string) (acpFixtureConfig, error) {
	var config acpFixtureConfig
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return config, fmt.Errorf("decode acp fixture base64url payload: %w", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("decode acp fixture JSON payload: %w", err)
	}
	if err := validateACPFixture(config); err != nil {
		return config, err
	}
	return config, nil
}

func functionalACPFixture(mode string) acpFixtureConfig {
	return acpFixtureConfig{Kind: acpFixtureKindFunctional, Mode: mode}
}

func goldenACPFixture(mode string) acpFixtureConfig {
	return acpFixtureConfig{Kind: acpFixtureKindGolden, Mode: mode}
}

// acpFixtureChildArgs renders the trailing arguments carried by a spawned
// peer command: the test-framework selector followed by the invocation-scoped
// fixture payload after the flag terminator.
func acpFixtureChildArgs(entrypointTest string, fixture acpFixtureConfig) []string {
	return []string{
		"-test.run=^" + entrypointTest + "$",
		"--",
		acpFixtureFlagPrefix + encodeACPFixture(fixture),
	}
}

// loadACPFixtureFromArgs extracts the invocation-scoped fixture from the
// current process arguments. present is false when the process was started
// without a fixture payload (an ordinary test-binary invocation).
func loadACPFixtureFromArgs() (config acpFixtureConfig, present bool, err error) {
	for _, arg := range os.Args {
		if !strings.HasPrefix(arg, acpFixtureFlagPrefix) {
			continue
		}
		config, err = decodeACPFixture(strings.TrimPrefix(arg, acpFixtureFlagPrefix))
		return config, true, err
	}
	return acpFixtureConfig{}, false, nil
}

// TestACPFixtureContractRejectsInvalidInvocationScopedData proves, through
// real child processes, that malformed invocation-scoped fixture payloads exit
// non-zero before any JSON-RPC traffic is written, while a valid payload
// decodes to the intended peer configuration.
func TestACPFixtureContractRejectsInvalidInvocationScopedData(t *testing.T) {
	relativePath := filepath.Join("relative", "marker")
	cases := []struct {
		name    string
		payload string
	}{
		{name: "invalid base64url", payload: "not-base64url!!"},
		{name: "invalid JSON", payload: encodeACPFixture(acpFixtureConfig{})[:0] + "{not-json"},
		{name: "missing kind", payload: encodeACPFixture(acpFixtureConfig{Mode: "1"})},
		{name: "unknown kind", payload: encodeACPFixture(acpFixtureConfig{Kind: "acpx", Mode: "1"})},
		{name: "missing mode", payload: encodeACPFixture(acpFixtureConfig{Kind: acpFixtureKindFunctional})},
		{name: "mode kind mismatch", payload: encodeACPFixture(acpFixtureConfig{Kind: acpFixtureKindFunctional, Mode: "success"})},
		{name: "golden mode on functional peer", payload: encodeACPFixture(acpFixtureConfig{Kind: acpFixtureKindGolden, Mode: "fail"})},
		{name: "relative retry directory", payload: encodeACPFixture(acpFixtureConfig{Kind: acpFixtureKindFunctional, Mode: "retry-resume", RetryAttemptDirectory: relativePath})},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestACPAgentHelperProcess$", "--", acpFixtureFlagPrefix+testCase.payload)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("malformed acp fixture exited zero; output=%s", output)
			}
			if strings.Contains(string(output), `"jsonrpc"`) {
				t.Fatalf("malformed acp fixture produced JSON-RPC traffic before rejection: %s", output)
			}
		})
	}

	t.Run("valid functional fixture decodes", func(t *testing.T) {
		fixture := functionalACPFixture("serialize")
		fixture.SessionID = "session-1"
		fixture.PromptSignalPath = filepath.Join(t.TempDir(), "signal")
		decoded, err := decodeACPFixture(encodeACPFixture(fixture))
		if err != nil || decoded != fixture {
			t.Fatalf("decode(encode(fixture)) = %#v, %v; want %#v", decoded, err, fixture)
		}
	})
	t.Run("valid golden fixture selects golden peer", func(t *testing.T) {
		command := exec.Command(os.Args[0], "-test.run=^TestACPGoldenRPCPeerProcess$", "--", acpFixtureFlagPrefix+encodeACPFixture(goldenACPFixture("success")))
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("golden peer rejected a valid invocation-scoped fixture: %v; output=%s", err, output)
		}
	})
}

// TestACPFixtureContractRejectsInvalidInvocationScopedData is declared here to
// keep the contract witness adjacent to the grammar it proves.
var _ = testing.Verbose
