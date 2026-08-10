package applicationopening

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

type runtimeOpenerFunc func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
) (roles.OpenedApplicationRuntime, error)

func (open runtimeOpenerFunc) OpenApplicationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	return open(ctx, request)
}

type applicationOpeningOwnerStub struct {
	scope factorysessions.ApplicationOpeningScope
	id    factorysessions.OpeningScopeID
}

type markerSink struct{}

func (markerSink) PresentFactoryView(factoryvisualization.View) {}

func (owner *applicationOpeningOwnerStub) RegisterApplication(scope factorysessions.ApplicationOpeningScope) (factorysessions.OpeningScopeID, error) {
	owner.scope, owner.id = scope, "application-test"
	return owner.id, nil
}

func (owner *applicationOpeningOwnerStub) Application(id factorysessions.OpeningScopeID) (factorysessions.ApplicationOpeningScope, bool) {
	return owner.scope, id == owner.id
}

func (*applicationOpeningOwnerStub) RegisterDirectJavaScript(factorysessions.DirectJavaScriptRunScope) (factorysessions.OpeningScopeID, error) {
	return "", errors.New("not used")
}

func (*applicationOpeningOwnerStub) DirectJavaScript(factorysessions.OpeningScopeID) (factorysessions.DirectJavaScriptRunScope, bool) {
	return factorysessions.DirectJavaScriptRunScope{}, false
}

func (*applicationOpeningOwnerStub) RegisterStdio(factorysessions.StdioOpeningScope) (factorysessions.OpeningScopeID, error) {
	return "", errors.New("not used")
}

func (*applicationOpeningOwnerStub) Stdio(factorysessions.OpeningScopeID) (factorysessions.StdioOpeningScope, bool) {
	return factorysessions.StdioOpeningScope{}, false
}

func (owner *applicationOpeningOwnerStub) ObserveHost(id factorysessions.OpeningScopeID, binding factorysessions.RuntimeHostBinding) {
	if id == owner.id && owner.scope.RuntimeHostObserver != nil {
		owner.scope.RuntimeHostObserver(binding)
	}
}

func (*applicationOpeningOwnerStub) Close(factorysessions.OpeningScopeID) {}

func TestNewRequiresEveryInjectedOperation(t *testing.T) {
	resolve := RuntimeInputResolver(func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
		return RuntimeInputs{}, nil
	})
	open := runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{}, nil
	})
	adapt := RuntimeAdapter(func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := roles.LifecyclePlanOperation(func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, nil
	})

	for name, construct := range map[string]func() error{
		"resolver":  func() error { _, err := New(nil, open, adapt, plan); return err },
		"opener":    func() error { _, err := New(resolve, nil, adapt, plan); return err },
		"adapter":   func() error { _, err := New(resolve, open, nil, plan); return err },
		"lifecycle": func() error { _, err := New(resolve, open, adapt, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatalf("New() error = nil, want missing %s operation", name)
			}
		})
	}
}

