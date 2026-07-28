package unary_contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
			"",
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
			"",
			strings.NewReader("# Review\n\nFrom stdin."),
		)
		submitted := assertUnarySubmitAcknowledgment(t, output, unaryContractStdinWorkName)
		assertUnaryWorkListedAfterSubmit(t, baseURL, unaryContractStdinWorkName, *submitted.WorkID)
	})
}

// TestCLISubmitUnaryDefaultAndExplicitSessionTargeting proves unary you submit
// targets the ~default Factory Session when --session is omitted and scopes to
// the named session when --session is provided.
func TestCLISubmitUnaryDefaultAndExplicitSessionTargeting(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, unaryContractFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	process := buildUnaryContractProcess(t, serviceedges.Edges{})
	baseURL := server.URL()

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("# Session target\n\nScoped submit."), 0o600); err != nil {
		t.Fatalf("write unary payload file: %v", err)
	}

	t.Run("default session when --session omitted", func(t *testing.T) {
		output := executeUnarySubmitCLI(
			t,
			process,
			baseURL,
			unaryContractDefaultSessionWorkName,
			payloadPath,
			"",
			nil,
		)
		submitted := assertUnarySubmitAcknowledgmentForSession(
			t,
			output,
			unaryContractDefaultSessionWorkName,
			factorysessions.DefaultSessionID,
		)
		assertUnaryWorkListedInSession(
			t,
			baseURL,
			factorysessions.DefaultSessionID,
			unaryContractDefaultSessionWorkName,
			*submitted.WorkID,
		)
	})

	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	explicitSessionID := opened.Session.Id

	t.Run("explicit session when --session provided", func(t *testing.T) {
		output := executeUnarySubmitCLI(
			t,
			process,
			baseURL,
			unaryContractExplicitSessionWorkName,
			payloadPath,
			explicitSessionID,
			nil,
		)
		submitted := assertUnarySubmitAcknowledgmentForSession(
			t,
			output,
			unaryContractExplicitSessionWorkName,
			explicitSessionID,
		)
		assertUnaryWorkListedInSession(
			t,
			baseURL,
			explicitSessionID,
			unaryContractExplicitSessionWorkName,
			*submitted.WorkID,
		)
	})
}

// TestCLISubmitUnaryStructuredFailurePreservesPublicMessage proves unary you submit
// exits non-success against a structured backend rejection and preserves only the
// public typed failure fields exposed by the CLI transport contract.
func TestCLISubmitUnaryStructuredFailurePreservesPublicMessage(t *testing.T) {
	server, assertRequest := newUnaryStructuredFailureServer(t)
	process := buildUnaryContractProcess(t, serviceedges.Edges{})
	payloadPath := writeUnaryContractPayloadFile(t, "# Reject\n\nStructured failure.")

	inputs := executeUnarySubmitExpectingFailure(
		t,
		process,
		server.URL,
		unaryContractStructuredFailureWorkName,
		payloadPath,
	)
	assertRequest(t)
	assertUnarySubmitStructuredFailurePreservesPublicMessage(t, inputs)
}

// TestCLISubmitUnaryContractHarnessExecutesThroughRootBuildProcess proves the
// Work-owned unary_contract cell constructs a customer process through
// root.BuildProcess, invokes public you submit through Process.Execute, and
// replaces external effects only through edges.Edges.
func TestCLISubmitUnaryContractHarnessExecutesThroughRootBuildProcess(t *testing.T) {
	server, assertRequest := newUnaryStructuredFailureServer(t)
	process := buildUnaryContractProcess(t, serviceedges.Edges{})
	payloadPath := writeUnaryContractPayloadFile(t, "# Harness\n\nStructured failure.")

	inputs := executeUnarySubmitExpectingFailure(
		t,
		process,
		server.URL,
		unaryContractStructuredFailureWorkName,
		payloadPath,
	)
	assertRequest(t)
	assertUnarySubmitStructuredFailurePreservesPublicMessage(t, inputs)
}
