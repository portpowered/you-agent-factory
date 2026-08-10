//go:build functionallong

package execution

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const structuredPromptResult = `{"summary":{"title":"upstream structured title"},"items":[{"name":"first structured item"}]}`

// TestStructuredResultFlowsIntoDownstreamPromptThroughRootBuildProcess proves
// the customer-visible multi-stage handoff: the first provider response is
// validated against outputSchema, then its nested object and array values are
// rendered into the actual second provider command request.
func TestStructuredResultFlowsIntoDownstreamPromptThroughRootBuildProcess(t *testing.T) {
	dir := support.ScaffoldFactory(t, structuredPromptFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", structuredPromptWorkerConfig())
	support.WriteWorkstationConfig(t, dir, "produce", structuredPromptWorkstationConfig(
		"Produce the structured handoff.",
		structuredPromptSchema,
	))
	support.WriteWorkstationConfig(t, dir, "consume", structuredPromptWorkstationConfig(
		`Title={{ (index .Inputs 0).StructuredResult.summary.title }} Item={{ (index (index (index .Inputs 0).StructuredResult "items") 0).name }}`,
		"",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"structured handoff"}`))

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(structuredPromptResult)},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("downstream COMPLETE")},
	)
	session, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("completed task count = %d, want 1; listed=%#v", got, listed)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %d, want produce and consume dispatches", len(requests))
	}
	consumePrompt := string(requests[1].Stdin)
	for _, want := range []string{"upstream structured title", "first structured item"} {
		if !strings.Contains(consumePrompt, want) {
			t.Fatalf("downstream provider prompt = %q, want structured value %q", consumePrompt, want)
		}
	}
}

// TestStructuredOutputSchemaViolationRoutesFailureWithoutDownstreamDispatch proves
// that malformed or schema-invalid worker output is a terminal workstation
// failure, retains raw output only in the response event, and cannot enter the
// Work projection or a downstream prompt.
func TestStructuredOutputSchemaViolationRoutesFailureWithoutDownstreamDispatch(t *testing.T) {
	const rejectedMarker = "do-not-leak-this-rejected-value"
	for _, test := range []struct {
		name     string
		schema   string
		response string
	}{
		{name: "malformed_json", schema: structuredPromptSchema, response: `{"summary":`},
		{
			name:     "schema_mismatch_pattern",
			schema:   `{"type":"string","pattern":"^ok$"}`,
			response: `"` + rejectedMarker + `"`,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dir := support.ScaffoldFactory(t, structuredPromptFactoryConfig())
			support.WriteAgentConfig(t, dir, "processor", structuredPromptWorkerConfig())
			support.WriteWorkstationConfig(t, dir, "produce", structuredPromptWorkstationConfig(
				"Produce the structured handoff.",
				test.schema,
			))
			support.WriteWorkstationConfig(t, dir, "consume", structuredPromptWorkstationConfig(
				`Title={{ (index .Inputs 0).StructuredResult.summary.title }}`,
				"",
			))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"structured handoff"}`))

			runner := testutil.NewProviderCommandRunner(
				platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(test.response)},
			)
			session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
				t,
				dir,
				serviceedges.Edges{ProviderCommandRunner: runner},
				30*time.Second,
			)

			if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 1 {
				t.Fatalf("session progress = %+v, want zero terminal and one failed", session.Runtime.Progress.Categories)
			}
			if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
				t.Fatalf("failed task count = %d, want 1; listed=%#v", got, listed)
			}
			if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 0 {
				t.Fatalf("completed task count = %d, want 0 after schema violation", got)
			}
			for _, item := range listed.Results {
				if item.State == nil || item.State.Name != "failed" {
					continue
				}
				if item.StructuredResult != nil || strings.Contains(fmt.Sprint(item.StructuredResult), rejectedMarker) {
					t.Fatalf("failed Work structured result = %#v, want rejected value absent", item.StructuredResult)
				}
			}

			observations := support.ObserveDispatchEvents(t, events)
			if len(observations) != 1 || observations[0].Response == nil {
				t.Fatalf("dispatch observations = %#v, want one completed produce dispatch", observations)
			}
			response := observations[0].Response
			if response.FailureDetail == nil || response.FailureDetail.Reason != factoryapi.WorkFailureTypeStructuredOutputSchemaViolation {
				t.Fatalf("failure detail = %#v, want structured schema violation", response.FailureDetail)
			}
			if !strings.Contains(response.FailureDetail.Message, "structured output schema violation") {
				t.Fatalf("failure message = %q, want stable schema-violation diagnostic", response.FailureDetail.Message)
			}
			if strings.Contains(response.FailureDetail.Message, rejectedMarker) {
				t.Fatalf("failure message = %q, want rejected response value excluded", response.FailureDetail.Message)
			}
			if response.Output == nil || !strings.Contains(*response.Output, test.response) {
				t.Fatalf("response output = %#v, want raw provider output retained", response.Output)
			}
			if len(runner.Requests()) != 1 {
				t.Fatalf("provider requests = %d, want one failed produce dispatch and no downstream consume", len(runner.Requests()))
			}
		})
	}
}

const structuredPromptSchema = `{"type":"object","properties":{"summary":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]},"items":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}},"required":["summary","items"]}`

func structuredPromptFactoryConfig() map[string]any {
	return map[string]any{
		"name": "structured-result-prompt",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "produced", "type": "PROCESSING"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{
			{
				"name":      "produce",
				"worker":    "processor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "produced"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "consume",
				"worker":    "processor",
				"inputs":    []map[string]string{{"workType": "task", "state": "produced"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "done"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func structuredPromptWorkerConfig() string {
	return "---\n" +
		"type: MODEL_WORKER\n" +
		"model: structured-prompt-model\n" +
		"modelProvider: " + string(modelprovider.ProviderCodex) + "\n" +
		"---\nProcess the work item.\n"
}

func structuredPromptWorkstationConfig(prompt, schema string) string {
	config := "---\n" + "type: MODEL_WORKSTATION\n"
	if schema != "" {
		config += "outputSchema: '" + strings.ReplaceAll(schema, "'", "''") + "'\n"
	}
	return config + "---\n" + prompt + "\n"
}
