package root_composition_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestLocalAICLIConformanceMatrixRunsThroughRootBuildProcess proves every
// catalog-declared pair reaches the customer-facing models invoke command,
// the managed Models host lifecycle, the pinned LocalAI gRPC fixture, and the
// decoded generic response. Each invocation uses the reusable process through
// Process.Execute rather than calling a service or building the CLI binary.
func TestLocalAICLIConformanceMatrixRunsThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	home := functionalTempDir(t)
	writeGenericConformanceCaches(t, home)
	dir := functionalScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))
	server, _ := startLocalAIConformanceServer(t, dir, home, fixture)
	edges, rejectingNetwork, compatibility, hostLauncher := localAIConformanceEdges(home, fixture)
	process := functionalBuildProcess(t, edges)
	support.CleanupProcess(t, process)

	matrix := conformance.Build(models.GenericOperationCatalog{})
	environment := functionalHomeEnvironment(home)
	executor := func(row conformance.Row) (models.GenericInvocationResult, error) {
		httpResult, err := executeLocalAIHTTPConformanceRow(t, server.URL()+"/models/invocations", row)
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s HTTP: %w", row.Label, err)
		}
		cliResult, err := executeLocalAICLIConformanceRow(t, process, dir, environment, row)
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s CLI: %w", row.Label, err)
		}
		if httpResult.Status != models.ModelInvocationStatusCompleted || cliResult.Status != models.ModelInvocationStatusCompleted {
			return models.GenericInvocationResult{}, fmt.Errorf("%s surface result was not completed", row.Label)
		}
		return cliResult, nil
	}
	report, err := matrix.Run(executor, conformance.ModeStrict)
	t.Log(conformanceReportText(report))
	for _, result := range report.Results {
		if result.Err != nil {
			t.Logf("%s error=%v", result.Row.Label, result.Err)
		}
	}
	if err != nil {
		t.Fatalf("LocalAI CLI conformance: %v", err)
	}
	if got, want := report.ImplementedCount(), len(matrix.Rows); got != want {
		t.Fatalf("implemented rows = %d, want %d", got, want)
	}
	if report.ExpectedUnimplementedCount() != 0 || report.UnexpectedFailureCount() != 0 {
		t.Fatalf("conformance report = %#v, want all implemented", report.Results)
	}

	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("model asset network calls = %d, want 0", rejectingNetwork.Calls())
	}
	if compatibility.Calls() == 0 {
		t.Fatal("managed host compatibility edge calls = 0, want at least one")
	}
	if hostLauncher.Calls() == 0 {
		t.Fatal("managed host launcher calls = 0, want at least one")
	}
	assertFixtureConformanceCallCounts(t, fixture, 2)

	t.Logf("HTTP+CLI %s", conformanceReportText(report))
}

func executeLocalAIHTTPConformanceRow(
	t *testing.T,
	endpoint string,
	row conformance.Row,
) (models.GenericInvocationResult, error) {
	t.Helper()
	response, failure, statusCode, err := postConformanceInvocation(t.Context(), endpoint, row)
	if err != nil {
		return models.GenericInvocationResult{}, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"public generic invocation returned HTTP %d: %s (%s)",
			statusCode, failure.Message, failure.Code,
		)
	}
	if response.Failure != nil {
		return models.GenericInvocationResult{}, fmt.Errorf("public generic invocation failed: %s", response.Failure.Message)
	}
	if err := assertConformanceResponse(row, response); err != nil {
		return models.GenericInvocationResult{}, err
	}
	return models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted}, nil
}

func executeLocalAICLIConformanceRow(
	t *testing.T,
	process support.Process,
	dir string,
	environment []string,
	row conformance.Row,
) (models.GenericInvocationResult, error) {
	t.Helper()
	text := row.Inputs[0].Content
	for _, input := range row.Inputs {
		if input.Modality == models.ModalityText {
			text = input.Content
			break
		}
	}
	args := []string{
		"you", "--json", "models", "invoke", conformanceModelName(row.Operation.Name),
		"--operation", row.Operation.Name,
	}
	for _, input := range row.Inputs {
		value, err := json.Marshal(struct {
			Name        string `json:"name"`
			Modality    string `json:"modality"`
			ContentType string `json:"contentType"`
			MediaType   string `json:"mediaType"`
			Content     string `json:"content"`
		}{
			Name: input.Name, Modality: string(input.Modality),
			ContentType: input.ContentType, MediaType: input.MediaType,
			Content: input.Content,
		})
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s encode CLI input: %w", row.Label, err)
		}
		args = append(args, "--input", string(value))
	}
	if len(row.Inputs) == 0 {
		args = append(args, "--text", text)
	}
	for _, parameter := range conformanceParameters() {
		value, err := json.Marshal(struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		}{Name: parameter.Name, Value: parameter.Value})
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s encode CLI parameter: %w", row.Label, err)
		}
		args = append(args, "--parameter", string(value))
	}

	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s CLI invocation: %w (stdout=%q stderr=%q)",
			row.Label, err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s decode CLI generic response: %w (stdout=%q stderr=%q)",
			row.Label, err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	if response.Failure != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s CLI generic invocation failed: %s", row.Label, response.Failure.Message,
		)
	}
	if err := assertConformanceResponse(row, response); err != nil {
		return models.GenericInvocationResult{}, err
	}
	return models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted}, nil
}

func conformanceReportText(report conformance.Report) string {
	var output strings.Builder
	if _, err := report.WriteTo(&output); err != nil {
		return fmt.Sprintf("unable to write report: %v", err)
	}
	return output.String()
}
