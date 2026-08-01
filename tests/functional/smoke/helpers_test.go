package smoke

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// waitForSmokeServerReady polls the public Factory Session work endpoint until
// the server started by a smoke CLI harness is accepting requests.
func waitForSmokeServerReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, support.DefaultSessionWorkURL(baseURL, "/work"), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for GET /work on %s", baseURL)
}

// namedFactorySmokeEnvironment gives spawned CLIs one unambiguous home across
// operating systems. Windows resolves USERPROFILE instead of HOME.
func namedFactorySmokeEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func runFactoryThroughCustomerProcess(
	t *testing.T,
	dir string,
	provider providercontract.Provider,
) factoryapi.StatusResponse {
	t.Helper()
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: server.Start,
		ProviderOverride: provider,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	status := support.WaitForTerminalStatus(t, server.WaitForURL(t), 15*time.Second)
	daemon.Stop(t)
	return status
}
