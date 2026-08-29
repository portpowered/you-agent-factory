package submission_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func configureSubmissionCodexWorkers(t *testing.T, factoryDir string, workerNames ...string) {
	t.Helper()

	for _, workerName := range workerNames {
		support.WriteAgentConfig(
			t,
			factoryDir,
			workerName,
			support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
		)
	}
}

func submissionServerConfig(
	factoryDir string,
	runner platformprocess.CommandRunner,
) support.FunctionalAPIServerConfig {
	return support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	}
}

func submissionStaticProviderRunner() platformprocess.CommandRunner {
	return support.NewStaticSuccessCommandRunner("Done. COMPLETE")
}

func submissionInputPreservingFactoryConfig() map[string]any {
	config := simplePipelineFactoryConfig()
	config["workstations"].([]map[string]any)[0]["outcomeFormat"] = "decision-envelope"
	return config
}

func submissionInputPreservingProviderRunner() platformprocess.CommandRunner {
	return submissionStaticCommandResultRunner{
		result: submissionAcceptedEmptyDecisionCommandResult(),
	}
}

func submissionAcceptedEmptyDecisionCommandResult() platformprocess.CommandResult {
	decision, err := json.Marshal(map[string]string{
		"decision": "ACCEPTED",
		"output":   "",
	})
	if err != nil {
		panic(err)
	}
	message, err := json.Marshal(string(decision))
	if err != nil {
		panic(err)
	}
	return platformprocess.CommandResult{Stdout: []byte(
		`{"type":"turn.started"}` + "\n" +
			`{"type":"item.completed","item":{"id":"message","type":"agent_message","text":` + string(message) + `}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n",
	)}
}

type submissionStaticCommandResultRunner struct {
	result platformprocess.CommandResult
}

func (runner submissionStaticCommandResultRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return runner.result, nil
}

func simplePipelineFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func competingPipelineFactoryConfig() map[string]any {
	config := simplePipelineFactoryConfig()
	config["workers"] = []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}}
	config["workstations"] = append(config["workstations"].([]map[string]any), map[string]any{
		"name":      "process-alternate",
		"worker":    "worker-b",
		"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	return config
}

func postSubmitWork(t *testing.T, baseURL string, body []byte) factoryapi.SubmitWorkResponse {
	t.Helper()

	endpoint := support.DefaultSessionWorkURL(baseURL, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 201: %s", endpoint, response.StatusCode, payload)
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return submitted
}

func postSubmitWorkExpectStatus(t *testing.T, baseURL string, body []byte, wantStatus int) {
	t.Helper()

	endpoint := support.DefaultSessionWorkURL(baseURL, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want %d: %s", endpoint, response.StatusCode, wantStatus, payload)
	}
}

func waitForWorkByTraceAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	listed, err := support.WaitForObservation(
		timeout,
		func() (factoryapi.ListWorkResponse, error) {
			return support.ListDefaultSessionWork(t, baseURL), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			for _, item := range listed.Results {
				if support.StringPointerValue(item.TraceId) == traceID &&
					workCustomerPlaceID(item) == placeID {
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf(
			"timed out waiting for trace %q at %s: %v; last work response: %#v",
			traceID,
			placeID,
			err,
			listed.Results,
		)
	}
	return listed
}

func waitForWorkByTraceComplete(
	t *testing.T,
	baseURL string,
	traceID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()
	return waitForWorkByTraceAtPlace(t, baseURL, traceID, "task:complete", timeout)
}

func waitForWorkIDsComplete(
	t *testing.T,
	baseURL string,
	workIDs []string,
	timeout time.Duration,
) []factoryapi.Work {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	var found map[string]factoryapi.Work
	listed, err := support.WaitForObservation(
		timeout,
		func() (factoryapi.ListWorkResponse, error) {
			return support.ListDefaultSessionWork(t, baseURL), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			currentFound := make(map[string]factoryapi.Work, len(want))
			for _, item := range listed.Results {
				workID := support.StringPointerValue(item.WorkId)
				if want[workID] && workStateName(item.State) == "complete" {
					currentFound[workID] = item
				}
			}
			found = currentFound
			return len(currentFound) == len(want)
		},
	)
	if err != nil {
		t.Fatalf(
			"timed out waiting for completed work IDs %v: %v; last work response: %#v",
			workIDs,
			err,
			listed.Results,
		)
	}
	items := make([]factoryapi.Work, 0, len(workIDs))
	for _, workID := range workIDs {
		items = append(items, found[workID])
	}
	return items
}

func waitForWorkByNameComplete(
	t *testing.T,
	baseURL string,
	workName string,
	workType string,
	timeout time.Duration,
) factoryapi.Work {
	t.Helper()

	var found factoryapi.Work
	listed, err := support.WaitForObservation(
		timeout,
		func() (factoryapi.ListWorkResponse, error) {
			return support.ListDefaultSessionWork(t, baseURL), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			matches := 0
			for _, item := range listed.Results {
				if item.Name == workName &&
					support.StringPointerValue(item.WorkTypeName) == workType &&
					workStateName(item.State) == "complete" {
					found = item
					matches++
				}
			}
			return matches == 1
		},
	)
	if err != nil {
		t.Fatalf(
			"timed out waiting for completed Work name %q type %q: %v; last work response: %#v",
			workName,
			workType,
			err,
			listed.Results,
		)
	}
	return found
}

func requireWorkByTrace(t *testing.T, listed factoryapi.ListWorkResponse, traceID string) factoryapi.Work {
	t.Helper()

	matches := 0
	var found factoryapi.Work
	for _, item := range listed.Results {
		if support.StringPointerValue(item.TraceId) == traceID {
			found = item
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("trace %q matched %d Work items, want exactly one: %#v", traceID, matches, listed.Results)
	}
	return found
}

func workStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func workCustomerPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return support.StringPointerValue(work.WorkTypeName) + ":"
	}
	return support.StringPointerValue(work.WorkTypeName) + ":" + work.State.Name
}

func functionalServerBaseURL(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse functional server URL %q: %v", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("functional server URL %q missing scheme or host", rawURL)
	}
	return strings.TrimSuffix(rawURL, "/")
}
