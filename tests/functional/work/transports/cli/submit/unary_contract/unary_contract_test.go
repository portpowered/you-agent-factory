package unary_contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLISubmitUnaryFileAndStdinReachWork proves unary you submit accepts
// payload content from a filesystem path and from stdin and enqueues Work on a
// running Factory Session with observable public CLI acknowledgment.
func TestCLISubmitUnaryFileAndStdinReachWork(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, unaryContractFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	process := buildUnaryContractProcess(t, serviceedges.Edges{})
	baseURL := server.URL()

	t.Run("file", func(t *testing.T) {
		payloadPath := filepath.Join(t.TempDir(), "request.md")
		if err := os.WriteFile(payloadPath, []byte("# Review\n\nFrom file."), 0o600); err != nil {
			t.Fatalf("write unary payload file: %v", err)
		}

		output := executeUnarySubmitCLI(
			t,
			process,
			baseURL,
			unaryContractFileWorkName,
			payloadPath,
			nil,
		)
		submitted := assertUnarySubmitAcknowledgment(t, output, unaryContractFileWorkName)
		assertUnaryWorkListedAfterSubmit(t, baseURL, unaryContractFileWorkName, *submitted.WorkID)
	})

	t.Run("stdin", func(t *testing.T) {
		output := executeUnarySubmitCLI(
			t,
			process,
			baseURL,
			unaryContractStdinWorkName,
			"-",
			strings.NewReader("# Review\n\nFrom stdin."),
		)
		submitted := assertUnarySubmitAcknowledgment(t, output, unaryContractStdinWorkName)
		assertUnaryWorkListedAfterSubmit(t, baseURL, unaryContractStdinWorkName, *submitted.WorkID)
	})
}
