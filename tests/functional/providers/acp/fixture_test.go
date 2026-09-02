package acp_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ACP functional peers receive their non-secret controls as an invocation-
// scoped argument. This keeps two child processes in one test binary from
// sharing mutable process environment while preserving the real process and
// stdio JSON-RPC boundary. Production environment witnesses deliberately stay
// on the environment path: ACP_TEST_API_TOKEN and YOU_ACP_GOLDEN_SENTINEL
// must continue to flow through the real provider invocation environment.
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
	HelperStartMarkerPath         string `json:"helperStartMarkerPath,omitempty"`
	HelperExitMarkerPath          string `json:"helperExitMarkerPath,omitempty"`
	HelperReadyMarkerPath         string `json:"helperReadyMarkerPath,omitempty"`
}

var acpFunctionalModes = map[string]struct{}{
	"1": {}, "fail": {}, "auth": {}, "model": {}, "package-conformance": {},
	"resource": {}, "content": {}, "version": {}, "init-fail": {}, "stderr": {},
	"malformed": {}, "eof": {}, "block": {}, "isolate": {}, "unsupported": {},
	"persistent": {}, "serialize": {}, "crash-once": {}, "spawn": {},
	"tournament": {}, "cancelled-response": {}, "resume": {}, "resume-not-found": {},
	"retry-resume": {}, "disconnect-once": {}, "shared-spine": {},
}

var acpGoldenModes = map[string]struct{}{
	"success": {}, "new-fail": {}, "config-fail": {}, "permission-reject": {},
	"permission-allow": {},
}

type acpFixturePathField struct {
	name  string
	value func(acpFixtureConfig) string
}

var acpAbsolutePathFields = []acpFixturePathField{
	{name: "retryAttemptDirectory", value: func(config acpFixtureConfig) string { return config.RetryAttemptDirectory }},
	{name: "retryHoldPath", value: func(config acpFixtureConfig) string { return config.RetryHoldPath }},
	{name: "disconnectMarkerPath", value: func(config acpFixtureConfig) string { return config.DisconnectMarkerPath }},
	{name: "disconnectReadyPath", value: func(config acpFixtureConfig) string { return config.DisconnectReadyPath }},
	{name: "disconnectReleasePath", value: func(config acpFixtureConfig) string { return config.DisconnectReleasePath }},
	{name: "packageConformanceReleasePath", value: func(config acpFixtureConfig) string { return config.PackageConformanceReleasePath }},
	{name: "crashMarkerPath", value: func(config acpFixtureConfig) string { return config.CrashMarkerPath }},
	{name: "promptSignalPath", value: func(config acpFixtureConfig) string { return config.PromptSignalPath }},
	{name: "promptReleasePath", value: func(config acpFixtureConfig) string { return config.PromptReleasePath }},
	{name: "helperStartMarkerPath", value: func(config acpFixtureConfig) string { return config.HelperStartMarkerPath }},
	{name: "helperExitMarkerPath", value: func(config acpFixtureConfig) string { return config.HelperExitMarkerPath }},
	{name: "helperReadyMarkerPath", value: func(config acpFixtureConfig) string { return config.HelperReadyMarkerPath }},
}

func validateACPFixture(config acpFixtureConfig, raw map[string]json.RawMessage) error {
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
	allowed := acpFunctionalModes
	if config.Kind == acpFixtureKindGolden {
		allowed = acpGoldenModes
	}
	if _, ok := allowed[config.Mode]; !ok {
		return fmt.Errorf("acp fixture mode %q is not valid for kind %q", config.Mode, config.Kind)
	}
	for _, field := range acpAbsolutePathFields {
		value, present := raw[field.name]
		if !present {
			continue
		}
		if len(value) == 0 || string(value) == "null" {
			return fmt.Errorf("acp fixture %s must be a non-empty absolute path", field.name)
		}
		if field.value(config) == "" {
			return fmt.Errorf("acp fixture %s must be a non-empty absolute path", field.name)
		}
		if !filepath.IsAbs(field.value(config)) {
			return fmt.Errorf("acp fixture %s = %q must be an absolute path", field.name, field.value(config))
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
	if encoded == "" {
		return config, fmt.Errorf("acp fixture payload is empty")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return config, fmt.Errorf("decode acp fixture base64url payload: %w", err)
	}
	if base64.RawURLEncoding.EncodeToString(data) != encoded {
		return config, fmt.Errorf("acp fixture base64url payload is not canonical unpadded base64url")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("decode acp fixture JSON payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return config, fmt.Errorf("acp fixture JSON payload has trailing data")
		}
		return config, fmt.Errorf("decode trailing acp fixture JSON payload: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return config, fmt.Errorf("decode acp fixture JSON object: %w", err)
	}
	if err := validateACPFixture(config, raw); err != nil {
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

// acpFixtureChildArgs renders the test selector and invocation-scoped
// fixture payload. The flag terminator keeps the fixture out of test.v's
// command-line parsing while leaving it visible to the child entrypoint.
func acpFixtureChildArgs(entrypointTest string, fixture acpFixtureConfig) []string {
	return []string{
		"-test.run=^" + entrypointTest + "$",
		"--",
		acpFixtureFlagPrefix + encodeACPFixture(fixture),
	}
}

// loadACPFixtureFromArgs returns no fixture for an ordinary test-binary
// invocation. A child started with a fixture receives exactly one payload;
// duplicate payloads are rejected before the peer can write JSON-RPC.
func loadACPFixtureFromArgs() (config acpFixtureConfig, present bool, err error) {
	for _, arg := range os.Args {
		if !strings.HasPrefix(arg, acpFixtureFlagPrefix) {
			continue
		}
		if present {
			return acpFixtureConfig{}, true, fmt.Errorf("acp fixture payload was supplied more than once")
		}
		present = true
		config, err = decodeACPFixture(strings.TrimPrefix(arg, acpFixtureFlagPrefix))
	}
	return config, present, err
}
