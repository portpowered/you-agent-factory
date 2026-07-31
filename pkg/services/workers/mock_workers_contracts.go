package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type MockWorkerRunType string

const (
	MockWorkerRunTypeAccept MockWorkerRunType = "accept"
	MockWorkerRunTypeScript MockWorkerRunType = "script"
	MockWorkerRunTypeReject MockWorkerRunType = "reject"
)

type MockWorkerUnmatchedDispatchPolicy string

const (
	MockWorkerUnmatchedDispatchPolicyAccept      MockWorkerUnmatchedDispatchPolicy = "accept"
	MockWorkerUnmatchedDispatchPolicyPassthrough MockWorkerUnmatchedDispatchPolicy = "passthrough"
)

func (policy MockWorkerUnmatchedDispatchPolicy) PassthroughUnmatched() bool {
	return policy == MockWorkerUnmatchedDispatchPolicyPassthrough
}

type MockWorkersConfig struct {
	MockWorkers             []MockWorkerConfig                `json:"mockWorkers"`
	UnmatchedDispatchPolicy MockWorkerUnmatchedDispatchPolicy `json:"unmatchedDispatchPolicy,omitempty"`
}

type MockWorkerConfig struct {
	ID              string                  `json:"id,omitempty"`
	WorkerName      string                  `json:"workerName,omitempty"`
	WorkstationName string                  `json:"workstationName,omitempty"`
	WorkInputs      []MockWorkInputSelector `json:"workInputs,omitempty"`
	RunType         MockWorkerRunType       `json:"runType"`
	ScriptConfig    *MockWorkerScriptConfig `json:"scriptConfig,omitempty"`
	RejectConfig    *MockWorkerRejectConfig `json:"rejectConfig,omitempty"`
}

type MockWorkInputSelector struct {
	WorkID      string `json:"workId,omitempty"`
	WorkType    string `json:"workType,omitempty"`
	State       string `json:"state,omitempty"`
	InputName   string `json:"inputName,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
	Channel     string `json:"channel,omitempty"`
	PayloadHash string `json:"payloadHash,omitempty"`
}

type MockWorkerScriptConfig struct {
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Stdin            string            `json:"stdin,omitempty"`
	Timeout          string            `json:"timeout,omitempty"`
}

type MockWorkerRejectConfig struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

func NewEmptyMockWorkersConfig() *MockWorkersConfig {
	return &MockWorkersConfig{MockWorkers: []MockWorkerConfig{}}
}

type MockWorkersConfigFileSystem interface{ ReadFile(string) ([]byte, error) }
type MockWorkersConfigLoader func(string) (*MockWorkersConfig, error)

func NewMockWorkersConfigLoader(files MockWorkersConfigFileSystem) (MockWorkersConfigLoader, error) {
	if files == nil {
		return nil, fmt.Errorf("Workers mock-worker config filesystem is required")
	}
	return func(path string) (*MockWorkersConfig, error) {
		if path == "" {
			return NewEmptyMockWorkersConfig(), nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read mock workers config %s: %w", path, err)
		}
		config, err := ParseMockWorkersConfig(data)
		if err != nil {
			return nil, fmt.Errorf("parse mock workers config %s: %w", path, err)
		}
		return config, nil
	}, nil
}

func ParseMockWorkersConfig(data []byte) (*MockWorkersConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	config := NewEmptyMockWorkersConfig()
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("decode mock workers JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode mock workers JSON: unexpected trailing JSON")
	}
	if config.MockWorkers == nil {
		config.MockWorkers = []MockWorkerConfig{}
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func (config *MockWorkersConfig) Validate() error {
	if config == nil {
		return fmt.Errorf("mock workers config is required")
	}
	switch config.UnmatchedDispatchPolicy {
	case "", MockWorkerUnmatchedDispatchPolicyAccept, MockWorkerUnmatchedDispatchPolicyPassthrough:
	default:
		return fmt.Errorf(`unmatchedDispatchPolicy must be one of %q or %q; got %q`, MockWorkerUnmatchedDispatchPolicyAccept, MockWorkerUnmatchedDispatchPolicyPassthrough, config.UnmatchedDispatchPolicy)
	}
	for index := range config.MockWorkers {
		if err := config.MockWorkers[index].Validate(); err != nil {
			return fmt.Errorf("mockWorkers[%d]: %w", index, err)
		}
	}
	return nil
}

func (config MockWorkerConfig) Validate() error {
	switch config.RunType {
	case MockWorkerRunTypeAccept:
		return nil
	case MockWorkerRunTypeScript:
		if config.ScriptConfig == nil {
			return fmt.Errorf("scriptConfig is required when runType is %q", MockWorkerRunTypeScript)
		}
		if config.ScriptConfig.Command == "" {
			return fmt.Errorf("scriptConfig.command is required when runType is %q", MockWorkerRunTypeScript)
		}
		return nil
	case MockWorkerRunTypeReject:
		if config.RejectConfig != nil && config.RejectConfig.ExitCode != nil {
			exitCode := *config.RejectConfig.ExitCode
			if exitCode < 1 || exitCode > 255 {
				return fmt.Errorf("rejectConfig.exitCode must be between 1 and 255")
			}
		}
		return nil
	default:
		return fmt.Errorf("runType must be one of %q, %q, or %q; got %q", MockWorkerRunTypeAccept, MockWorkerRunTypeScript, MockWorkerRunTypeReject, config.RunType)
	}
}
