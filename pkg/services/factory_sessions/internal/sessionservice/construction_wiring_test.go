package service

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestInstallFactoryDefinitionsRejectsMissingDefinitions(t *testing.T) {
	t.Parallel()

	err := InstallFactoryDefinitions(&SessionRuntime{}, nil)
	if err == nil || err.Error() != "factory definitions service is required" {
		t.Fatalf("InstallFactoryDefinitions() error = %v, want %q", err, "factory definitions service is required")
	}
}

func TestInstallFactoryDefinitionsRejectsMissingRuntime(t *testing.T) {
	t.Parallel()

	err := InstallFactoryDefinitions(nil, factorydefinitions.Service(nil))
	if err == nil || err.Error() != "session runtime is required" {
		t.Fatalf("InstallFactoryDefinitions() error = %v, want %q", err, "session runtime is required")
	}
}

type rootInvocationProbe struct{}

func (rootInvocationProbe) InvokeFactorySession(
	context.Context,
	string,
	factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	return factorydefinitions.FactoryInvocationResult{
		RequestID: "request-root",
		Status:    factorydefinitions.InvocationTerminalStatusCompleted,
		SessionID: "session-root",
		WorkID:    "work-root",
	}, nil
}

func TestRootCapabilitiesStayOnTheSingularService(t *testing.T) {
	t.Parallel()

	var activated string
	root := &Service{}
	root.bindRootCapabilities(
		rootInvocationProbe{},
		func(_ context.Context, name string) error {
			activated = name
			return nil
		},
		nil,
	)

	invoked, err := root.InvokeFactorySession(context.Background(), "session-root", factorysessions.InvocationRequest{})
	if err != nil {
		t.Fatalf("InvokeFactorySession() error = %v", err)
	}
	if invoked.RequestID != "request-root" ||
		invoked.Status != factorysessions.InvocationTerminalStatusCompleted ||
		invoked.SessionID != "session-root" {
		t.Fatalf("InvokeFactorySession() = %#v, want root-owned projection", invoked)
	}
	if err := root.ActivateNamedFactory(context.Background(), "factory-root"); err != nil {
		t.Fatalf("ActivateNamedFactory() error = %v", err)
	}
	if activated != "factory-root" {
		t.Fatalf("activation name = %q, want factory-root", activated)
	}
}

func TestRootStartRejectsIncompleteAndUnsupportedRequests(t *testing.T) {
	t.Parallel()

	root := &Service{}
	_, err := root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeLive,
	})
	var validation *factorysessions.ValidationError
	if !errors.As(err, &validation) || validation.Field != "live" {
		t.Fatalf("missing live start = %v, want ValidationError field=live", err)
	}

	_, err = root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartMode("UNKNOWN"),
	})
	if !errors.As(err, &validation) || validation.Field != "mode" {
		t.Fatalf("unsupported start mode = %v, want ValidationError field=mode", err)
	}

	_, err = root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeDurableAsync,
		Live: &factorysessions.OpenRequest{FolderPath: "/tmp"},
	})
	if !errors.As(err, &validation) || validation.Field != "mode" {
		t.Fatalf("conflicting live/durable start = %v, want ValidationError field=mode", err)
	}

	_, err = root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeDurableAsync,
	})
	if !errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured) {
		t.Fatalf("unconfigured durable start = %v, want ErrExecutionServiceNotConfigured", err)
	}
}

type rootStartExecutionProbe struct {
	factorysessions.ExecutionService
	asyncResult factorysessions.AsyncStartResult
	syncResult  factorysessions.SyncStartResult
}

func (probe rootStartExecutionProbe) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	return probe.asyncResult, nil
}

func (probe rootStartExecutionProbe) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return probe.syncResult, nil
}

func TestRootStartRoutesDurableModesThroughOneService(t *testing.T) {
	t.Parallel()

	root := &Service{durable: rootStartExecutionProbe{
		asyncResult: factorysessions.AsyncStartResult{
			SessionID: "async-root",
			Status:    string(factorysessions.LifecycleStatusQueued),
		},
		syncResult: factorysessions.SyncStartResult{
			AsyncStartResult: factorysessions.AsyncStartResult{
				SessionID: "sync-root",
				Status:    string(factorysessions.LifecycleStatusSucceeded),
			},
			SyncOutcome: factorysessions.SyncOutcome("COMPLETED"),
		},
	}}

	asyncStarted, err := root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeDurableAsync,
	})
	if err != nil {
		t.Fatalf("durable async Start() error = %v", err)
	}
	if asyncStarted.SessionID != "async-root" || asyncStarted.Async == nil ||
		asyncStarted.Mode != factorysessions.StartModeDurableAsync {
		t.Fatalf("durable async Start() = %#v, want async root result", asyncStarted)
	}

	syncStarted, err := root.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeDurableSync,
	})
	if err != nil {
		t.Fatalf("durable sync Start() error = %v", err)
	}
	if syncStarted.SessionID != "sync-root" || syncStarted.Sync == nil ||
		syncStarted.Mode != factorysessions.StartModeDurableSync {
		t.Fatalf("durable sync Start() = %#v, want sync root result", syncStarted)
	}

	automaticStarted, err := root.Start(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("automatic async Start() error = %v", err)
	}
	if automaticStarted.Async == nil || automaticStarted.Mode != factorysessions.StartModeDurableAsync {
		t.Fatalf("automatic Start() = %#v, want async root result", automaticStarted)
	}

	automaticSync, err := root.Start(context.Background(), factorysessions.StartRequest{
		Wait: &factorysessions.WaitOptions{},
	})
	if err != nil {
		t.Fatalf("automatic sync Start() error = %v", err)
	}
	if automaticSync.Sync == nil || automaticSync.Mode != factorysessions.StartModeDurableSync {
		t.Fatalf("automatic wait Start() = %#v, want sync root result", automaticSync)
	}
}
