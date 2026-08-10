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

func (*applicationOpeningOwnerStub) RegisterInvocationEvents(factorysessions.InvocationEventScope) (factorysessions.OpeningScopeID, error) {
	return "", errors.New("not used")
}

func (*applicationOpeningOwnerStub) InvocationEvents(factorysessions.OpeningScopeID) (factorysessions.FactoryEventConsumer, bool) {
	return nil, false
}

func (*applicationOpeningOwnerStub) StartFactoryEventBridge(context.Context, factorysessions.Service, factorysessions.OpeningScopeID) (interface {
	Finish(context.Context, factorysessions.Service, factorysessions.FactoryInvocationOutcome) error
}, error) {
	return nil, errors.New("not used")
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

func TestOpenApplicationRejectsNilServiceAndStopsAtOpeningFailures(t *testing.T) {
	var nilService *Service
	if _, err := nilService.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{}); err == nil {
		t.Fatal("nil service OpenApplication error = nil")
	}

	resolveErr := errors.New("resolve failed")
	openErr := errors.New("open failed")
	for _, testCase := range []struct {
		name       string
		resolveErr error
		openErr    error
	}{
		{name: "resolve", resolveErr: resolveErr},
		{name: "open", openErr: openErr},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			opened := 0
			service, err := New(
				func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
					if testCase.resolveErr != nil {
						return RuntimeInputs{}, testCase.resolveErr
					}
					return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
				},
				runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
					opened++
					return roles.OpenedApplicationRuntime{}, testCase.openErr
				}),
				func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
					t.Fatal("application adapter ran after opening failure")
					return factorysessions.BoundProcessComponents{}, nil
				},
				func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
					t.Fatal("lifecycle planner ran after opening failure")
					return lifecycle.Plan{}, nil
				},
			)
			if err != nil {
				t.Fatalf("New(): %v", err)
			}
			_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
				Runtime: &factorysessions.RuntimeOpeningRequest{},
			})
			want := testCase.resolveErr
			if want == nil {
				want = testCase.openErr
			}
			if !errors.Is(err, want) || opened != boolToInt(testCase.resolveErr == nil) {
				t.Fatalf("OpenApplication() error = %v, opened = %d, want %v and %d", err, opened, want, boolToInt(testCase.resolveErr == nil))
			}
		})
	}
}

func TestOpenApplicationAllowsNilResolvedRequest(t *testing.T) {
	openedNilRequest := false
	service, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{}, nil
		},
		runtimeOpenerFunc(func(_ context.Context, request *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			openedNilRequest = request == nil
			return roles.OpenedApplicationRuntime{}, nil
		}),
		func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, nil },
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if _, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{},
	}); err != nil {
		t.Fatalf("OpenApplication(): %v", err)
	}
	if !openedNilRequest {
		t.Fatal("runtime opener received a non-nil request after resolver returned no request")
	}
}

func TestOpenApplicationBindsLivePresentationAndRejectsInvalidVisualization(t *testing.T) {
	sink := &markerSink{}
	owner := &applicationOpeningOwnerStub{}
	boundHTTP := false
	scopeID, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		VisualizationSink: sink,
		RuntimeHTTPServicesBound: func(got roles.RuntimeHTTPServices) {
			boundHTTP = got == (roles.RuntimeHTTPServices{})
		},
		Completion: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("RegisterApplication(): %v", err)
	}
	var adaptedSink factoryvisualization.Sink
	service, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
		},
		runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{HTTP: roles.RuntimeHTTPServices{}}, nil
		}),
		func(_ roles.OpenedApplicationRuntime, got factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			adaptedSink = got
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(request roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
			if request.Completion == nil {
				t.Fatal("live lifecycle plan omitted owner completion")
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
		t.Fatalf("OpenApplication() live: %v", err)
	}
	if adaptedSink != sink || !boundHTTP {
		t.Fatalf("live presentation = sink:%v http:%v, want injected values", adaptedSink == sink, boundHTTP)
	}

	closed := 0
	invalidOwner := &applicationOpeningOwnerStub{}
	invalidID, err := invalidOwner.RegisterApplication(factorysessions.ApplicationOpeningScope{VisualizationSink: struct{}{}})
	if err != nil {
		t.Fatalf("RegisterApplication(invalid): %v", err)
	}
	invalidService, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
		},
		runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{Close: func() error {
				closed++
				return errors.New("invalid visualization close")
			}}}, nil
		}),
		func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			t.Fatal("adapter ran for invalid visualization sink")
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, nil }, invalidOwner,
	)
	if err != nil {
		t.Fatalf("New(invalid): %v", err)
	}
	_, err = invalidService.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, ScopeID: invalidID,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid type") || closed != 1 {
		t.Fatalf("invalid visualization error = %v, close count = %d", err, closed)
	}
}

func TestOpenApplicationClosesHistoricalReplayWhenPlanningFails(t *testing.T) {
	planErr := errors.New("historical plan failed")
	closeErr := errors.New("historical close failed")
	closed := 0
	service, err := New(
		func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
		},
		runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{
				HistoricalReplay: &factorysessions.HistoricalReplayInspection{},
				Resources:        roles.RuntimeResources{Close: func() error { closed++; return closeErr }},
			}, nil
		}),
		func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
			t.Fatal("live adapter ran for historical replay")
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, planErr },
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{},
	})
	if !errors.Is(err, planErr) || !errors.Is(err, closeErr) || closed != 1 {
		t.Fatalf("historical error = %v, close count = %d", err, closed)
	}
}

func TestOpenApplicationRejectsScopeWithoutOwner(t *testing.T) {
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
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, ScopeID: "unregistered",
	})
	if err == nil || !strings.Contains(err.Error(), "presentation scope") {
		t.Fatalf("scope lookup error = %v, want unavailable owner scope", err)
	}
}

func TestCloseOpenedRuntimeWithoutClosePreservesCause(t *testing.T) {
	cause := errors.New("opening failed")
	if err := closeOpenedRuntime(roles.OpenedApplicationRuntime{}, cause); !errors.Is(err, cause) {
		t.Fatalf("closeOpenedRuntime() = %v, want original cause", err)
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
