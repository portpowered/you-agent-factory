package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	runtimeapifixture "github.com/portpowered/infinite-you/tests/functional/sessions/root_composition/runtime_api_fixture"
)

type runtimeOption func(*support.FunctionalAPIServerConfig)

type runtimeAPIScenario struct {
	provider       any
	providerRunner platformprocess.CommandRunner
	scriptRunner   platformprocess.CommandRunner
	models         []string
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := runtimeapifixture.CloseSharedFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime API package fixture cleanup: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func withProvider(provider any) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		switch provider := provider.(type) {
		case nil:
			cfg.Edges.ProviderOverride = nil
		case providers.Service:
			cfg.Edges.ProviderOverride = provider
		case interface {
			Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
		}:
			cfg.Edges.ProviderOverride = support.ProviderServiceFromInference(provider)
		default:
			panic("withProvider requires a Providers service or legacy test provider")
		}
	}
}

func withWorkerCommands(providerRunner, scriptRunner platformprocess.CommandRunner) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Edges.ProviderCommandRunner = providerRunner
		cfg.Edges.ScriptCommandRunner = scriptRunner
	}
}

func withEnvironment(environment []string) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Env = append([]string(nil), environment...)
	}
}

type functionalAPIServer struct {
	*support.FunctionalAPIServer
	shared    *runtimeapifixture.PackageFixture
	sessionID string
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

func startSharedFunctionalServer(t *testing.T, factoryDir string, scenario runtimeAPIScenario) *functionalAPIServer {
	t.Helper()

	handle := runtimeapifixture.StartSharedFunctionalServer(t, factoryDir, runtimeapifixture.Scenario{
		Provider:       scenario.provider,
		ProviderRunner: scenario.providerRunner,
		ScriptRunner:   scenario.scriptRunner,
		Models:         scenario.models,
	})
	return &functionalAPIServer{
		shared:    handle.Fixture(),
		sessionID: handle.SessionID(),
	}
}

func (fs *functionalAPIServer) URL() string {
	if fs == nil {
		return ""
	}
	if fs.shared != nil {
		return fs.shared.BaseURL()
	}
	if fs.FunctionalAPIServer != nil {
		return fs.FunctionalAPIServer.URL()
	}
	return ""
}

func (fs *functionalAPIServer) sessionURL(path string) string {
	if fs == nil || fs.sessionID == "" {
		return ""
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/factory-sessions/" + url.PathEscape(fs.sessionID) + path
}

func (fs *functionalAPIServer) workURL(path string) string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL(path)
	}
	return support.DefaultSessionWorkURL(fs.URL(), path)
}

func (fs *functionalAPIServer) eventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionEventsURL(fs.URL(), fs.sessionID)
	}
	return support.DefaultSessionEventsURL(fs.URL())
}

func (fs *functionalAPIServer) responseEventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionResponseEventsURL(fs.URL(), fs.sessionID)
	}
	return support.SessionResponseEventsURL(fs.URL(), "~default")
}

func (fs *functionalAPIServer) statusURL() string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL("/status")
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/status"
}

func (fs *functionalAPIServer) StatusURL() string {
	return fs.statusURL()
}

func (fs *functionalAPIServer) Session(t *testing.T) factoryapi.FactorySession {
	t.Helper()
	if fs != nil && fs.shared != nil {
		response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, fs.sessionURL(""))
		session, err := response.AsFactorySession()
		if err != nil {
			t.Fatalf("decode shared Factory Session: %v", err)
		}
		return session
	}
	return support.GetDefaultSession(t, fs.URL())
}

func (fs *functionalAPIServer) GetFactoryEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	if fs != nil && fs.shared != nil {
		return support.GetFactoryEventsForSessionAt(t, fs.URL(), fs.sessionID)
	}
	return support.GetFactoryEventsAt(t, fs.URL())
}

func (fs *functionalAPIServer) openEventStream(t *testing.T) *factoryEventHTTPStream {
	t.Helper()
	stream := openFactoryEventHTTPStream(t, fs.eventsURL())
	if fs != nil && fs.shared != nil {
		stream.setCloseHook(fs.shared.TrackStream())
	}
	return stream
}

func (fs *functionalAPIServer) SubmitRuntimeWork(t *testing.T, submitted ...work.SubmitRequest) []work.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(submitted)
	for i := range normalized {
		request := normalized[i]
		response := postJSON[factoryapi.SubmitWorkResponse](
			t,
			fs.workURL("/work"),
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

func (fs *functionalAPIServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()

	submitted := fs.SubmitRuntimeWork(t, work.SubmitRequest{
		Name:       "functional-server-submit",
		WorkTypeID: workTypeID,
		Payload:    payload,
	})
	if len(submitted) != 1 || submitted[0].TraceID == "" {
		t.Fatalf("POST /work returned %#v, want one trace ID", submitted)
	}
	return submitted[0].TraceID
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
