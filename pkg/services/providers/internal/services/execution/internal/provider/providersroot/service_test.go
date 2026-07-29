package providersroot

import (
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecutePreservesWorkerContextForNativeProviders(t *testing.T) {
	runner := &workers.MockWorkerCommandRunner{
		Config: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	}
	factory, err := workerprovider.NewFactory(
		runner,
		workers.ClockFunc(testClock),
		&agypty.MockAllocator{},
		filepath.EvalSymlinks,
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	root, err := NewService(Config{Factory: factory})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:        providers.IDKiro,
		AttemptID:       "dispatch-goal-1",
		WorkerType:      "goal-executor",
		WorkstationName: "execute-goal",
		SystemPrompt:    "system",
		UserMessage:     "user",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "mock worker accepted" {
		t.Fatalf("result.Content = %q, want mock worker accepted", result.Content)
	}
}

func TestNewServiceRequiresFactory(t *testing.T) {
	_, err := NewService(Config{})
	if err == nil {
		t.Fatal("NewService() error = nil, want factory required")
	}
}

func TestInferenceRequestForwardsEnvFields(t *testing.T) {
	request := providers.ExecuteRequest{
		Provider:           providers.IDCodex,
		AttemptID:          "dispatch-env-1",
		WorkerType:         "goal-executor",
		WorkstationName:    "execute-goal",
		EnvVars:            map[string]string{"FIXTURE": "configured"},
		ProcessEnvironment: []string{"FIXTURE=configured"},
	}
	infer := inferenceRequest(request)
	if infer.EnvVars["FIXTURE"] != "configured" {
		t.Fatalf("EnvVars = %#v, want configured env", infer.EnvVars)
	}
	if len(infer.ProcessEnvironment) != 1 || infer.ProcessEnvironment[0] != "FIXTURE=configured" {
		t.Fatalf("ProcessEnvironment = %#v, want forwarded process env", infer.ProcessEnvironment)
	}
}

func testClock() time.Time {
	return time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
}
