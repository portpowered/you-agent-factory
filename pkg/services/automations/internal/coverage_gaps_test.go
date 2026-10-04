package internal_test

import (
	"context"
	"sync"
	"testing"
)

func TestWorkflowIdentityForFactoryDir_UsesConfiguredWorkflowID(t *testing.T) {
	svc := newAutomationService(automationFixture{
		WorkflowID:        "workflow-override",
		DefaultFactoryDir: "/default/factory",
	})
	if got := svc.WorkflowIdentityForFactoryDir("/runtime/factory"); got != "workflow-override" {
		t.Fatalf("workflow identity = %q, want workflow-override", got)
	}
}

func TestWorkflowIdentityForFactoryDir_FallsBackToFactoryDirAndDefault(t *testing.T) {
	svc := newAutomationService(automationFixture{DefaultFactoryDir: "/default/factory"})
	if got := svc.WorkflowIdentityForFactoryDir("/runtime/factory"); got != "/runtime/factory" {
		t.Fatalf("workflow identity = %q, want /runtime/factory", got)
	}
	if got := svc.WorkflowIdentityForFactoryDir(""); got != "/default/factory" {
		t.Fatalf("empty factory dir identity = %q, want /default/factory", got)
	}
}

func TestStartSchedulerSidecarsForRuntime_NoOpsOnNilInput(t *testing.T) {
	svc := newAutomationService(automationFixture{})
	var sidecars sync.WaitGroup
	svc.StartSchedulerSidecarsForRuntime(context.Background(), &sidecars, "", nil, nil, nil)
}
