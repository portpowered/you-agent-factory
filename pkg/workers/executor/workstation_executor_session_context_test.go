package executor

import (
	"context"
	"testing"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
			mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}}
			we := newTestWorkstationExecutor(
				staticRuntimeConfig{
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
				mock,
			)
			we.WorkflowContext = &factory_context.FactoryContext{SessionID: tc.sessionID}

			_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
				DispatchID:      "d-session",
				TransitionID:    "t-session",
				WorkerType:      "worker-a",
				WorkstationName: "standard",
				InputTokens: InputTokens(interfaces.Token{
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
