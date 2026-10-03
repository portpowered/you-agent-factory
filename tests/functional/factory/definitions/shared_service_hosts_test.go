package definitions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedDefinitionsServiceHostShutdownTimeout = 60 * time.Second

// sharedDefinitionsServiceHost owns one continuous service-mode invocation.
// The host Factory and process wiring are shared only by scenarios that use
// the same public service boundary; each scenario still owns its project,
// HOME, session, request payload, and captured streams.
type sharedDefinitionsServiceHost struct {
	process      support.ApplicationProcess
	baseURL      string
	handler      http.Handler
	factoryDir   string
	homeDir      string
	env          []string
	provider     *support.RecordingCommandRunner
	listenerDone <-chan struct{}
	cancel       context.CancelFunc
	done         <-chan error
}

var (
	sharedDefinitionsValidationHostOnce sync.Once
	sharedDefinitionsValidationHost     *sharedDefinitionsServiceHost
	sharedDefinitionsValidationHostErr  error
	sharedDefinitionsValidationReady    sync.Once

	sharedDefinitionsInitHostOnce sync.Once
	sharedDefinitionsInitHost     *sharedDefinitionsServiceHost
	sharedDefinitionsInitHostErr  error
	sharedDefinitionsInitReady    sync.Once

	sharedDefinitionsInitClientOnce     sync.Once
	sharedDefinitionsInitClient         support.ApplicationProcess
	sharedDefinitionsInitClientErr      error
	sharedDefinitionsInitClientCloseErr error
)

