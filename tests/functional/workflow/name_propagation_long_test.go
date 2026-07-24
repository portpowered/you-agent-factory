//go:build functionallong

package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamePropagation_InPromptTemplate(t *testing.T) {
	support.SkipLongFunctional(t, "slow prompt-template name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "design-doc-review",
		WorkTypeID: "task",
		Payload:    []byte(`review the design document`),
		TraceID:    "trace-prompt-test",
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{"task:complete": 1})

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
		workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{"task:complete": 1, "task:init": 0})

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

	assertCompletedWorkName(t, listedWork, "task", "architecture-review")
}

func assertCompletedWorkName(t *testing.T, response factoryapi.ListWorkResponse, workType, wantName string) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName != nil && *item.WorkTypeName == workType && item.State != nil && item.State.Name == "complete" {
			if item.Name != wantName {
				t.Errorf("%s:complete name = %q, want %q", workType, item.Name, wantName)
			}
			return
		}
	}
	t.Errorf("listed Work missing %s:complete", workType)
}
