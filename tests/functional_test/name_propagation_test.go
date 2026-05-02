package functional_test

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
)

// TestNamePropagation_InPromptTemplate verifies that the Name field is
// available in prompt templates via canonical .Inputs access and appears in the rendered
// prompt sent to the provider.
func TestNamePropagation_InPromptTemplate(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "name_propagation"))

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

	// Verify the rendered prompt contains the name from the template.
	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}

	userMessage := providerCalls[0].UserMessage
	if !strings.Contains(userMessage, "Task Name: design-doc-review") {
		t.Errorf("expected rendered prompt to contain 'Task Name: design-doc-review', got:\n%s", userMessage)
	}
}

// TestNamePropagation_MarkdownFile verifies that submitting a .md file (not JSON)
// derives the Name from the filename and that the template renders it correctly.
func TestNamePropagation_MarkdownFile(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "name_propagation"))

	// Seed a markdown file instead of a JSON SubmitRequest.
	// The filewatcher should derive Name = "architecture-review" from the filename.
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

	// 1. Verify the rendered prompt template contains the filename-derived Name.
	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}

	userMessage := providerCalls[0].UserMessage
	if !strings.Contains(userMessage, "Task Name: architecture-review") {
		t.Errorf("expected rendered prompt to contain 'Task Name: architecture-review', got:\n%s", userMessage)
	}

	// 2. Verify the payload (raw markdown) appears in the rendered prompt.
	if !strings.Contains(userMessage, "# Architecture Review") {
		t.Errorf("expected raw markdown content in rendered prompt, got:\n%s", userMessage)
	}

	// 3. Verify the completed token carries the filename-derived Name.
	marking := h.Marking()
	for _, token := range marking.Tokens {
		if token.PlaceID == "task:complete" {
			if token.Color.Name != "architecture-review" {
				t.Errorf("expected Name 'architecture-review' on completed token, got %q", token.Color.Name)
			}
		}
	}
}

// spawningExecutor accepts work and spawns a child work item with its own Name.
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

	// Only spawn a child on the first call to avoid infinite recursion.
	if callNum == 1 {
		parentWorkID := ""
		if len(dispatch.InputTokens) > 0 {
			parentWorkID = firstInputToken(dispatch.InputTokens).Color.WorkID
		}
		result.SpawnedWork = []interfaces.TokenColor{
			{
				WorkTypeID: "task",
				WorkID:     fmt.Sprintf("%s-child", parentWorkID),
				Name:       "spawned-subtask",
				DataType:   interfaces.DataTypeWork,
				ParentID:   parentWorkID,
				Payload:    []byte(`child payload`),
			},
		}
	}

	return result, nil
}

func (s *spawningExecutor) getCalls() []interfaces.WorkDispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]interfaces.WorkDispatch(nil), s.calls...)
}

// TestNamePropagation_SpawnedChildWork verifies that spawned child work items
// can carry their own Name, independent of the parent's Name.
func TestNamePropagation_SpawnedChildWork(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "name_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "parent-task",
		WorkTypeID: "task",
		Payload:    []byte(`parent payload`),
		TraceID:    "trace-spawn-test",
	})

	// The spawning executor will handle the first dispatch and spawn a child.
	// A second capturing executor handles the child's dispatch.
	spawnExec := &spawningExecutor{}

	// We need a provider for the child's dispatch through the model worker.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Child done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("executor", spawnExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both parent and child should reach task:complete.
	marking := h.Marking()

	parentFound := false
	childFound := false
	for _, token := range marking.Tokens {
		if token.PlaceID == "task:complete" {
			if token.Color.Name == "parent-task" {
				parentFound = true
			}
		}
		// Child starts at task:init and may or may not have been processed yet.
		// The spawned token enters task:init with Name "spawned-subtask".
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

	// Verify the spawning executor received the parent Name.
	calls := spawnExec.getCalls()
	if len(calls) < 1 {
		t.Fatal("expected at least 1 dispatch to spawning executor")
	}
	if firstInputToken(calls[0].InputTokens).Color.Name != "parent-task" {
		t.Errorf("expected parent Name 'parent-task' in dispatch, got %q", firstInputToken(calls[0].InputTokens).Color.Name)
	}
}
