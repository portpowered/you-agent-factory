package functional_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

// exportImportSmokeHarness drives the canonical current-factory export surface
// (`GET /factory/~current`) and named-factory import surface (`POST /factory`)
// so future export/import and factory-management smokes can reuse one
// orchestration path and layer scenario-specific assertions on top.
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
	Server           *FunctionalServer
	ExportedFactory  factoryapi.NamedFactory
	ImportRequest    factoryapi.NamedFactory
	ImportedFactory  factoryapi.NamedFactory
	CurrentFactory   factoryapi.NamedFactory
	Dashboard        DashboardResponse
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

	server := StartFunctionalServerWithConfig(t, rootDir, true, func(cfg *service.FactoryServiceConfig) {
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
	dashboard := server.GetDashboard(t)

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
		Dashboard:        dashboard,
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
	if r.Dashboard.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("dashboard activation drift: dashboard runtime_status = %q, want %q", r.Dashboard.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if r.Dashboard.Topology.WorkstationNodesById == nil {
		t.Fatal("dashboard activation drift: dashboard topology is missing workstation nodes")
	}

	gotWorkstationNames := make([]string, 0, len(*r.Dashboard.Topology.WorkstationNodesById))
	sawTerminalPlace := false
	for _, node := range *r.Dashboard.Topology.WorkstationNodesById {
		if node.WorkstationName != nil && *node.WorkstationName != "" {
			gotWorkstationNames = append(gotWorkstationNames, *node.WorkstationName)
		}
		if node.OutputPlaceIds != nil && slices.Contains(*node.OutputPlaceIds, fixture.Expected.TerminalPlaceID) {
			sawTerminalPlace = true
		}
	}
	slices.Sort(gotWorkstationNames)

	wantWorkstationNames := append([]string(nil), fixture.Expected.WorkstationNames...)
	slices.Sort(wantWorkstationNames)

	if !reflect.DeepEqual(gotWorkstationNames, wantWorkstationNames) {
		t.Fatalf(
			"dashboard activation drift: dashboard workstation names = %#v, want %#v",
			gotWorkstationNames,
			wantWorkstationNames,
		)
	}
	if !sawTerminalPlace {
		t.Fatalf(
			"dashboard activation drift: dashboard topology is missing terminal place %q after import",
			fixture.Expected.TerminalPlaceID,
		)
	}
	if r.ImportedDir != filepath.Join(r.RootDir, string(r.ImportRequest.Name)) {
		t.Fatalf(
			"dashboard activation drift: resolved current factory dir = %q, want %q",
			r.ImportedDir,
			filepath.Join(r.RootDir, string(r.ImportRequest.Name)),
		)
	}
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
