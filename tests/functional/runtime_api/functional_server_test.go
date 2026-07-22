package runtime_api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type FunctionalServer struct {
	*support.FunctionalAPIServer
}

type runtimeOption func(*support.FunctionalAPIServerConfig)

func withProvider(provider workerprovider.Provider) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Edges.ProviderOverride = provider
	}
}

func withWorkerCommands(providerRunner, scriptRunner platformprocess.CommandRunner) runtimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) {
		cfg.Edges.ProviderCommandRunner = providerRunner
		cfg.Edges.ScriptCommandRunner = scriptRunner
	}
}

func (fs *FunctionalServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()

	body, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name:         functionalServerStringPtr("functional-server-submit"),
		WorkTypeName: workTypeID,
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	resp, err := http.Post(
		support.DefaultSessionWorkURL(fs.URL(), "/work"),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

func functionalServerStringPtr(value string) *string { return &value }

func (fs *FunctionalServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return support.ListDefaultSessionWork(t, fs.URL())
}

func StartFunctionalServerWithArgs(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	runArgs []string,
	runtimeOptions ...runtimeOption,
) *FunctionalServer {
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
	return &FunctionalServer{FunctionalAPIServer: support.StartFunctionalAPIServer(t, cfg)}
}

func StartFunctionalServer(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	runtimeOptions ...runtimeOption,
) *FunctionalServer {
	t.Helper()
	return StartFunctionalServerWithArgs(t, factoryDir, useMockWorkers, nil, runtimeOptions...)
}
