package workstation_test

import (
	"context"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
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
		{name: "default session", sessionID: factory_context.DefaultSessionID, want: factory_context.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mock := &sessionCapturingExecutor{}
			we := &executor.WorkstationExecutor{
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					Workers: map[string]*workerconfig.Config{
						"worker-a": {Type: workertaxonomy.WorkerTypeModel, Body: "system"},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"standard": {
							Type:           workertaxonomy.WorkstationTypeModel,
							PromptTemplate: `submit --session {{ .Context.SessionID }}`,
						},
					},
				},
				WorkflowContext: &factory_context.FactoryContext{SessionID: tc.sessionID},
				Executor:        mock,
				Renderer:        &executor.DefaultPromptRenderer{},
			}

			_, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "d-session",
				TransitionID:    "t-session",
				WorkerType:      "worker-a",
				WorkstationName: "standard",
				InputTokens: executor.InputTokens(factorytoken.Token{
					ID:    "tok-1",
					Color: factorytoken.Color{WorkID: "work-1"},
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
