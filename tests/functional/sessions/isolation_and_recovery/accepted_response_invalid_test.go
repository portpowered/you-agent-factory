package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Independent explicit sessions share one immutable process graph. The public
// resume command consumes real temporary recordings; provider observations are
// scoped by each scenario's Factory Session and never execute paid work.
func TestAcceptedResponseSurvivesInvalidRecordingWithoutRetry(t *testing.T) {
	t.Parallel()
	runner := &invalidAcceptedResponseRunner{calls: make(map[string]int)}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	for _, shape := range []string{"missing", "preview", "inconsistent"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, acceptedResponsePauseFactory())
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
			for _, name := range []string{"process", "finish"} {
				support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			}
			session := uuid.NewString()
			payload := invalidAcceptedRecording(t, shape, session)
			source := filepath.Join(dir, "accepted-source.json")
			if err := os.WriteFile(source, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			inputs := support.FakeInputs(ctx, []string{"you", "run", "--session", session, "--dir", dir, "--resume", source, "--quiet"})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err := process.Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), "cannot recover accepted dispatch") ||
				!strings.Contains(err.Error(), "will not be retried") {
				t.Fatalf("public resume = %v; stderr=%s", err, inputs.Stderr())
			}
			if strings.Contains(err.Error()+inputs.Stderr()+inputs.Stdout(), "PRIVATE-ACCEPTED-PAYLOAD") {
				t.Fatal("resume diagnostic exposed recorded output")
			}
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("failed resume changed its source recording")
			}
			runner.mu.Lock()
			calls := runner.calls[session]
			runner.mu.Unlock()
			if calls != 0 {
				t.Fatalf("failed accepted recovery executed provider %d times", calls)
			}
		})
	}
}

type invalidAcceptedResponseRunner struct {
	mu    sync.Mutex
	calls map[string]int
}

func (runner *invalidAcceptedResponseRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls[request.ExecutionScopeID]++
	runner.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider execution during accepted recovery")
}

func invalidAcceptedRecording(t *testing.T, shape, session string) []byte {
	t.Helper()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, false), &artifact); err != nil {
		t.Fatal(err)
	}
	snapshot, err := definitions.NewFactorySnapshot(acceptedResponsePauseFactory())
	if err != nil {
		t.Fatal(err)
	}
	base := artifact.RecordedAt
	artifact.Events[0] = seededReplayResumeEvent(t, "run-request", 0, 0, base, definitions.FactoryEventTypeRunRequest,
		definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base})
	dispatch := "dispatch-accepted"
	addEvent := func(kind definitions.FactoryEventType, payload any) {
		sequence := len(artifact.Events)
		event := seededReplayResumeEvent(t, fmt.Sprintf("accepted/%d", sequence), sequence, sequence, base.Add(time.Duration(sequence)*time.Second), kind, payload)
		event.Context.DispatchID = &dispatch
		event.Context.WorkIDs = &[]string{"work-seeded-replay-resume"}
		artifact.Events = append(artifact.Events, event)
	}
	addEvent(definitions.FactoryEventTypeDispatchRequest, definitions.DispatchRequestEventPayload{
		TransitionID: "process", Inputs: []definitions.DispatchConsumedWorkRef{{WorkID: "work-seeded-replay-resume"}},
	})
	addEvent(definitions.FactoryEventTypeModelRequest, workers.ModelRequestEventPayload{ModelRequestID: "accepted-model", Attempt: 1})
	if shape != "missing" {
		preview := "PRIVATE-ACCEPTED-PAYLOAD diagnostic only"
		response := workers.ModelResponseEventPayload{
			ModelRequestID: "accepted-model", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputPreview: &preview,
		}
		if shape == "inconsistent" {
			content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "PRIVATE-ACCEPTED-PAYLOAD complete"}}
			response.OutputContent = &content
			response.ModelRequestID = "different-model-request"
		}
		addEvent(definitions.FactoryEventTypeModelResponse, response)
	}
	addEvent(definitions.FactoryEventTypeAgentRunResponse, workers.AgentRunResponseEventPayload{AgentRunID: dispatch + "/agent-run/1", Outcome: "ACCEPTED"})
	for index := range artifact.Events {
		artifact.Events[index].Context.SessionID = &session
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
