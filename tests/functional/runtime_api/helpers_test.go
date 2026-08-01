package runtime_api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type runtimeOption func(*support.FunctionalAPIServerConfig)

func withSubmissionRecorder(recorder recordings.SubmissionRecorder) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) { cfg.Edges.SubmissionRecorder = recorder }
}

func withClock(clock platformclock.Source) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) { cfg.Edges.Clock = clock }
}

func withProvider(provider workers.Runner) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) { cfg.Edges.ProviderOverride = provider }
}

func withWorkerCommands(providerRunner, scriptRunner platformprocess.CommandRunner) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Edges.ProviderCommandRunner = providerRunner
		cfg.Edges.ScriptCommandRunner = scriptRunner
	}
}

func withInvocationMetricsRecorder(recorder factorysessions.InvocationMetricsRecorder) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Edges.InvocationMetricsRecorder = recorder
	}
}

func withEnvironment(environment []string) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Env = append([]string(nil), environment...)
	}
}

type functionalAPIServer struct {
	*support.FunctionalAPIServer
}

func generatedWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func generatedWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}

func simplePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func persistTestPipelineConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "stage1", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "step1", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}},
			{Name: "finish", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}},
		},
	}
}

func startFunctionalServerWithArgs(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	runArgs []string,
	runtimeOptions ...runtimeOption,
) *functionalAPIServer {
	t.Helper()

	cfg := support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
		Args:                      runArgs,
	}
	for _, option := range runtimeOptions {
		option(&cfg)
	}
	base := support.StartFunctionalAPIServer(t, cfg)
	return &functionalAPIServer{FunctionalAPIServer: base}
}

func startFunctionalServer(t *testing.T, factoryDir string, useMockWorkers bool, runtimeOptions ...runtimeOption) *functionalAPIServer {
	t.Helper()
	return startFunctionalServerWithArgs(t, factoryDir, useMockWorkers, nil, runtimeOptions...)
}

func (fs *functionalAPIServer) SubmitRuntimeWork(t *testing.T, submitted ...work.SubmitRequest) []work.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(submitted)
	for i := range normalized {
		request := normalized[i]
		response := postJSON[factoryapi.SubmitWorkResponse](
			t,
			support.DefaultSessionWorkURL(fs.URL(), "/work"),
			map[string]any{
				"name":                   request.Name,
				"workTypeName":           request.WorkTypeID,
				"payload":                json.RawMessage(request.Payload),
				"traceId":                request.TraceID,
				"currentChainingTraceId": request.CurrentChainingTraceID,
			},
			"submit runtime work",
		)
		if response.WorkId != nil {
			normalized[i].WorkID = *response.WorkId
		}
		normalized[i].RequestID = response.RequestId
		normalized[i].TraceID = response.TraceId
		normalized[i].CurrentChainingTraceID = response.TraceId
	}
	return normalized
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, resp.StatusCode, string(payload))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}

func mustGeneratedFunctionalTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("encode generated text part: %v", err)
	}
	return part
}

func generatedAudioPath(audio factoryapi.WorkAudioContentPart) string {
	if audio.File != nil && strings.TrimSpace(string(*audio.File)) != "" {
		return string(*audio.File)
	}
	if strings.TrimSpace(string(audio.Url)) != "" {
		return strings.TrimPrefix(string(audio.Url), "file://")
	}
	return ""
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
