package invocation

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestActivationOnlyStartRequest(t *testing.T) {
	skip := true
	target := roles.InvocationTarget{
		FactorySessionID:                 "session-1",
		FactoryDir:                       "/factories/alpha",
		FactorySourcePath:                "/factories/alpha/you.yaml",
		RunnerID:                         "runner-1",
		Worktree:                         "wt-1",
		WorkerReasoningEffort:            "high",
		ExecutionBaseDir:                 "/exec",
		HomeDir:                          "/home/op",
		Verbose:                          true,
		CanonicalSessionID:               "canonical-1",
		ModelCacheDir:                    "/models",
		WorkflowID:                       "wf-1",
		RecordPath:                       "/rec",
		ReplayPath:                       "/replay",
		ResumePath:                       "/resume",
		MockWorkersConfig:                &workers.MockWorkersConfig{},
		SkipPermissionsOverride:          &skip,
		SkipRunnerPrerequisiteValidation: true,
		InvocationArguments: &work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"color": {Values: []string{"blue"}, ValueMode: "single"},
			},
		},
	}
	roots := factoryruntime.RuntimeArtifactRoots{Logs: "/logs", Metrics: "/metrics"}

	got := ActivationOnlyStartRequest(target, roots)
	if !got.ActivationOnly {
		t.Fatal("ActivationOnly must be true")
	}
	if got.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", got.SessionID)
	}
	if got.FolderPath != target.FactoryDir {
		t.Fatalf("FolderPath = %q", got.FolderPath)
	}
	assertActivationRuntimeSelection(t, got)
	assertActivationClonesInputs(t, target, got)
	if got.RuntimeSelection.DefinitionSourcePath != target.FactorySourcePath || got.Definition.SourceRef != target.FactorySourcePath {
		t.Fatalf("definition source = %q %q", got.RuntimeSelection.DefinitionSourcePath, got.Definition.SourceRef)
	}
}

func assertActivationRuntimeSelection(t *testing.T, got factorysessions.SessionStartRequest) {
	t.Helper()
	sel := got.RuntimeSelection
	if sel == nil {
		t.Fatal("RuntimeSelection must not be nil")
	}
	if sel.CanonicalSessionID != "canonical-1" {
		t.Fatalf("CanonicalSessionID = %q", sel.CanonicalSessionID)
	}
	if sel.ModelCacheDirectory != "/models" {
		t.Fatalf("ModelCacheDirectory = %q", sel.ModelCacheDirectory)
	}
	if sel.LogDirectory != "/logs" || sel.MetricsDirectory != "/metrics" {
		t.Fatalf("artifact dirs = %q %q", sel.LogDirectory, sel.MetricsDirectory)
	}
	if sel.Workers.RunnerID != "runner-1" || sel.Workers.Worktree != "wt-1" || sel.Workers.WorkerReasoningEffort != "high" {
		t.Fatalf("worker selection = %+v", sel.Workers)
	}
	if sel.Workers.MockWorkers == nil || sel.Workers.InvocationSkipPermissionsOverride == nil || !sel.Workers.SkipBuiltInPrerequisiteValidation {
		t.Fatalf("worker flags = %+v", sel.Workers)
	}
	if sel.Host.Port != 0 {
		t.Fatalf("Host.Port = %d", sel.Host.Port)
	}
}

func assertActivationClonesInputs(t *testing.T, target roles.InvocationTarget, got factorysessions.SessionStartRequest) {
	t.Helper()
	sel := got.RuntimeSelection
	if sel.DefinitionInvocationArguments == nil {
		t.Fatal("DefinitionInvocationArguments must be cloned")
	}
	// Mutate output; input must be immutable.
	sel.DefinitionInvocationArguments.Arguments["color"] = work.InvocationArgument{Values: []string{"red"}}
	if target.InvocationArguments.Arguments["color"].Values[0] != "blue" {
		t.Fatal("input InvocationArguments was mutated")
	}
	if sel.Workers.MockWorkers == target.MockWorkersConfig {
		t.Fatal("MockWorkersConfig was aliased")
	}
	sel.Workers.MockWorkers = &workers.MockWorkersConfig{}
	if target.MockWorkersConfig == nil {
		t.Fatal("input MockWorkersConfig was mutated")
	}
	*sel.Workers.InvocationSkipPermissionsOverride = false
	if *target.SkipPermissionsOverride != true {
		t.Fatal("input SkipPermissionsOverride was mutated")
	}
}
