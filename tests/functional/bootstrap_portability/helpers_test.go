package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type functionalAPIServer struct {
	service *service.FactoryService
	*support.FunctionalAPIServer
}

func startFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *functionalAPIServer {
	t.Helper()
	server := &functionalAPIServer{}
	base := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
		Configure:                 configure,
		ExtraOptions:              extraOpts,
		CaptureService: func(svc *service.FactoryService) {
			server.service = svc
		},
	})
	server.FunctionalAPIServer = base
	return server
}

func (fs *functionalAPIServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()

	req := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeID,
		Payload:      payload,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}

	resp, err := http.Post(fs.URL()+"/work", "application/json", bytes.NewReader(body))
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

func getGeneratedJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", endpoint, resp.StatusCode)
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", endpoint, err)
	}
	return out
}

func waitForGeneratedWorkAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, token := range work.Results {
			if token.TraceId == traceID && token.PlaceId == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
}
