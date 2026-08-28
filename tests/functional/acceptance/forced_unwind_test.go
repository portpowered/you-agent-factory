package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	acceptanceForcedUnwindEnv          = "YOU_FUNCTIONAL_ACCEPTANCE_FORCED_UNWIND"
	acceptanceForcedUnwindReportEnv    = "YOU_FUNCTIONAL_ACCEPTANCE_FORCED_UNWIND_REPORT"
	acceptanceForcedUnwindCloseTimeout = 5 * time.Second
)

type acceptanceForcedUnwindState struct {
	process             support.ApplicationProcess
	rootDir             string
	homeDir             string
	logDir              string
	workDir             string
	configPath          string
	installedFactoryDir string
	reservedServerURL   string
	processClosed       bool
}

type acceptanceForcedUnwindReport struct {
	ProcessClosed          bool   `json:"process_closed"`
	RootDir                string `json:"root_dir,omitempty"`
	RootAbsent             bool   `json:"root_absent"`
	HomeDir                string `json:"home_dir,omitempty"`
	HomeAbsent             bool   `json:"home_absent"`
	LogDir                 string `json:"log_dir,omitempty"`
	LogAbsent              bool   `json:"log_absent"`
	WorkDir                string `json:"work_dir,omitempty"`
	WorkAbsent             bool   `json:"work_absent"`
	ConfigPath             string `json:"config_path,omitempty"`
	ConfigAbsent           bool   `json:"config_absent"`
	InstalledFactoryDir    string `json:"installed_factory_dir,omitempty"`
	InstalledFactoryAbsent bool   `json:"installed_factory_absent"`
	ReservedServerURL      string `json:"reserved_server_url,omitempty"`
	ReservedPortAvailable  bool   `json:"reserved_port_available"`
}

var acceptanceForcedUnwind *acceptanceForcedUnwindState

// TestMain writes the forced-unwind observation after testing has run all
// t.Cleanup callbacks. The report is opt-in so ordinary acceptance runs keep
// their existing output, timing, and top-level test denominator.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := writeAcceptanceForcedUnwindReport(); err != nil {
		fmt.Fprintf(os.Stderr, "write acceptance forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// failAcceptanceForcedUnwindAfterAssertion acquires the package-owned
// acceptance resources and deliberately fails after their public assertions.
// It is enabled only by the one-shot child characterization command, so it
// does not add an executable test to the normal acceptance denominator.
func failAcceptanceForcedUnwindAfterAssertion(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	process support.ApplicationProcess,
) {
	t.Helper()
	if os.Getenv(acceptanceForcedUnwindEnv) != "1" {
		return
	}
	if harness == nil || process == nil {
		t.Fatal("acceptance forced-unwind harness or process is unavailable")
	}

	session := harness.NewSession(t).WithNoExternalServer(t)
	_, initialized := initializeConfigWithProcess(t, process, session, "forced-unwind-config-init")
	installedFactoryDir := materializedNamedFactoryDir(t, initialized, packagedGoalFactoryName)
	state := &acceptanceForcedUnwindState{
		process:             process,
		rootDir:             filepath.Dir(session.HomeDir),
		homeDir:             session.HomeDir,
		logDir:              session.LogDir,
		workDir:             session.WorkDir,
		configPath:          initialized.ConfigPath,
		installedFactoryDir: installedFactoryDir,
		reservedServerURL:   session.ServerURL,
	}
	acceptanceForcedUnwind = state
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), acceptanceForcedUnwindCloseTimeout)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("close acceptance forced-unwind process: %v", err)
			return
		}
		state.processClosed = true
	})

	t.Fatal("intentional acceptance forced-unwind characterization failure")
}

func writeAcceptanceForcedUnwindReport() error {
	path := strings.TrimSpace(os.Getenv(acceptanceForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := acceptanceForcedUnwindReport{}
	if state := acceptanceForcedUnwind; state != nil {
		report.ProcessClosed = state.processClosed
		report.RootDir = state.rootDir
		report.RootAbsent = acceptancePathAbsent(state.rootDir)
		report.HomeDir = state.homeDir
		report.HomeAbsent = acceptancePathAbsent(state.homeDir)
		report.LogDir = state.logDir
		report.LogAbsent = acceptancePathAbsent(state.logDir)
		report.WorkDir = state.workDir
		report.WorkAbsent = acceptancePathAbsent(state.workDir)
		report.ConfigPath = state.configPath
		report.ConfigAbsent = acceptancePathAbsent(state.configPath)
		report.InstalledFactoryDir = state.installedFactoryDir
		report.InstalledFactoryAbsent = acceptancePathAbsent(state.installedFactoryDir)
		report.ReservedServerURL = state.reservedServerURL
		report.ReservedPortAvailable = acceptanceReservedPortAvailable(state.reservedServerURL)
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

func acceptancePathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func acceptanceReservedPortAvailable(serverURL string) bool {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return false
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	address := net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	return listener.Close() == nil
}
