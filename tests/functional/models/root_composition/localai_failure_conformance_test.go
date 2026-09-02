package root_composition_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestLocalAIFailureDiagnosticsReachHTTPAndCLI proves the shared fixture's
// dependency and protocol failures travel through both public surfaces as
// typed, actionable outcomes. Health remains available in each mode so the
// failure is observed at the operation boundary rather than being collapsed
// into an unrelated host-startup timeout.
func TestLocalAIFailureDiagnosticsReachHTTPAndCLI(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		mode        localai.Mode
		class       models.InvocationFailureClass
		httpStatus  int
		httpCode    string
		messagePart string
	}{
		{
			name:        "backend unavailable",
			mode:        localai.ModeUnavailable,
			class:       models.InvocationFailureClassBackendReadiness,
			httpStatus:  http.StatusServiceUnavailable,
			httpCode:    "MODEL_BACKEND_NOT_READY",
			messagePart: "LocalAI backend is unavailable",
		},
		{
			name:        "protocol mismatch",
			mode:        localai.ModeProtocolMismatch,
			class:       models.InvocationFailureClassBackendProtocol,
			httpStatus:  http.StatusBadGateway,
			httpCode:    "MODEL_BACKEND_FAILURE",
			messagePart: "LocalAI backend protocol is incompatible",
		},
		{
			name:        "malformed response",
			mode:        localai.ModeMalformed,
			class:       models.InvocationFailureClassMalformedResponse,
			httpStatus:  http.StatusBadGateway,
			httpCode:    "MODEL_BACKEND_FAILURE",
			messagePart: "LocalAI backend returned malformed response",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := functionalStartLocalAI(t, localai.Options{Mode: test.mode, EmbeddingDimensions: 5})
			home := functionalTempDir(t)
			writeGenericConformanceCaches(t, home)
			dir := functionalScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))
			server, _ := startLocalAIConformanceServer(t, dir, home, fixture)

			row := embeddingConformanceRow(t)
			_, failure, statusCode, err := postConformanceInvocation(
				t.Context(), server.URL()+"/models/invocations", row,
			)
			if err != nil {
				t.Fatalf("HTTP %s request: %v", test.name, err)
			}
			if statusCode != test.httpStatus || string(failure.Code) != test.httpCode {
				t.Fatalf(
					"HTTP %s response = status %d code %q message %q, want status %d code %q",
					test.name, statusCode, failure.Code, failure.Message,
					test.httpStatus, test.httpCode,
				)
			}
			if !strings.Contains(failure.Message, test.messagePart) ||
				!strings.Contains(failure.Message, "embed") ||
				!strings.Contains(failure.Message, "EMBED") {
				t.Fatalf("HTTP %s message = %q, want model, operation, and actionable class", test.name, failure.Message)
			}

			edges, _, _, _ := localAIConformanceEdges(home, fixture)
			process := functionalBuildProcess(t, edges)
			support.CleanupProcess(t, process)
			_, cliErr := executeLocalAICLIConformanceRow(
				t, process, dir, functionalHomeEnvironment(home), row,
			)
			if cliErr == nil {
				t.Fatalf("CLI %s invocation succeeded, want typed failure", test.name)
			}
			var typed *models.InvocationFailure
			if !errors.As(cliErr, &typed) {
				t.Fatalf("CLI %s error = %v (%T), want *models.InvocationFailure", test.name, cliErr, cliErr)
			}
			if typed.Class != test.class || typed.Model.NameOrURI != "embed" || typed.Operation != models.OperationEMBED {
				t.Fatalf("CLI %s typed failure = %#v, want class/model/operation %s/embed/EMBED", test.name, typed, test.class)
			}
			if !strings.Contains(typed.Error(), test.messagePart) {
				t.Fatalf("CLI %s message = %q, want %q", test.name, typed.Error(), test.messagePart)
			}

			if calls := fixture.Calls(); len(calls) < 3 {
				t.Fatalf("fixture calls = %d, want readiness plus HTTP and CLI operation calls", len(calls))
			}
		})
	}
}

func embeddingConformanceRow(t *testing.T) conformance.Row {
	t.Helper()
	for _, row := range conformance.Build(models.GenericOperationCatalog{}).Rows {
		if row.Operation.Name == models.OperationEMBED && row.Variant == conformance.VariantText {
			return row
		}
	}
	t.Fatal("conformance matrix has no EMBED/text row")
	return conformance.Row{}
}
