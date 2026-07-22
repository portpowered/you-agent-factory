package workstation_test

import (
	"context"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/executor"
)

type runtimePathCapturingExecutor struct {
	dispatch workerexecution.WorkstationExecutionRequest
}

func (m *runtimePathCapturingExecutor) Execute(_ context.Context, d workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.dispatch = d
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func TestWorkstationExecutor_DefaultsEmptyExecutionPathToRuntimeBaseDirectoryAcrossModelWorkstationKinds(t *testing.T) {
	runtimeBaseDir := t.TempDir()

	tests := []struct {
		name             string
		workstationName  string
		workstationType  string
		operation        string
		workerOperations []interfaces.ModelOperation
		wantModelOp      string
	}{
		{
			name:            "model workstation",
			workstationName: "review",
			workstationType: interfaces.WorkstationTypeModel,
		},
		{
			name:            "invoke workstation",
			workstationName: "speak",
			workstationType: interfaces.WorkstationTypeInvoke,
			operation:       "TTS",
			workerOperations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{
					{Name: "text", Required: true},
				},
			}},
			wantModelOp: "TTS",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mock := &runtimePathCapturingExecutor{}
			we := &executor.WorkstationExecutor{
				Now: time.Now,
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					RuntimeBasePath: runtimeBaseDir,
					Workers: map[string]*interfaces.FactoryWorkerConfig{
						"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system", Operations: tc.workerOperations},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						tc.workstationName: {
							Type:           tc.workstationType,
							PromptTemplate: "Work from {{ .Context.WorkDir }}",
							Operation:      tc.operation,
							OperationBindings: []interfaces.ModelOperationBinding{{
								Slot: "text",
								Selector: &interfaces.ModelOperationBindingSelector{
									Type: interfaces.ModelOperationContentTypeText,
								},
							}},
						},
					},
				},
				Executor: mock,
				Renderer: &executor.DefaultPromptRenderer{},
			}

			_, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "d-default-runtime-dir",
				TransitionID:    "t-default-runtime-dir",
				WorkerType:      "worker-a",
				WorkstationName: tc.workstationName,
				InputTokens: executor.InputTokens(factoryruntime.RuntimeToken{
					ID: "tok-1",
					Color: factoryruntime.RuntimeTokenColor{
						WorkID: "work-1",
						Content: []work.WorkContentPart{{
							Type: work.WorkContentPartTypeText,
							Slot: "text",
							Text: "hello",
						}},
					},
				}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mock.dispatch.WorkingDirectory != runtimeBaseDir {
				t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, runtimeBaseDir)
			}
			if mock.dispatch.WorkingDirectoryAuthored {
				t.Fatal("WorkingDirectoryAuthored = true, want false for default runtime root")
			}
			if mock.dispatch.UserMessage != "Work from "+runtimeBaseDir {
				t.Fatalf("user message = %q, want runtime root", mock.dispatch.UserMessage)
			}
			if mock.dispatch.ModelOperation != tc.wantModelOp {
				t.Fatalf("model operation = %q, want %q", mock.dispatch.ModelOperation, tc.wantModelOp)
			}
		})
	}
}

func TestWorkstationExecutor_ResolvesTemplatedWorkingDirectoryFromSessionContext(t *testing.T) {
	sessionRoot := t.TempDir()
	sessionID := "session-beta"
	wantDir := filepath.Join(sessionRoot, "workspace", sessionID)

	mock := &runtimePathCapturingExecutor{}
	we := &executor.WorkstationExecutor{
		Now: time.Now,
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			RuntimeBasePath: sessionRoot,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: `workspace/{{ .Context.SessionID }}`,
					Env: map[string]string{
						"SESSION_WORKDIR": `{{ .Context.WorkDir }}/workspace/{{ .Context.SessionID }}`,
					},
				},
			},
		},
		WorkflowContext: &workerexecution.Context{SessionID: sessionID},
		Executor:        mock,
		Renderer:        &executor.DefaultPromptRenderer{},
	}

	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-session-template",
		TransitionID:    "t-session-template",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.dispatch.WorkingDirectory != wantDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantDir)
	}
	if !mock.dispatch.WorkingDirectoryAuthored {
		t.Fatal("WorkingDirectoryAuthored = false, want true for templated workstation path")
	}
	if mock.dispatch.UserMessage != "Work from "+wantDir {
		t.Fatalf("user message = %q, want %q", mock.dispatch.UserMessage, "Work from "+wantDir)
	}
	if mock.dispatch.FactorySessionID != sessionID {
		t.Fatalf("FactorySessionID = %q, want %q", mock.dispatch.FactorySessionID, sessionID)
	}
	if filepath.Clean(mock.dispatch.EnvVars["SESSION_WORKDIR"]) != wantDir {
		t.Fatalf("env vars = %#v, want SESSION_WORKDIR=%q", mock.dispatch.EnvVars, wantDir)
	}
}