func sharedDefinitionsValidationServer(t testing.TB) *sharedDefinitionsServiceHost {
	t.Helper()
	sharedDefinitionsValidationHostOnce.Do(func() {
		sharedDefinitionsValidationHost, sharedDefinitionsValidationHostErr = startSharedDefinitionsServiceHost(
			validAPIValidationFactoryConfig(),
			"unexpected provider invocation in shared Factory Definitions validation host",
		)
	})
	if sharedDefinitionsValidationHostErr != nil {
		t.Fatalf("start shared Factory Definitions validation host: %v", sharedDefinitionsValidationHostErr)
	}
	if sharedDefinitionsValidationHost == nil {
		t.Fatal("shared Factory Definitions validation host is unavailable")
	}
	sharedDefinitionsValidationReady.Do(func() {
		support.WaitForStatus(t, sharedDefinitionsValidationHost.baseURL, sharedDefinitionsServiceHostShutdownTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
	})
	return sharedDefinitionsValidationHost
}

func sharedDefinitionsInitServer(t testing.TB) *sharedDefinitionsServiceHost {
	t.Helper()
	sharedDefinitionsInitHostOnce.Do(func() {
		sharedDefinitionsInitHost, sharedDefinitionsInitHostErr = startSharedDefinitionsServiceHost(
			initHostFactoryConfig(),
			"unexpected provider invocation in shared Factory Definitions init host",
		)
	})
	if sharedDefinitionsInitHostErr != nil {
		t.Fatalf("start shared Factory Definitions init host: %v", sharedDefinitionsInitHostErr)
	}
	if sharedDefinitionsInitHost == nil {
		t.Fatal("shared Factory Definitions init host is unavailable")
	}
	sharedDefinitionsInitReady.Do(func() {
		support.WaitForStatus(t, sharedDefinitionsInitHost.baseURL, sharedDefinitionsServiceHostShutdownTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
	})
	return sharedDefinitionsInitHost
}

func sharedDefinitionsInitProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	sharedDefinitionsInitClientOnce.Do(func() {
		sharedDefinitionsInitClient, sharedDefinitionsInitClientErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
	})
	if sharedDefinitionsInitClientErr != nil {
		t.Fatalf("build shared Factory Definitions init client: %v", sharedDefinitionsInitClientErr)
	}
	if sharedDefinitionsInitClient == nil {
		t.Fatal("shared Factory Definitions init client is unavailable")
	}
	return sharedDefinitionsInitClient
}

func (host *sharedDefinitionsServiceHost) URL() string {
	if host == nil {
		return ""
	}
	return host.baseURL
}

func startSharedDefinitionsServiceHost(
	cfg map[string]any,
	providerError string,
) (*sharedDefinitionsServiceHost, error) {
	factoryDir, err := writeSharedDefinitionsFactory(cfg)
	if err != nil {
		return nil, err
	}
	homeDir, err := os.MkdirTemp("", "c05-factory-definitions-home-")
	if err != nil {
		_ = os.RemoveAll(factoryDir)
		return nil, fmt.Errorf("create shared service home: %w", err)
	}

	provider := support.NewRecordingCommandRunner(providerError)
	api := support.NewProcessAPIServer()
	listenerDone := make(chan struct{})
	var handler http.Handler
	apiStarter := func(ctx context.Context, request platformhttpserver.StartRequest) error {
		handler = request.Handler
		// ProcessAPIServer.Start returns only after its httptest listener has
		// been closed, so the completed starter is a deterministic observation.
		defer close(listenerDone)
		return api.Start(ctx, request)
	}
	processContext, cancel := context.WithCancel(context.Background())
	process, err := support.BuildProcessWithContext(processContext, serviceedges.Edges{
		APIServerStarter:      apiStarter,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		cancel()
		_ = os.RemoveAll(factoryDir)
		_ = os.RemoveAll(homeDir)
		return nil, fmt.Errorf("build shared service root process: %w", err)
	}

	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--continuously", "--with-server", "--quiet", "--dir", factoryDir, "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(inputs.Input)
	}()

	baseURL, err := api.WaitForBaseURL(sharedDefinitionsServiceHostShutdownTimeout)
	if err != nil {
		_ = stopSharedDefinitionsServiceHost(&sharedDefinitionsServiceHost{
			process: process, factoryDir: factoryDir, homeDir: homeDir,
			cancel: cancel, done: done,
		})
		return nil, fmt.Errorf("wait for shared service API server: %w", err)
	}

	return &sharedDefinitionsServiceHost{
		process:      process,
		baseURL:      baseURL,
		handler:      handler,
		factoryDir:   factoryDir,
		homeDir:      homeDir,
		env:          append([]string(nil), inputs.Input.Env...),
		provider:     provider,
		listenerDone: listenerDone,
		cancel:       cancel,
		done:         done,
	}, nil
}

func writeSharedDefinitionsFactory(cfg map[string]any) (string, error) {
	factoryDir, err := os.MkdirTemp("", "c05-factory-definitions-factory-")
	if err != nil {
		return "", fmt.Errorf("create shared service Factory directory: %w", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		_ = os.RemoveAll(factoryDir)
		return "", fmt.Errorf("marshal shared service Factory config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), raw, 0o644); err != nil {
		_ = os.RemoveAll(factoryDir)
		return "", fmt.Errorf("write shared service Factory config: %w", err)
	}

	workstations, ok := cfg["workstations"].([]map[string]any)
	if !ok {
		return factoryDir, nil
	}
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		workstationDir := filepath.Join(factoryDir, "workstations", name)
		if err := os.MkdirAll(workstationDir, 0o755); err != nil {
			_ = os.RemoveAll(factoryDir)
			return "", fmt.Errorf("create shared workstation %q: %w", name, err)
		}
		if err := os.WriteFile(
			filepath.Join(workstationDir, "AGENTS.md"),
			[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
			0o644,
		); err != nil {
			_ = os.RemoveAll(factoryDir)
			return "", fmt.Errorf("write shared workstation %q: %w", name, err)
		}
	}
	return factoryDir, nil
}

func closeSharedDefinitionsServiceHosts() error {
	sharedDefinitionsInitClientCloseErr = nil
	type closeResult struct {
		name string
		err  error
	}
	results := make(chan closeResult, 3)
	pending := 0
	closeHost := func(name string, host *sharedDefinitionsServiceHost) {
		if host == nil {
			return
		}
		pending++
		go func() { results <- closeResult{name: name + " host", err: stopSharedDefinitionsServiceHost(host)} }()
	}
	closeHost("validation", sharedDefinitionsValidationHost)
	closeHost("init", sharedDefinitionsInitHost)
	if sharedDefinitionsInitClient != nil {
		pending++
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), sharedDefinitionsServiceHostShutdownTimeout)
			defer cancel()
			sharedDefinitionsInitClientCloseErr = sharedDefinitionsInitClient.Close(ctx)
			results <- closeResult{name: "init client", err: sharedDefinitionsInitClientCloseErr}
		}()
	}
	var failures []string
	for range pending {
		result := <-results
		if result.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", result.name, result.err))
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func stopSharedDefinitionsServiceHost(host *sharedDefinitionsServiceHost) error {
	if host == nil {
		return nil
	}
	if host.cancel != nil {
		host.cancel()
	}
	var failures []string
	if host.done != nil {
		select {
		case err := <-host.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				failures = append(failures, fmt.Sprintf("Execute: %v", err))
			}
		case <-time.After(sharedDefinitionsServiceHostShutdownTimeout):
			failures = append(failures, "timed out waiting for Execute shutdown")
		}
	}
	if host.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedDefinitionsServiceHostShutdownTimeout)
		if err := host.process.Close(ctx); err != nil {
			failures = append(failures, fmt.Sprintf("close process: %v", err))
		}
		cancel()
	}
	if host.listenerDone != nil && !definitionsListenerClosed(host.listenerDone) {
		failures = append(failures, "API listener did not report shutdown")
	}
	if err := os.RemoveAll(host.factoryDir); err != nil {
		failures = append(failures, fmt.Sprintf("remove Factory directory: %v", err))
	}
	if err := os.RemoveAll(host.homeDir); err != nil {
		failures = append(failures, fmt.Sprintf("remove home directory: %v", err))
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func openDefinitionsNamedSession(t *testing.T, baseURL, folder, name string) string {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folder,
		Target:     &factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindNamed, Name: &name},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", payload)
	if status != http.StatusOK {
		t.Fatalf("open named session = %d %s", status, body)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(body, &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("missing opened session: %#v", opened)
	}
	return opened.Session.Id
}

func assertDefinitionsHTTPCancellation(t *testing.T, host *sharedDefinitionsServiceHost, valid factoryapi.Factory, payload []byte) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	probe := httptest.NewRecorder()
	host.handler.ServeHTTP(probe, httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewReader(payload)))
	if probe.Code != http.StatusOK {
		t.Fatalf("uncanceled captured route = %d %s", probe.Code, probe.Body.String())
	}
	cancel()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() {
		host.handler.ServeHTTP(recorder, request)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("composed validation did not complete after cancellation")
	}
	// The composed route currently promotes Sessions.ValidateFactory, which
	// reports a canceled validation owner as BAD_REQUEST. Definitions' own
	// adapter writes no body; route promotion needs a separate scope decision.
	assertDefinitionsHTTPError(t, recorder.Body.Bytes(), recorder.Code, http.StatusBadRequest, "BAD_REQUEST")
	peer, status := postValidateFactory(t, host.URL(), valid)
	if status != http.StatusOK || len(peer.Targets) != 0 {
		t.Fatalf("peer validation after cancellation = %#v status=%d", peer, status)
	}
}

