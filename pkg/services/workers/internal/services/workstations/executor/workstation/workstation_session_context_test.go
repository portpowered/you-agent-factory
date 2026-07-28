package workstation_test

import (
	"context"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
)

type sessionCapturingExecutor struct {
	dispatch workerexecution.WorkstationExecutionRequest
}

func (m *sessionCapturingExecutor) Execute(_ context.Context, d workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.dispatch = d
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func TestWorkstationExecutor_PromptRendersFactorySessionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: workerexecution.DefaultSessionID, want: workerexecution.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mock := &sessionCapturingExecutor{}
			we := &executor.WorkstationExecutor{
				Now: time.Now,
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					Workers: map[string]*interfaces.FactoryWorkerConfig{
						"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"standard": {
							Type:           interfaces.WorkstationTypeModel,
							PromptTemplate: `submit --session {{ .Context.SessionID }}`,
						},
					},
				},
				WorkflowContext: &workerexecution.Context{SessionID: tc.sessionID},
				Executor:        mock,
				Renderer:        &executor.DefaultPromptRenderer{},
			}

			_, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "d-session",
				TransitionID:    "t-session",
				WorkerType:      "worker-a",
				WorkstationName: "standard",
				InputTokens: executor.InputTokens(factoryruntime.RuntimeToken{
					ID:    "tok-1",
					Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"},
				}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mock.dispatch.UserMessage != "submit --session "+tc.want {
				t.Fatalf("user message = %q, want session %q", mock.dispatch.UserMessage, tc.want)
			}
			if mock.dispatch.FactorySessionID != tc.want {
				t.Fatalf("FactorySessionID = %q, want %q", mock.dispatch.FactorySessionID, tc.want)
			}
		})
	}
}
