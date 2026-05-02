//go:build functionallong

package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamePropagation_InPromptTemplate(t *testing.T) {
	support.SkipLongFunctional(t, "slow prompt-template name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "design-doc-review",
		WorkTypeID: "task",
		Payload:    []byte(`review the design document`),
		TraceID:    "trace-prompt-test",
	})

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)
	h.Assert().HasTokenInPlace("task:complete")

	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}
	if userMessage := providerCalls[0].UserMessage; !strings.Contains(userMessage, "Task Name: design-doc-review") {
		t.Errorf("expected rendered prompt to contain 'Task Name: design-doc-review', got:\n%s", userMessage)
	}
}

func TestNamePropagation_MarkdownFile(t *testing.T) {
	support.SkipLongFunctional(t, "slow markdown name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))

	testutil.WriteSeedMarkdownFile(t, dir, "task", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init")

	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}

	userMessage := providerCalls[0].UserMessage
	if !strings.Contains(userMessage, "Task Name: architecture-review") {
		t.Errorf("expected rendered prompt to contain 'Task Name: architecture-review', got:\n%s", userMessage)
	}
	if !strings.Contains(userMessage, "# Architecture Review") {
		t.Errorf("expected raw markdown content in rendered prompt, got:\n%s", userMessage)
	}

	marking := h.Marking()
	for _, token := range marking.Tokens {
		if token.PlaceID == "task:complete" && token.Color.Name != "architecture-review" {
			t.Errorf("expected Name 'architecture-review' on completed token, got %q", token.Color.Name)
		}
	}
}

type spawningExecutor struct {
	mu    sync.Mutex
	calls []interfaces.WorkDispatch
}

func (s *spawningExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, dispatch)
	callNum := len(s.calls)
	s.mu.Unlock()

	result := interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}

	if callNum == 1 {
		parentWorkID := ""
		if len(dispatch.InputTokens) > 0 {
			parentWorkID = firstInputToken(dispatch.InputTokens).Color.WorkID
		}
		result.SpawnedWork = []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     fmt.Sprintf("%s-child", parentWorkID),
			Name:       "spawned-subtask",
			DataType:   interfaces.DataTypeWork,
			ParentID:   parentWorkID,
			Payload:    []byte(`child payload`),
		}}
	}

	return result, nil
}

func (s *spawningExecutor) getCalls() []interfaces.WorkDispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]interfaces.WorkDispatch(nil), s.calls...)
}

func TestNamePropagation_SpawnedChildWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow spawned-child name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "parent-task",
		WorkTypeID: "task",
		Payload:    []byte(`parent payload`),
		TraceID:    "trace-spawn-test",
	})

	spawnExec := &spawningExecutor{}
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Child done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("executor", spawnExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	marking := h.Marking()
	parentFound := false
	childFound := false
	for _, token := range marking.Tokens {
		if token.PlaceID == "task:complete" && token.Color.Name == "parent-task" {
			parentFound = true
		}
		if token.Color.Name == "spawned-subtask" {
			childFound = true
		}
	}

	if !parentFound {
		t.Error("expected parent token with Name 'parent-task' in task:complete")
	}
	if !childFound {
		t.Error("expected spawned child token with Name 'spawned-subtask' in marking")
	}

	calls := spawnExec.getCalls()
	if len(calls) < 1 {
		t.Fatal("expected at least 1 dispatch to spawning executor")
	}
	if firstInputToken(calls[0].InputTokens).Color.Name != "parent-task" {
		t.Errorf("expected parent Name 'parent-task' in dispatch, got %q", firstInputToken(calls[0].InputTokens).Color.Name)
	}
}
