package classify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedClassifyRoutesSmallMediumAndLarge(t *testing.T) {
	for _, lane := range []string{"small", "medium", "large"} {
		lane := lane
		t.Run(lane, func(t *testing.T) {
			runner := support.NewShapedProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte(lane)},
				platformprocess.CommandResult{Stdout: []byte(lane + " lane completed")},
			)
			server := startClassifyServer(t, runner)
			response := invokeClassify(t, server, map[string]any{"request": "route this " + lane + " request"})

			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED; response = %#v", response.Status, response)
			}
			if got := primaryText(t, response); !strings.Contains(got, lane+" lane completed") {
				t.Fatalf("primary result = %q, want selected %s lane result", got, lane)
			}
			dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
			if len(dispatches) != 2 || dispatches[0].Request.TransitionId != "classify-request" ||
				dispatches[1].Request.TransitionId != "execute-"+lane {
				t.Fatalf("dispatches = %#v, want classifier then %s executor", dispatches, lane)
			}
		})
	}
}

func TestPackagedClassifyInvalidLabelFailsWithoutExecutingLane(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("extra-large")})
	server := startClassifyServer(t, runner)
	response := invokeClassify(t, server, map[string]any{"request": "route an invalid classification"})

	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider calls = %d, want classifier only", runner.CallCount())
	}
}

func TestPackagedClassifyRoleProviderAndModelSelections(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("small")},
		platformprocess.CommandResult{Stdout: []byte("selected lane completed")},
	)
	server := startClassifyServer(t, runner)
	response := invokeClassify(t, server, map[string]any{
		"request":            "route with role overrides",
		"classifierProvider": "CLAUDE", "classifierModel": "classifier-model",
		"smallProvider": "CODEX", "smallModel": "small-model",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %#v, want classifier and selected lane", requests)
	}
	assertProviderSelection(t, requests[0], "claude", "classifier-model")
	assertProviderSelection(t, requests[1], "codex", "small-model")
}

func startClassifyServer(t *testing.T, runner *support.ShapedProviderCommandRunner) *support.FunctionalAPIServer {
	t.Helper()
	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedClassifyFactoryName)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Args:  []string{"--provider", "CODEX", "--model", "operator-model"},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
}

func invokeClassify(t *testing.T, server *support.FunctionalAPIServer, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	requestID := fmt.Sprintf("classify-%s", strings.ReplaceAll(fmt.Sprint(args["request"]), " ", "-"))
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	response, err := http.Post(
		server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("POST invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST invocation status = %d", response.StatusCode)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation: %v", err)
	}
	return decoded
}

func primaryText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary text: %v", err)
	}
	return part.Text
}

func assertProviderSelection(t *testing.T, request platformprocess.CommandRequest, provider, model string) {
	t.Helper()
	if request.Command != provider {
		t.Fatalf("command = %q, want %q", request.Command, provider)
	}
	for index := 0; index+1 < len(request.Args); index++ {
		if request.Args[index] == "--model" && request.Args[index+1] == model {
			return
		}
	}
	t.Fatalf("args = %#v, want --model %q", request.Args, model)
}
