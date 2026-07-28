package deep_research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedDeepResearchRequiredInputCompletes proves that invoking the
// packaged @you/deep-research Factory with only the required research topic
// completes under mock workers, runs the expected specialist-and-lead dispatch
// sequence for a delegating topic shape, and returns a primary synthesis that
// reflects the submitted topic.
func TestPackagedDeepResearchRequiredInputCompletes(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research required topic %d with enough breadth for specialist delegation",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	args := map[string]any{"topic": topic}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-required-input",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED; response = %#v", response.Status, response)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result part", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	primaryText := string(primary)
	for _, want := range []string{
		topic,
		`"researchDepth":2`,
		`"maxSubagents":2`,
		"lead-research-synthesis",
		"research-specialist-technical",
	} {
		if !strings.Contains(primaryText, want) {
			t.Fatalf("primary result = %s, want substring %q", primaryText, want)
		}
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf(
			"dispatch count = %d, want two bounded specialist dispatches and one lead synthesis",
			len(dispatches.Dispatches),
		)
	}
	labels := make(map[string]bool)
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			labels[*dispatch.Label] = true
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
		}
	}
	for _, want := range []string{
		"research-specialist-technical",
		"research-specialist-tradeoffs",
		"lead-research-synthesis",
	} {
		if !labels[want] {
			t.Fatalf("dispatch labels = %#v, want %q", labels, want)
		}
	}
}

// TestPackagedDeepResearchOptionalInputsReachWorkers proves that optional
// deep-research overrides such as research depth, specialist cap, and approved
// model execution selection reach mock workers and are observable on dispatch
// execution selection and the primary synthesis result.
func TestPackagedDeepResearchOptionalInputsReachWorkers(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research optional overrides %d with enough breadth for specialist delegation",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	args := map[string]any{
		"topic":           topic,
		"researchDepth":   3,
		"maxSubagents":    1,
		"modelProvider":   "CODEX",
		"model":           "gpt-5",
		"reasoningEffort": "medium",
	}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-optional-inputs",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED; response = %#v", response.Status, response)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result part", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	primaryText := string(primary)
	for _, want := range []string{
		topic,
		`"researchDepth":3`,
		`"maxSubagents":1`,
		`"modelProvider":"CODEX"`,
		`"model":"gpt-5"`,
		`"reasoningEffort":"medium"`,
		"research-specialist-technical",
	} {
		if !strings.Contains(primaryText, want) {
			t.Fatalf("primary result = %s, want substring %q", primaryText, want)
		}
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf(
			"dispatch count = %d, want one bounded specialist dispatch and one lead synthesis",
			len(dispatches.Dispatches),
		)
	}
	labels := make(map[string]bool)
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			labels[*dispatch.Label] = true
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
		}
		if dispatch.ModelProvider == nil || *dispatch.ModelProvider != "CODEX" ||
			dispatch.Model == nil || *dispatch.Model != "gpt-5" ||
			dispatch.ReasoningEffort == nil || *dispatch.ReasoningEffort != "medium" {
			t.Fatalf(
				"dispatch execution selection = provider=%#v model=%#v reasoning=%#v, want approved overrides",
				dispatch.ModelProvider,
				dispatch.Model,
				dispatch.ReasoningEffort,
			)
		}
	}
	if !labels["research-specialist-technical"] || !labels["lead-research-synthesis"] {
		t.Fatalf(
			"dispatch labels = %#v, want technical specialist and lead synthesis",
			labels,
		)
	}
	if labels["research-specialist-tradeoffs"] {
		t.Fatalf("dispatch labels = %#v, want tradeoffs specialist omitted when maxSubagents is 1", labels)
	}
}

// TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome proves that a
// configured mock-worker rejection during packaged @you/deep-research invocation
// returns a failed public terminal outcome without a completed success primary
// result attributable to the failing run.
func TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research worker failure %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		MockWorkersConfig:         packagedDeepResearchRejectingMockWorkersConfig(),
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: packagedDeepResearchFailingCommandRunner{},
		},
	})

	args := map[string]any{
		"topic":        topic,
		"maxSubagents": 0,
	}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-worker-failure",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.Result != nil && response.Result.PrimaryResult != nil && len(*response.Result.PrimaryResult) > 0 {
		t.Fatalf("primary result = %#v, want no completed success primary result after worker failure", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf(
			"dispatch count = %d, want one lead synthesis dispatch when maxSubagents is 0",
			len(dispatches.Dispatches),
		)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Label == nil || *dispatch.Label != "lead-research-synthesis" {
		t.Fatalf("dispatch label = %#v, want lead-research-synthesis", dispatch.Label)
	}
	if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || strings.TrimSpace(dispatch.FailureDetail.Message) == "" {
		t.Fatalf("dispatch failureDetail = %#v, want stable public failure record", dispatch.FailureDetail)
	}
}

type packagedDeepResearchFailingCommandRunner struct{}

func (packagedDeepResearchFailingCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("packaged deep research provider failure")
}

func packagedDeepResearchRejectingMockWorkersConfig() *workers.MockWorkersConfig {
	exitCode := 7
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stderr:   "packaged deep research mock worker failure",
				ExitCode: &exitCode,
			},
		}},
	}
}

func startPackagedDeepResearchInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	factoryDir string,
	requestID string,
	args map[string]any,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	workflowFile := filepath.Join(factoryDir, "scripts", "deep-research.workflow.js")
	return postJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		server.URL()+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: requestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
				WorkflowFile: &workflowFile,
			},
			Args:         &args,
			Orchestrator: factory.Orchestrator,
		},
		"start packaged deep-research invocation",
	)
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
		var payload bytes.Buffer
		_, _ = payload.ReadFrom(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload.String())
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func listFactorySessionDispatches(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	response, err := http.Get(strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/dispatches: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var payload bytes.Buffer
		_, _ = payload.ReadFrom(response.Body)
		t.Fatalf(
			"GET /factory-sessions/%s/dispatches status = %d, want 200: %s",
			sessionID,
			response.StatusCode,
			payload.String(),
		)
	}
	var decoded factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode dispatch list: %v", err)
	}
	return decoded
}
