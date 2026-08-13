package workers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMockWorkerCommandRunnerUsesGoalRoutingEnvelopePolicy(t *testing.T) {
	t.Parallel()

	runner := &MockWorkerCommandRunner{Config: &MockWorkersConfig{
		MockWorkers: []MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         MockWorkerRunTypeAccept,
		}},
	}}
	ctx := WithMockWorkerOutputPolicy(context.Background(), OutputPolicy{
		Format:                      "decision-envelope",
		DecisionEnvelope:            true,
		GoalRoutingDecisionEnvelope: true,
	})

	result, err := runner.Run(ctx, CommandRequest{
		Command:         "codex",
		WorkerType:      "goal-executor",
		WorkstationName: "execute-goal",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var envelope struct {
		Decision string `json:"decision"`
		Output   string `json:"output"`
	}
	for _, line := range strings.Split(strings.TrimSpace(string(result.Stdout)), "\n") {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("mock provider event %q is invalid JSON: %v", line, err)
		}
		if event.Type == "item.completed" {
			if err := json.Unmarshal([]byte(event.Item.Text), &envelope); err != nil {
				t.Fatalf("mock agent message %q is not a decision envelope: %v", event.Item.Text, err)
			}
			break
		}
	}
	if envelope.Decision != "accepted" || envelope.Output != defaultMockWorkerAcceptedOutput {
		t.Fatalf("mock decision envelope = %#v, want lower-case accepted routing label", envelope)
	}
}
