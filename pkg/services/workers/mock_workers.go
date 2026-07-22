package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MockWorkerRunType identifies the deterministic behavior a mock worker entry
// applies when it matches a worker dispatch.
type MockWorkerRunType string

const (
	MockWorkerRunTypeAccept MockWorkerRunType = "accept"
	MockWorkerRunTypeScript MockWorkerRunType = "script"
	MockWorkerRunTypeReject MockWorkerRunType = "reject"
)

// MockWorkerUnmatchedDispatchPolicy controls how mock-worker mode handles
// dispatches that do not match any mockWorkers entry.
type MockWorkerUnmatchedDispatchPolicy string

const (
	// MockWorkerUnmatchedDispatchPolicyAccept returns the default synthetic
	// accepted mock result for unmatched dispatches. This is the zero-value
	// behavior when unmatchedDispatchPolicy is omitted.
	MockWorkerUnmatchedDispatchPolicyAccept MockWorkerUnmatchedDispatchPolicy = "accept"
	// MockWorkerUnmatchedDispatchPolicyPassthrough executes unmatched
	// dispatches through the normal worker runner/provider path.
	MockWorkerUnmatchedDispatchPolicyPassthrough MockWorkerUnmatchedDispatchPolicy = "passthrough"
)

// PassthroughUnmatched reports whether unmatched dispatches should execute
// through the real worker path instead of returning the default accept result.
func (p MockWorkerUnmatchedDispatchPolicy) PassthroughUnmatched() bool {
	return p == MockWorkerUnmatchedDispatchPolicyPassthrough
}

// TODO: remove this and rely on openapi generated schema for mock-workers.
// MockWorkersConfig is the JSON contract for agent-factory mock-worker runs.
type MockWorkersConfig struct {
	MockWorkers             []MockWorkerConfig                `json:"mockWorkers"`
	UnmatchedDispatchPolicy MockWorkerUnmatchedDispatchPolicy `json:"unmatchedDispatchPolicy,omitempty"`
}

// MockWorkerConfig selects a worker dispatch and declares the deterministic
// behavior to apply at the execution boundary.
type MockWorkerConfig struct {
	ID              string                  `json:"id,omitempty"`
	WorkerName      string                  `json:"workerName,omitempty"`
	WorkstationName string                  `json:"workstationName,omitempty"`
	WorkInputs      []MockWorkInputSelector `json:"workInputs,omitempty"`
	RunType         MockWorkerRunType       `json:"runType"`
	ScriptConfig    *MockWorkerScriptConfig `json:"scriptConfig,omitempty"`
	RejectConfig    *MockWorkerRejectConfig `json:"rejectConfig,omitempty"`
}

// MockWorkInputSelector narrows a mock worker match by consumed work input.
type MockWorkInputSelector struct {
	WorkID      string `json:"workId,omitempty"`
	WorkType    string `json:"workType,omitempty"`
	State       string `json:"state,omitempty"`
	InputName   string `json:"inputName,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
	Channel     string `json:"channel,omitempty"`
	PayloadHash string `json:"payloadHash,omitempty"`
}

// MockWorkerScriptConfig declares the command a script mock executes through
// the shared command-runner boundary.
type MockWorkerScriptConfig struct {
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Stdin            string            `json:"stdin,omitempty"`
	Timeout          string            `json:"timeout,omitempty"`
}

// MockWorkerRejectConfig declares observable output for a rejected mock result.
type MockWorkerRejectConfig struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// NewEmptyMockWorkersConfig returns the default mock-worker config used when
// mock mode is enabled without a config file. With no entries, dispatches fall
// through to the runtime's default accept behavior.
func NewEmptyMockWorkersConfig() *MockWorkersConfig {
	return &MockWorkersConfig{MockWorkers: []MockWorkerConfig{}}
}

// MockWorkersConfigFileSystem is the exact filesystem effect needed to load a
// customer-selected mock-worker configuration. Wire supplies the production
// adapter and process tests may replace it at the external edge.
type MockWorkersConfigFileSystem interface {
	ReadFile(string) ([]byte, error)
}

// MockWorkersConfigLoader is the Workers-owned operation that reads and
// validates a customer-selected mock-worker configuration.
type MockWorkersConfigLoader func(path string) (*MockWorkersConfig, error)

// NewMockWorkersConfigLoader constructs the owner operation from its exact
// external effect. The returned operation fails closed when Wire did not
// supply that effect.
func NewMockWorkersConfigLoader(fileSystem MockWorkersConfigFileSystem) (MockWorkersConfigLoader, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("Workers mock-worker config filesystem is required")
	}
	return func(path string) (*MockWorkersConfig, error) {
		return loadMockWorkersConfig(fileSystem, path)
	}, nil
}

func loadMockWorkersConfig(fileSystem MockWorkersConfigFileSystem, path string) (*MockWorkersConfig, error) {
	if path == "" {
		return NewEmptyMockWorkersConfig(), nil
	}
	data, err := fileSystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mock workers config %s: %w", path, err)
	}
	cfg, err := ParseMockWorkersConfig(data)
	if err != nil {
		return nil, fmt.Errorf("parse mock workers config %s: %w", path, err)
	}
	return cfg, nil
}

// ParseMockWorkersConfig validates raw JSON into the normalized runtime
// mock-worker configuration.
func ParseMockWorkersConfig(data []byte) (*MockWorkersConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	cfg := NewEmptyMockWorkersConfig()
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode mock workers JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode mock workers JSON: unexpected trailing JSON")
	}
	if cfg.MockWorkers == nil {
		cfg.MockWorkers = []MockWorkerConfig{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that mock-worker entries are complete for their run type.
func (c *MockWorkersConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("mock workers config is required")
	}
	switch c.UnmatchedDispatchPolicy {
	case "", MockWorkerUnmatchedDispatchPolicyAccept, MockWorkerUnmatchedDispatchPolicyPassthrough:
	default:
		return fmt.Errorf(
			`unmatchedDispatchPolicy must be one of %q or %q; got %q`,
			MockWorkerUnmatchedDispatchPolicyAccept,
			MockWorkerUnmatchedDispatchPolicyPassthrough,
			c.UnmatchedDispatchPolicy,
		)
	}
	for i := range c.MockWorkers {
		if err := c.MockWorkers[i].Validate(); err != nil {
			return fmt.Errorf("mockWorkers[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks a single mock-worker entry.
func (c MockWorkerConfig) Validate() error {
	switch c.RunType {
	case MockWorkerRunTypeAccept:
		return nil
	case MockWorkerRunTypeScript:
		if c.ScriptConfig == nil {
			return fmt.Errorf("scriptConfig is required when runType is %q", MockWorkerRunTypeScript)
		}
		if c.ScriptConfig.Command == "" {
			return fmt.Errorf("scriptConfig.command is required when runType is %q", MockWorkerRunTypeScript)
		}
		return nil
	case MockWorkerRunTypeReject:
		if c.RejectConfig != nil && c.RejectConfig.ExitCode != nil {
			exitCode := *c.RejectConfig.ExitCode
			if exitCode < 1 || exitCode > 255 {
				return fmt.Errorf("rejectConfig.exitCode must be between 1 and 255")
			}
		}
		return nil
	default:
		return fmt.Errorf("runType must be one of %q, %q, or %q; got %q",
			MockWorkerRunTypeAccept,
			MockWorkerRunTypeScript,
			MockWorkerRunTypeReject,
			c.RunType)
	}
}
