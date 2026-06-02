package workstation_test

import (
	"context"
	"testing"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
)

type sessionCapturingExecutor struct {
	dispatch interfaces.WorkstationExecutionRequest
}

func (m *sessionCapturingExecutor) Execute(_ context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.dispatch = d
	return interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}, nil
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
					Workers: map[string]*interfaces.WorkerConfig{
						"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"standard": {
							Type:           interfaces.WorkstationTypeModel,
							PromptTemplate: `submit --session {{ .Context.SessionID }}`,
						},
					},
				},
				WorkflowContext: &factory_context.FactoryContext{SessionID: tc.sessionID},
				Executor:        mock,
				Renderer:        &executor.DefaultPromptRenderer{},
			}

			_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
				DispatchID:      "d-session",
				TransitionID:    "t-session",
				WorkerType:      "worker-a",
				WorkstationName: "standard",
				InputTokens: executor.InputTokens(interfaces.Token{
					ID:    "tok-1",
					Color: interfaces.TokenColor{WorkID: "work-1"},
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
