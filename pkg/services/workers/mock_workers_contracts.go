package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
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

// Clone returns a detached copy suitable for carrying one mock-worker
// selection into a request-scoped execution. The Workers root never retains a
// caller-owned config or its nested mutable values.
func (config *MockWorkersConfig) Clone() *MockWorkersConfig {
	if config == nil {
		return nil
	}
	clone := &MockWorkersConfig{
		UnmatchedDispatchPolicy: config.UnmatchedDispatchPolicy,
		MockWorkers:             make([]MockWorkerConfig, len(config.MockWorkers)),
	}
	for index, worker := range config.MockWorkers {
		clone.MockWorkers[index] = worker
		clone.MockWorkers[index].WorkInputs = append(
			[]MockWorkInputSelector(nil),
			worker.WorkInputs...,
		)
		if worker.ScriptConfig != nil {
			script := *worker.ScriptConfig
			script.Args = append([]string(nil), worker.ScriptConfig.Args...)
			script.Env = cloneStringMap(worker.ScriptConfig.Env)
			clone.MockWorkers[index].ScriptConfig = &script
		}
		if worker.RejectConfig != nil {
			reject := *worker.RejectConfig
			if worker.RejectConfig.ExitCode != nil {
				exitCode := *worker.RejectConfig.ExitCode
				reject.ExitCode = &exitCode
			}
			clone.MockWorkers[index].RejectConfig = &reject
		}
	}
	return clone
}

type MockWorkersConfigFileSystem interface{ ReadFile(string) ([]byte, error) }
type MockWorkersConfigLoader func(string) (*MockWorkersConfig, error)

// MockWorkersConfigDecodeDiagnostics contains safe metadata produced while
// decoding one mock-worker configuration. It records ignored paths only; the
// ignored values are never retained for logging or persistence.
type MockWorkersConfigDecodeDiagnostics struct {
	IgnoredJSONPaths []string
}

// Paths returns a detached, deterministic copy of ignored JSON paths.
func (diagnostics MockWorkersConfigDecodeDiagnostics) Paths() []string {
	return sortedMockWorkersJSONPaths(diagnostics.IgnoredJSONPaths)
}

// MockWorkersConfigDiagnosticsLoader is the optional diagnostics-aware loader
// used by customer-facing callers that need to warn about ignored fields.
type MockWorkersConfigDiagnosticsLoader func(string) (*MockWorkersConfig, MockWorkersConfigDecodeDiagnostics, error)

func NewMockWorkersConfigLoader(files MockWorkersConfigFileSystem) (MockWorkersConfigLoader, error) {
	load, err := NewMockWorkersConfigDiagnosticsLoader(files)
	if err != nil {
		return nil, err
	}
	return func(path string) (*MockWorkersConfig, error) {
		config, _, err := load(path)
		return config, err
	}, nil
}

// NewMockWorkersConfigDiagnosticsLoader constructs the Workers-owned loader
// that retains safe ignored-field paths for an operational caller.
func NewMockWorkersConfigDiagnosticsLoader(
	files MockWorkersConfigFileSystem,
) (MockWorkersConfigDiagnosticsLoader, error) {
	if files == nil {
		return nil, fmt.Errorf("Workers mock-worker config filesystem is required")
	}
	return func(path string) (*MockWorkersConfig, MockWorkersConfigDecodeDiagnostics, error) {
		if path == "" {
			return NewEmptyMockWorkersConfig(), MockWorkersConfigDecodeDiagnostics{}, nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return nil, MockWorkersConfigDecodeDiagnostics{}, fmt.Errorf("read mock workers config %s: %w", path, err)
		}
		config, diagnostics, err := ParseMockWorkersConfigWithDiagnostics(data)
		if err != nil {
			return nil, MockWorkersConfigDecodeDiagnostics{}, fmt.Errorf("parse mock workers config %s: %w", path, err)
		}
		return config, diagnostics, nil
	}, nil
}

