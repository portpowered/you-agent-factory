package definitions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	definitionsForcedUnwindEnv       = "YOU_FUNCTIONAL_FACTORY_DEFINITIONS_FORCED_UNWIND"
	definitionsForcedUnwindReportEnv = "YOU_FUNCTIONAL_FACTORY_DEFINITIONS_FORCED_UNWIND_REPORT"
)

type definitionsForcedUnwindState struct {
	sharedProcess  *sharedDefinitionsProcessFixture
	validationHost *sharedDefinitionsServiceHost
	initHost       *sharedDefinitionsServiceHost
	initClient     support.ApplicationProcess
	sessionID      string
	sessionBaseURL string
	workspaceRoot  string
	sessionClosed  bool
}

type definitionsForcedUnwindReport struct {
	SharedProcessClosed      bool   `json:"shared_process_closed"`
	SharedProcessCloseError  string `json:"shared_process_close_error,omitempty"`
	ServiceHostsCloseError   string `json:"service_hosts_close_error,omitempty"`
	InitClientClosed         bool   `json:"init_client_closed"`
	ValidationListenerURL    string `json:"validation_listener_url,omitempty"`
	ValidationListenerClosed bool   `json:"validation_listener_closed"`
	ValidationFactoryDir     string `json:"validation_factory_dir,omitempty"`
	ValidationFactoryAbsent  bool   `json:"validation_factory_absent"`
	ValidationHomeDir        string `json:"validation_home_dir,omitempty"`
	ValidationHomeAbsent     bool   `json:"validation_home_absent"`
	InitListenerURL          string `json:"init_listener_url,omitempty"`
	InitListenerClosed       bool   `json:"init_listener_closed"`
	InitFactoryDir           string `json:"init_factory_dir,omitempty"`
	InitFactoryAbsent        bool   `json:"init_factory_absent"`
	InitHomeDir              string `json:"init_home_dir,omitempty"`
	InitHomeAbsent           bool   `json:"init_home_absent"`
	SessionID                string `json:"session_id,omitempty"`
	SessionDeleted           bool   `json:"session_deleted"`
	WorkspaceRoot            string `json:"workspace_root,omitempty"`
	WorkspaceRootAbsent      bool   `json:"workspace_root_absent"`
}

var definitionsForcedUnwind *definitionsForcedUnwindState

// failDefinitionsForcedUnwindAfterAssertion acquires every package-scoped
// Definitions owner before deliberately failing. The environment is used only
// by the one-shot child characterization command, so this does not add a new
// top-level test to the package denominator.
func failDefinitionsForcedUnwindAfterAssertion(t *testing.T) {
	t.Helper()
	if os.Getenv(definitionsForcedUnwindEnv) != "1" {
		return
	}
	definitionsForcedUnwind = &definitionsForcedUnwindState{
		sharedProcess: sharedDefinitionsFixture,
	}
	if definitionsForcedUnwind.sharedProcess == nil {
		t.Fatal("shared Factory Definitions process is unavailable for forced unwind")
	}
	definitionsForcedUnwind.validationHost = sharedDefinitionsValidationServer(t)
	definitionsForcedUnwind.initHost = sharedDefinitionsInitServer(t)
	definitionsForcedUnwind.initClient = sharedDefinitionsInitProcess(t)

	workspaceParent := t.TempDir()
	definitionsForcedUnwind.workspaceRoot = filepath.Join(workspaceParent, "forced-unwind-workspace")
	if err := os.MkdirAll(definitionsForcedUnwind.workspaceRoot, 0o755); err != nil {
		t.Fatalf("create forced-unwind workspace: %v", err)
	}
	definitionsForcedUnwind.sessionBaseURL = definitionsForcedUnwind.initHost.baseURL
	definitionsForcedUnwind.sessionID = initFactoryViaSessionCreateWithProcess(
		t,
		definitionsForcedUnwind.initClient,
		definitionsForcedUnwind.sessionBaseURL,
		definitionsForcedUnwind.workspaceRoot,
	)
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, definitionsForcedUnwind.sessionBaseURL, definitionsForcedUnwind.sessionID)
		definitionsForcedUnwind.sessionClosed = definitionsFactorySessionDeleted(
			definitionsForcedUnwind.sessionBaseURL,
			definitionsForcedUnwind.sessionID,
		)
	})

	invalidFactory, err := factoryDefinitionFromConfig(multipleActionableDefectsFactoryConfig())
	if err != nil {
		t.Fatalf("marshal forced-unwind invalid Factory: %v", err)
	}
	result, status := postValidateFactory(t, definitionsForcedUnwind.validationHost.baseURL, invalidFactory)
	if status != http.StatusOK || len(result.Targets) == 0 {
		t.Fatalf("forced-unwind validation result = %#v status=%d, want public validation failure targets", result, status)
	}

	t.Fatal("intentional Factory Definitions forced-unwind characterization failure")
}

func writeDefinitionsForcedUnwindReport(
	serviceHostsCloseErr error,
	processCloseErr error,
) error {
	path := strings.TrimSpace(os.Getenv(definitionsForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := definitionsForcedUnwindReport{}
	if serviceHostsCloseErr != nil {
		report.ServiceHostsCloseError = serviceHostsCloseErr.Error()
	}
	if processCloseErr != nil {
		report.SharedProcessCloseError = processCloseErr.Error()
	}
	if state := definitionsForcedUnwind; state != nil {
		report.SharedProcessClosed = state.sharedProcess != nil && processCloseErr == nil
		report.InitClientClosed = state.initClient != nil && sharedDefinitionsInitClientCloseErr == nil
		report.SessionID = state.sessionID
		report.SessionDeleted = state.sessionClosed
		report.WorkspaceRoot = state.workspaceRoot
		report.WorkspaceRootAbsent = definitionsPathAbsent(state.workspaceRoot)
		if state.validationHost != nil {
			report.ValidationListenerURL = state.validationHost.baseURL
			report.ValidationListenerClosed = definitionsListenerClosed(state.validationHost.baseURL)
			report.ValidationFactoryDir = state.validationHost.factoryDir
			report.ValidationFactoryAbsent = definitionsPathAbsent(state.validationHost.factoryDir)
			report.ValidationHomeDir = state.validationHost.homeDir
			report.ValidationHomeAbsent = definitionsPathAbsent(state.validationHost.homeDir)
		}
		if state.initHost != nil {
			report.InitListenerURL = state.initHost.baseURL
			report.InitListenerClosed = definitionsListenerClosed(state.initHost.baseURL)
			report.InitFactoryDir = state.initHost.factoryDir
			report.InitFactoryAbsent = definitionsPathAbsent(state.initHost.factoryDir)
			report.InitHomeDir = state.initHost.homeDir
			report.InitHomeAbsent = definitionsPathAbsent(state.initHost.homeDir)
		}
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func definitionsFactorySessionDeleted(baseURL, sessionID string) bool {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(sessionID) == "" {
		return false
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusNotFound
}

func definitionsListenerClosed(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return false
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return false
	}
	return true
}

func definitionsPathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}
