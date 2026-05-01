package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/service"
	"go.uber.org/zap"
)

type exportImportSmokeHarness struct {
	fixture exportImportFixture
	options exportImportSmokeHarnessOptions
}

type exportImportSmokeHarnessOptions struct {
	sourceFactoryName string
	importFactoryName string
	afterImport       func(*testing.T, exportImportSmokeHarnessResult)
}

type exportImportSmokeHarnessOption func(*exportImportSmokeHarnessOptions)

type exportImportSmokeHarnessResult struct {
	RootDir          string
	Server           *functionalAPIServer
	ExportedFactory  factoryapi.NamedFactory
	ImportRequest    factoryapi.NamedFactory
	ImportedFactory  factoryapi.NamedFactory
	CurrentFactory   factoryapi.NamedFactory
	Status           factoryapi.StatusResponse
	SourceFactoryDir string
	ImportedDir      string
}

func newExportImportSmokeHarness(
	fixture exportImportFixture,
	opts ...exportImportSmokeHarnessOption,
) exportImportSmokeHarness {
	options := exportImportSmokeHarnessOptions{
		sourceFactoryName: "exported-service-simple",
		importFactoryName: "reimported-service-simple",
	}
	for _, opt := range opts {
		opt(&options)
	}
	return exportImportSmokeHarness{
		fixture: fixture,
		options: options,
	}
}

func (h exportImportSmokeHarness) Run(t *testing.T) exportImportSmokeHarnessResult {
	t.Helper()

	rootDir := t.TempDir()
	sourceFactoryDir := h.fixture.persistAs(t, rootDir, h.options.sourceFactoryName)
	if err := config.WriteCurrentFactoryPointer(rootDir, h.options.sourceFactoryName); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(%s): %v", h.options.sourceFactoryName, err)
	}

	server := startFunctionalServerWithConfig(t, rootDir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.NewNop()
	})
	waitForCurrentFactoryRuntimeIdle(t, server.service, 5*time.Second)

	exported := getCurrentNamedFactory(t, server.URL())
	importRequest := exported
	importRequest.Name = factoryapi.FactoryName(h.options.importFactoryName)

	imported := createNamedFactory(t, server.URL(), importRequest)
	current := getCurrentNamedFactory(t, server.URL())
	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")

	importedDir, err := config.ResolveCurrentFactoryDir(rootDir)
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryDir(%s): %v", h.options.importFactoryName, err)
	}

	result := exportImportSmokeHarnessResult{
		RootDir:          rootDir,
		Server:           server,
		ExportedFactory:  exported,
		ImportRequest:    importRequest,
		ImportedFactory:  imported,
		CurrentFactory:   current,
		Status:           status,
		SourceFactoryDir: sourceFactoryDir,
		ImportedDir:      importedDir,
	}

	if h.options.afterImport != nil {
		h.options.afterImport(t, result)
	}
	return result
}

func (r exportImportSmokeHarnessResult) AssertAPIContractSuccess(t *testing.T, fixture exportImportFixture) {
	t.Helper()

	if r.ExportedFactory.Name == "" {
		t.Fatal("api contract drift: GET /factory/~current returned an empty current factory name")
	}
	if !reflect.DeepEqual(
		comparableExportImportFactory(r.ExportedFactory.Factory),
		comparableExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"payload drift: exported current factory diverged from canonical generated payload\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(r.ExportedFactory.Factory),
			comparableExportImportFactory(fixture.GeneratedExportFactor),
		)
	}
	if r.ImportRequest.Name != r.ImportedFactory.Name {
		t.Fatalf("api contract drift: POST /factory created name = %q, want %q", r.ImportedFactory.Name, r.ImportRequest.Name)
	}
	if !reflect.DeepEqual(
		comparableExportImportFactory(r.ImportedFactory.Factory),
		comparableExportImportFactory(r.ImportRequest.Factory),
	) {
		t.Fatalf(
			"api contract drift: POST /factory response diverged from submitted payload\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(r.ImportedFactory.Factory),
			comparableExportImportFactory(r.ImportRequest.Factory),
		)
	}
	if r.CurrentFactory.Name != r.ImportRequest.Name {
		t.Fatalf("api contract drift: GET /factory/~current after import = %q, want %q", r.CurrentFactory.Name, r.ImportRequest.Name)
	}
	if !reflect.DeepEqual(
		comparableExportImportFactory(r.CurrentFactory.Factory),
		comparableExportImportFactory(r.ImportRequest.Factory),
	) {
		t.Fatalf(
			"api contract drift: current-factory readback diverged from imported payload\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(r.CurrentFactory.Factory),
			comparableExportImportFactory(r.ImportRequest.Factory),
		)
	}
}

func (r exportImportSmokeHarnessResult) AssertDashboardActivationSuccess(
	t *testing.T,
	fixture exportImportFixture,
) {
	t.Helper()

	fixture.assertCurrentFactorySignals(t, r.RootDir, r.Server.service, string(r.ImportRequest.Name))

	if r.Status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("dashboard activation drift: GET /status runtime_status = %q, want %q", r.Status.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if r.ImportedDir != filepath.Join(r.RootDir, string(r.ImportRequest.Name)) {
		t.Fatalf(
			"dashboard activation drift: resolved current factory dir = %q, want %q",
			r.ImportedDir,
			filepath.Join(r.RootDir, string(r.ImportRequest.Name)),
		)
	}
}

func createNamedFactory(t *testing.T, serverURL string, namedFactory factoryapi.NamedFactory) factoryapi.NamedFactory {
	t.Helper()

	body, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal create factory request: %v", err)
	}

	resp, err := http.Post(serverURL+"/factory", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("POST /factory status = %d, want 201", resp.StatusCode)
	}

	var created factoryapi.NamedFactory
	decodeJSONResponse(t, resp, &created, "decode create factory response")
	return created
}

func getCurrentNamedFactory(t *testing.T, serverURL string) factoryapi.NamedFactory {
	t.Helper()

	resp, err := http.Get(serverURL + "/factory/~current")
	if err != nil {
		t.Fatalf("GET /factory/~current: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory/~current status = %d, want 200", resp.StatusCode)
	}

	var current factoryapi.NamedFactory
	decodeJSONResponse(t, resp, &current, "decode current factory response")
	return current
}

func decodeJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertCurrentFactoryPointer(t *testing.T, rootDir, want string) {
	t.Helper()

	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if got != want {
		t.Fatalf("current factory pointer = %q, want %q", got, want)
	}
}
