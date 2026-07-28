package fusion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFusionRequiredInputCompletes proves that invoking the packaged
// @you/fusion Factory with only the required input completes under mock
// workers, runs the drafter then refiner dispatch sequence, and returns a
// primary result reflecting the refined fusion outcome for the submitted request.
func TestPackagedFusionRequiredInputCompletes(t *testing.T) {
	input := fmt.Sprintf(
		"functional packaged fusion required input %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedFusionFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	response := startPackagedFusionInvocation(
		t,
		server,
		"packaged-fusion-required-input",
		map[string]any{"input": input},
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one refined result part", response.PrimaryResult)
	}
	primaryText := invocationPrimaryResultText(t, response)
	if !strings.Contains(primaryText, "mock worker accepted") {
		t.Fatalf("primary result = %q, want refined mock-worker outcome", primaryText)
	}
	if strings.Contains(primaryText, input) {
		t.Fatalf("primary result = %q, want refined output rather than raw submitted input echo", primaryText)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 2 {
		t.Fatalf(
			"dispatch count = %d, want drafter and refiner dispatches",
			len(dispatches),
		)
	}
	wantWorkstations := []string{"draft-fusion", "refine-fusion"}
	for index, dispatch := range dispatches {
		if dispatch.Response == nil {
			t.Fatalf("dispatch[%d] = %#v, want completed public dispatch response", index, dispatch)
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch[%d] outcome = %q, want ACCEPTED", index, dispatch.Response.Outcome)
		}
		if dispatch.Request.TransitionId != wantWorkstations[index] {
			t.Fatalf(
				"dispatch[%d] transition = %q, want %q in documented order",
				index,
				dispatch.Request.TransitionId,
				wantWorkstations[index],
			)
		}
	}
}

func startPackagedFusionInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	requestID string,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()

	return postJSON[factoryapi.InvocationResponse](
		t,
		server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		factoryapi.InvocationRequest{
			RequestId: &requestID,
			Args:      &args,
		},
		"start packaged fusion invocation",
	)
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, string(payload))
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}