// ParseMockWorkersConfig validates raw JSON into the normalized runtime
// mock-worker configuration. Unknown object fields are ignored; callers that
// need safe paths for warnings should use ParseMockWorkersConfigWithDiagnostics.
func ParseMockWorkersConfig(data []byte) (*MockWorkersConfig, error) {
	config, _, err := ParseMockWorkersConfigWithDiagnostics(data)
	return config, err
}

// ParseMockWorkersConfigWithDiagnostics validates one mock-worker JSON
// document and reports sorted unique paths for unknown object fields. Known
// field types, run-type validation, and exactly-one-document enforcement stay
// strict.
func ParseMockWorkersConfigWithDiagnostics(
	data []byte,
) (*MockWorkersConfig, MockWorkersConfigDecodeDiagnostics, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	config := NewEmptyMockWorkersConfig()
	if err := decoder.Decode(config); err != nil {
		return nil, MockWorkersConfigDecodeDiagnostics{}, fmt.Errorf("decode mock workers JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, MockWorkersConfigDecodeDiagnostics{}, fmt.Errorf("decode mock workers JSON: unexpected trailing JSON")
	}
	if config.MockWorkers == nil {
		config.MockWorkers = []MockWorkerConfig{}
	}
	if err := config.Validate(); err != nil {
		return nil, MockWorkersConfigDecodeDiagnostics{}, err
	}
	paths, err := collectMockWorkersJSONPaths(data)
	if err != nil {
		return nil, MockWorkersConfigDecodeDiagnostics{}, fmt.Errorf("decode mock workers JSON: %w", err)
	}
	return config, MockWorkersConfigDecodeDiagnostics{IgnoredJSONPaths: paths}, nil
}

var mockWorkersRawMessageType = reflect.TypeOf(json.RawMessage{})

func collectMockWorkersJSONPaths(data []byte) ([]string, error) {
	value, err := decodeOneMockWorkersJSONValue(data)
	if err != nil {
		return nil, err
	}
	var paths []string
	collectMockWorkersJSONPathsForType(value, reflect.TypeOf(MockWorkersConfig{}), "$", &paths)
	return sortedMockWorkersJSONPaths(paths), nil
}

func decodeOneMockWorkersJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func collectMockWorkersJSONPathsForType(value any, typ reflect.Type, path string, paths *[]string) {
	if value == nil || typ == nil {
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == mockWorkersRawMessageType {
		return
	}

	switch typ.Kind() {
	case reflect.Map:
		collectMockWorkersJSONPathsForMap(value, typ, path, paths)
	case reflect.Slice, reflect.Array:
		collectMockWorkersJSONPathsForSequence(value, typ, path, paths)
	case reflect.Struct:
		collectMockWorkersJSONPathsForStruct(value, typ, path, paths)
	}
}

func collectMockWorkersJSONPathsForMap(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok || typ.Key().Kind() != reflect.String {
		return
	}
	for key, child := range object {
		collectMockWorkersJSONPathsForType(child, typ.Elem(), appendMockWorkersJSONPath(path, key), paths)
	}
}

func collectMockWorkersJSONPathsForSequence(value any, typ reflect.Type, path string, paths *[]string) {
	values, ok := value.([]any)
	if !ok {
		return
	}
	for index, item := range values {
		collectMockWorkersJSONPathsForType(item, typ.Elem(), path+"["+strconv.Itoa(index)+"]", paths)
	}
}

func collectMockWorkersJSONPathsForStruct(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	fields := mockWorkersJSONFieldTypes(typ)
	for key, child := range object {
		fieldPath := appendMockWorkersJSONPath(path, key)
		fieldType, known := fields[strings.ToLower(key)]
		if !known {
			*paths = append(*paths, fieldPath)
			continue
		}
		collectMockWorkersJSONPathsForType(child, fieldType, fieldPath, paths)
	}
}

func mockWorkersJSONFieldTypes(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = field.Name
		}
		fields[strings.ToLower(name)] = field.Type
	}
	return fields
}

func appendMockWorkersJSONPath(path, key string) string {
	if isSimpleMockWorkersJSONPathKey(key) {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

func isSimpleMockWorkersJSONPathKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9' && index > 0) ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func sortedMockWorkersJSONPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
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