func definitionsHTTPRequest(t *testing.T, method, endpoint string, payload []byte) ([]byte, http.Header, int) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body, response.Header, response.StatusCode
}

func assertDefinitionsHTTPError(t *testing.T, body []byte, status, wantStatus int, code string) {
	t.Helper()
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("decode failure %s: %v", body, err)
	}
	if status != wantStatus || string(failure.Code) != code {
		t.Fatalf("status=%d failure=%#v, want %d %s", status, failure, wantStatus, code)
	}
}

func assertDefinitionsHTTPErrorTarget(t *testing.T, body []byte, code string) {
	t.Helper()
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil || failure.Targets == nil || !hasValidationTargetCode(*failure.Targets, code) {
		t.Fatalf("failure lost %s target: %s (%v)", code, body, err)
	}
}

func assertDefinitionsHTTPNamedActivation(t *testing.T, host *sharedDefinitionsServiceHost, id string) {
	t.Helper()
	endpoint := host.URL() + "/factory-sessions/" + id + "/factory"
	before := support.GetJSON[factoryapi.Factory](t, endpoint)
	selected := before
	selected.Name += "-activated"
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: selected, Mode: &mode})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	if status != http.StatusOK {
		t.Fatalf("named activation = %d %s", status, body)
	}
	var saved factoryapi.Factory
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if got := support.GetJSON[factoryapi.Factory](t, endpoint); got.Name != selected.Name || !reflect.DeepEqual(got, saved) {
		t.Fatalf("named activation readback = %#v, want %#v", got, saved)
	}
}
