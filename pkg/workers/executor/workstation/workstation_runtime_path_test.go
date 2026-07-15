package workstation_test

import (
	"context"
	"path/filepath"
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
		workerOperations []workerconfig.ModelOperation
		wantModelOp      string
	}{
		{
			name:            "model workstation",
			workstationName: "review",
			workstationType: workertaxonomy.WorkstationTypeModel,
		},
		{
			name:            "invoke workstation",
			workstationName: "speak",
			workstationType: workertaxonomy.WorkstationTypeInvoke,
			operation:       "TTS",
			workerOperations: []workerconfig.ModelOperation{{
				Name: "TTS",
				Inputs: []workerconfig.ModelOperationSlot{
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
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					RuntimeBasePath: runtimeBaseDir,
					Workers: map[string]*workerconfig.Config{
						"worker-a": {Type: workertaxonomy.WorkerTypeModel, Body: "system", Operations: tc.workerOperations},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						tc.workstationName: {
							Type:           tc.workstationType,
							PromptTemplate: "Work from {{ .Context.WorkDir }}",
							Operation:      tc.operation,
							OperationBindings: []interfaces.ModelOperationBinding{{
								Slot: "text",
								Selector: &interfaces.ModelOperationBindingSelector{
									Type: workerconfig.ModelOperationContentTypeText,
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
				InputTokens: executor.InputTokens(factorytoken.Token{
					ID: "tok-1",
					Color: factorytoken.Color{
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
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			RuntimeBasePath: sessionRoot,
			Workers: map[string]*workerconfig.Config{
				"worker-a": {Type: workertaxonomy.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             workertaxonomy.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: `workspace/{{ .Context.SessionID }}`,
					Env: map[string]string{
						"SESSION_WORKDIR": `{{ .Context.WorkDir }}/workspace/{{ .Context.SessionID }}`,
					},
				},
			},
		},
		WorkflowContext: &factory_context.FactoryContext{SessionID: sessionID},
		Executor:        mock,
		Renderer:        &executor.DefaultPromptRenderer{},
	}

	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-session-template",
		TransitionID:    "t-session-template",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