func TestOpenApplicationUsesOwnerScopeAndValueOnlyRuntimeRequest(t *testing.T) {
	request := &factorysessions.RuntimeOpeningRequest{}
	resolvedRequest := &factorysessions.RuntimeOpeningRequest{}
	var order []string
	var boundSink factoryvisualization.Sink
	var completionCalls atomic.Int32
	sink := &markerSink{}
	owner := &applicationOpeningOwnerStub{}
	scopeID, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		VisualizationSink: sink,
		Completion: func(context.Context) error {
			completionCalls.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
	service, err := New(
		func(ctx context.Context, gotRequest *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			order = append(order, "resolve")
			if ctx == nil || gotRequest != request {
				t.Fatal("resolver received different invocation inputs")
			}
			return RuntimeInputs{Request: resolvedRequest}, nil
		},
		runtimeOpenerFunc(func(_ context.Context, gotRequest *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			order = append(order, "open")
			if gotRequest != resolvedRequest || gotRequest.ScopeID != scopeID {
				t.Fatalf("runtime opener request = %#v, want resolved request with scope %q", gotRequest, scopeID)
			}
			return roles.OpenedApplicationRuntime{}, nil
		}),
		func(_ roles.OpenedApplicationRuntime, gotSink factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			order = append(order, "bind")
			boundSink = gotSink
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(got roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
			order = append(order, "plan")
			if got.Completion == nil {
				t.Fatal("lifecycle plan did not receive owner completion")
			}
			return lifecycle.Plan{}, nil
		}, owner,
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	opened, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: request, ScopeID: scopeID,
	})
	if err != nil {
		t.Fatalf("OpenApplication(): %v", err)
	}
	if opened.Plan.Components != nil && len(opened.Plan.Components) != 0 {
		t.Fatalf("opened plan components = %#v", opened.Plan.Components)
	}
	if boundSink != sink || len(order) != 4 || strings.Join(order, ",") != "resolve,open,bind,plan" {
		t.Fatalf("order/sink = %v/%v", order, boundSink == sink)
	}
	if resolvedRequest.ScopeID != scopeID {
		t.Fatalf("resolved request scope = %q, want %q", resolvedRequest.ScopeID, scopeID)
	}
}

func TestOpenApplicationRejectsMissingOwnerScope(t *testing.T) {
	service, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
		},
		runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{}, nil
		}),
		func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, nil },
		&applicationOpeningOwnerStub{},
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, ScopeID: "missing",
	})
	if err == nil || !strings.Contains(err.Error(), "presentation scope") {
		t.Fatalf("OpenApplication() error = %v, want missing owner scope", err)
	}
}

func TestOpenApplicationClosesOpenedResourcesOnBindingOrPlanningFailure(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		bindError bool
	}{
		{name: "binding", bindError: true},
		{name: "planning"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			bindErr := errors.New("bind failed")
			planErr := errors.New("plan failed")
			closeErr := errors.New("close failed")
			closed := 0
			service, err := New(
				func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
					return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
				},
				runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
					return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{Close: func() error {
						closed++
						return closeErr
					}}}, nil
				}),
				func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
					if testCase.bindError {
						return factorysessions.BoundProcessComponents{}, bindErr
					}
					return factorysessions.BoundProcessComponents{}, nil
				},
				func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
					if testCase.bindError {
						return lifecycle.Plan{}, nil
					}
					return lifecycle.Plan{}, planErr
				},
			)
			if err != nil {
				t.Fatalf("New(): %v", err)
			}
			_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
				Runtime: &factorysessions.RuntimeOpeningRequest{},
			})
			want := planErr
			if testCase.bindError {
				want = bindErr
			}
			if !errors.Is(err, want) || !errors.Is(err, closeErr) || closed != 1 {
				t.Fatalf("OpenApplication() error = %v, closed = %d", err, closed)
			}
		})
	}
}

func TestOpenApplicationPublishesHistoricalReplayThroughOwner(t *testing.T) {
	inspection := factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{SessionID: "recorded-session"},
	}
	var published factorysessions.HistoricalReplayInspection
	owner := &applicationOpeningOwnerStub{}
	scopeID, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		HistoricalReplayBound: func(got factorysessions.HistoricalReplayInspection) { published = got },
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
	service, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
		},
		runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{HistoricalReplay: &inspection}, nil
		}),
		func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			t.Fatal("historical replay unexpectedly bound live runtime")
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(request roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
			if request.Components.Transport == nil {
				t.Fatal("historical replay lifecycle is missing no-op transport")
			}
			return lifecycle.Plan{}, nil
		}, owner,
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if _, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, ScopeID: scopeID,
	}); err != nil {
		t.Fatalf("OpenApplication(): %v", err)
	}
	if published.Session.SessionID != "recorded-session" {
		t.Fatalf("published inspection = %#v", published)
	}
}
