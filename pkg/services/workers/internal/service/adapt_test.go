package service

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestAdaptRunnerRequestPreservesDetachedInputIdentityAndDispatchFacts(t *testing.T) {
	t.Parallel()

	original := workers.Token{
		ID:    "token-1",
		State: "draft",
		Color: workers.Color{
			Name:                     "source-name",
			RequestID:                "request-1",
			WorkID:                   "work-1",
			WorkTypeID:               "task",
			DataType:                 workers.DataTypeWork,
			ChainingTraceDepth:       2,
			CurrentChainingTraceID:   "input-chain",
			PreviousChainingTraceIDs: []string{"input-previous"},
			TraceID:                  "old-trace",
			ParentID:                 "old-parent",
			Tags:                     map[string]string{"old": "tag"},
			Relations:                []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "work-0"}},
			Content:                  []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "old-content"}},
			Payload:                  []byte("old-payload"),
			StructuredResult:         map[string]any{"answer": "old"},
			StructuredResultPresent:  true,
			InvocationArguments:      &work.InvocationArguments{},
		},
		CreatedAt: time.Unix(10, 0),
		EnteredAt: time.Unix(11, 0),
		History: workers.History{
			TotalVisits: map[string]int{"transition-1": 2},
			LastError:   "old-error",
			FailureLog: []workers.Failure{{
				TransitionID: "transition-1",
				Error:        "old-error",
				Attempt:      2,
			}},
		},
	}
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       "dispatch-1",
			RequestID:        "request-2",
			TraceID:          "trace-2",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "worker",
			WorkerType:      "worker-type",
			WorkstationName: "station",
			RunnerID:        runners.ScriptIdentity,
			Command:         "run-worker",
			Args:            []string{"--detached"},
			Environment: workers.EnvironmentPolicy{
				Vars:               map[string]string{"WORKER_ENV": "set"},
				ProcessEnvironment: []string{"PROCESS_ENV=set"},
				WorkingDirectory:   "working-directory",
			},
			Workspace: workers.WorkspacePolicy{
				Worktree: "worktree-1",
			},
		},
		Input: workers.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:               "dispatch-1",
				TransitionID:             "transition-1",
				WorkerType:               "dispatch-worker",
				WorkstationName:          "dispatch-station",
				ProjectID:                "project-1",
				CurrentChainingTraceID:   "dispatch-chain",
				PreviousChainingTraceIDs: []string{"dispatch-previous"},
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: workers.InputTokens(original),
				InputBindings: map[string][]string{
					"input": {"token-1"},
				},
			},
			Work: []workers.WorkInput{{
				Kind:       string(workers.DataTypeWork),
				State:      "review",
				InputNames: []string{"input"},
				WorkID:     "work-1",
				Name:       "detached-name",
				WorkTypeID: "task",
				RequestID:  "request-2",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "new-content",
				}},
				Tags:      map[string]string{"new": "tag"},
				Relations: []work.Relation{{Type: work.RelationParentChild, TargetWorkID: "work-2"}},
				Lineage: workers.WorkLineage{
					ParentWorkID: "new-parent",
					TraceID:      "new-trace",
					OriginRef:    "detached-name",
				},
				AttemptFacts: workers.AttemptFacts{
					AttemptNumber: 3,
					LastFailure:   "new-failure",
				},
			}},
		},
	}

	got := adaptRunnerRequest(request, runners.ScriptIdentity, nil)
	tokens := workers.WorkDispatchInputTokens(got.Dispatch)
	if len(tokens) != 1 {
		t.Fatalf("adapted input tokens = %#v, want one token", tokens)
	}
	token := tokens[0]
	if token.ID != "token-1" || token.State != "review" {
		t.Fatalf("adapted token identity/state = (%q, %q), want (token-1, review)", token.ID, token.State)
	}
	if token.Color.Name != "detached-name" || token.Color.RequestID != "request-2" ||
		token.Color.TraceID != "new-trace" || token.Color.ParentID != "new-parent" ||
		token.Color.Content[0].Text != "new-content" || token.Color.Payload == nil ||
		string(token.Color.Payload) != "new-content" {
		t.Fatalf("adapted token product facts = %#v", token.Color)
	}
	if token.Color.Tags["new"] != "tag" || len(token.Color.Relations) != 1 ||
		token.Color.Relations[0].TargetWorkID != "work-2" {
		t.Fatalf("adapted token metadata = %#v", token.Color)
	}
	if token.Color.ChainingTraceDepth != 2 || token.Color.CurrentChainingTraceID != "input-chain" ||
		len(token.Color.PreviousChainingTraceIDs) != 1 || token.Color.PreviousChainingTraceIDs[0] != "input-previous" {
		t.Fatalf("adapted token chaining facts = %#v", token.Color)
	}
	if token.History.TotalVisits["transition-1"] != 2 || token.History.LastError != "old-error" ||
		len(token.History.FailureLog) != 1 {
		t.Fatalf("adapted token history = %#v", token.History)
	}
	if token.Color.StructuredResult.(map[string]any)["answer"] != "old" ||
		!token.Color.StructuredResultPresent {
		t.Fatalf("adapted structured result = %#v (present=%t)", token.Color.StructuredResult, token.Color.StructuredResultPresent)
	}
	if got.Dispatch.CurrentChainingTraceID != "dispatch-chain" ||
		len(got.Dispatch.PreviousChainingTraceIDs) != 1 ||
		got.Dispatch.PreviousChainingTraceIDs[0] != "dispatch-previous" ||
		got.Dispatch.InputBindings["input"][0] != "token-1" {
		t.Fatalf("adapted dispatch facts = %#v", got.Dispatch)
	}
	if got.Command != "run-worker" || got.Args[0] != "--detached" ||
		got.EnvVars["WORKER_ENV"] != "set" || got.ProcessEnvironment[0] != "PROCESS_ENV=set" ||
		got.WorkingDirectory != "working-directory" || got.Worktree != "worktree-1" {
		t.Fatalf("adapted execution policy = %#v", got)
	}
}

func TestAdaptRunnerRequestProjectsInputNamesAndKindsWithoutOriginalTokens(t *testing.T) {
	t.Parallel()

	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       "dispatch-1",
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
		Target: workers.ExecutionTarget{
			RunnerID: runners.ScriptIdentity,
		},
		Input: workers.ExecutionInput{
			Work: []workers.WorkInput{
				{
					Kind:       string(workers.DataTypeWork),
					State:      "review",
					InputNames: []string{"primary"},
					WorkID:     "work-1",
					Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "work"}},
				},
				{
					Kind:       string(workers.DataTypeResource),
					State:      "ready",
					InputNames: []string{"capacity"},
					WorkID:     "resource-1",
					Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "resource"}},
				},
			},
		},
	}

	got := adaptRunnerRequest(request, runners.ScriptIdentity, nil)
	tokens := workers.WorkDispatchInputTokens(got.Dispatch)
	if len(tokens) != 2 || tokens[0].ID != "work-input-0" || tokens[1].ID != "work-input-1" {
		t.Fatalf("generated input identities = %#v, want stable detached identities", tokens)
	}
	if tokens[0].Color.DataType != workers.DataTypeWork || tokens[1].Color.DataType != workers.DataTypeResource ||
		tokens[0].State != "review" || tokens[1].State != "ready" {
		t.Fatalf("generated input facts = %#v, want ordered work/resource states", tokens)
	}
	if got.Dispatch.InputBindings["primary"][0] != "work-input-0" ||
		got.Dispatch.InputBindings["capacity"][0] != "work-input-1" {
		t.Fatalf("generated input bindings = %#v", got.Dispatch.InputBindings)
	}
}
