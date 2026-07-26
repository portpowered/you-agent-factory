package applicationopening

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

type runtimeOpenerFunc func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
	runtimeopening.ExternalEffects,
	*zap.Logger,
) (roles.OpenedApplicationRuntime, error)

func (open runtimeOpenerFunc) OpenApplicationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects runtimeopening.ExternalEffects,
	logger *zap.Logger,
) (roles.OpenedApplicationRuntime, error) {
	return open(ctx, request, effects, logger)
}

func TestNewRequiresEveryInjectedOperation(t *testing.T) {
	t.Parallel()

	resolve := RuntimeInputResolver(func(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
		roles.ApplicationOpeningPorts,
		*zap.Logger,
	) (RuntimeInputs, error) {
		return RuntimeInputs{}, nil
	})
	open := runtimeOpenerFunc(func(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
		runtimeopening.ExternalEffects,
		*zap.Logger,
	) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{}, nil
	})
	adapt := RuntimeAdapter(func(
		roles.OpenedApplicationRuntime,
		runtimeopening.ExternalEffects,
		factoryvisualization.Sink,
	) (factorysessions.BoundProcessComponents, error) {
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
			t.Parallel()
			if err := construct(); err == nil {
				t.Fatalf("New() error = nil, want missing %s operation", name)
			}
		})
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestOpenApplicationResolvesThenOpensAndBindsExactInputs(t *testing.T) {
	t.Parallel()

	request := &factorysessions.RuntimeOpeningRequest{}
	resolvedRequest := &factorysessions.RuntimeOpeningRequest{}
	observer := factorysessions.RuntimeHostObserver(func(factorysessions.RuntimeHostBinding) {})
	invocationPorts := roles.ApplicationOpeningPorts{RuntimeHostObserver: observer}
	resolvedEffects := runtimeopening.ExternalEffects{}
	logger := zap.NewNop()
	var order []string
	service, err := New(
		func(
			ctx context.Context,
			gotRequest *factorysessions.RuntimeOpeningRequest,
			gotPorts roles.ApplicationOpeningPorts,
			gotLogger *zap.Logger,
		) (RuntimeInputs, error) {
			order = append(order, "resolve")
			if ctx == nil || gotRequest != request || gotLogger != logger {
				t.Fatal("resolver received different invocation inputs")
			}
			if gotPorts.RuntimeHostObserver == nil {
				t.Fatal("resolver did not receive invocation-local ports")
			}
			return RuntimeInputs{Request: resolvedRequest, Effects: resolvedEffects, Logger: logger}, nil
		},
		runtimeOpenerFunc(func(
			_ context.Context,
			gotRequest *factorysessions.RuntimeOpeningRequest,
			_ runtimeopening.ExternalEffects,
			gotLogger *zap.Logger,
		) (roles.OpenedApplicationRuntime, error) {
			order = append(order, "open")
			if gotRequest != resolvedRequest || gotLogger != logger {
				t.Fatal("runtime opener did not receive resolved inputs")
			}
			return roles.OpenedApplicationRuntime{}, nil
		}),
		func(
			roles.OpenedApplicationRuntime,
			runtimeopening.ExternalEffects,
			factoryvisualization.Sink,
		) (factorysessions.BoundProcessComponents, error) {
			order = append(order, "bind")
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(got roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
			order = append(order, "plan")
			return lifecycle.Plan{Resources: []lifecycle.NamedResource{{Name: "factory runtime"}}}, nil
		},
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	opened, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: request, Ports: invocationPorts,
	}, logger, nil)
	if err != nil {
		t.Fatalf("OpenApplication(): %v", err)
	}
	if len(opened.Plan.Resources) != 1 || opened.Plan.Resources[0].Name != "factory runtime" {
		t.Fatalf("opened plan resources = %#v", opened.Plan.Resources)
	}
	if len(order) != 4 || order[0] != "resolve" || order[1] != "open" || order[2] != "bind" || order[3] != "plan" {
		t.Fatalf("operation order = %v, want [resolve open bind plan]", order)
	}
}

func TestOpenApplicationClosesOpenedResourcesWhenBindingFails(t *testing.T) {
	t.Parallel()

	bindErr := errors.New("bind failed")
	closeErr := errors.New("close failed")
	closed := 0
	service, err := New(
		func(
			context.Context,
			*factorysessions.RuntimeOpeningRequest,
			roles.ApplicationOpeningPorts,
			*zap.Logger,
		) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}, Logger: zap.NewNop()}, nil
		},
		runtimeOpenerFunc(func(
			context.Context,
			*factorysessions.RuntimeOpeningRequest,
			runtimeopening.ExternalEffects,
			*zap.Logger,
		) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{Close: func() error {
				closed++
				return closeErr
			}}}, nil
		}),
		func(
			roles.OpenedApplicationRuntime,
			runtimeopening.ExternalEffects,
			factoryvisualization.Sink,
		) (factorysessions.BoundProcessComponents, error) {
			return factorysessions.BoundProcessComponents{}, bindErr
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, nil },
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	_, err = service.OpenApplication(
		context.Background(), roles.ApplicationOpeningRequest{Runtime: &factorysessions.RuntimeOpeningRequest{}}, zap.NewNop(), nil,
	)
	if !errors.Is(err, bindErr) || !errors.Is(err, closeErr) {
		t.Fatalf("OpenApplication() error = %v, want binding and cleanup causes", err)
	}
	if closed != 1 {
		t.Fatalf("resource close count = %d, want 1", closed)
	}
}

func TestOpenApplicationClosesOpenedResourcesExactlyOnceWhenLifecyclePlanningFails(t *testing.T) {
	t.Parallel()

	planErr := errors.New("plan failed")
	closeErr := errors.New("close failed")
	closed := 0
	service, err := New(
		func(
			context.Context,
			*factorysessions.RuntimeOpeningRequest,
			roles.ApplicationOpeningPorts,
			*zap.Logger,
		) (RuntimeInputs, error) {
			return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}, Logger: zap.NewNop()}, nil
		},
		runtimeOpenerFunc(func(
			context.Context,
			*factorysessions.RuntimeOpeningRequest,
			runtimeopening.ExternalEffects,
			*zap.Logger,
		) (roles.OpenedApplicationRuntime, error) {
			return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{Close: func() error {
				closed++
				return closeErr
			}}}, nil
		}),
		func(
			roles.OpenedApplicationRuntime,
			runtimeopening.ExternalEffects,
			factoryvisualization.Sink,
		) (factorysessions.BoundProcessComponents, error) {
			return factorysessions.BoundProcessComponents{}, nil
		},
		func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
			return lifecycle.Plan{}, planErr
		},
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	_, err = service.OpenApplication(
		context.Background(), roles.ApplicationOpeningRequest{Runtime: &factorysessions.RuntimeOpeningRequest{}}, zap.NewNop(), nil,
	)
	if !errors.Is(err, planErr) || !errors.Is(err, closeErr) {
		t.Fatalf("OpenApplication() error = %v, want planning and cleanup causes", err)
	}
	if closed != 1 {
		t.Fatalf("resource close count = %d, want 1", closed)
	}
}

func TestOpenApplicationStopsAtResolveAndOpenFailures(t *testing.T) {
	t.Parallel()

	resolveErr := errors.New("resolve failed")
	openErr := errors.New("open failed")
	tests := []struct {
		name        string
		resolveErr  error
		openErr     error
		wantContext string
		wantOpens   int
	}{
		{name: "resolve", resolveErr: resolveErr, wantContext: "open Factory Session application", wantOpens: 0},
		{name: "open", openErr: openErr, wantContext: "open Factory Session application runtime", wantOpens: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			opened, adapted := 0, 0
			service, err := New(
				func(
					context.Context,
					*factorysessions.RuntimeOpeningRequest,
					roles.ApplicationOpeningPorts,
					*zap.Logger,
				) (RuntimeInputs, error) {
					if test.resolveErr != nil {
						return RuntimeInputs{}, test.resolveErr
					}
					return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}, Logger: zap.NewNop()}, nil
				},
				runtimeOpenerFunc(func(
					context.Context,
					*factorysessions.RuntimeOpeningRequest,
					runtimeopening.ExternalEffects,
					*zap.Logger,
				) (roles.OpenedApplicationRuntime, error) {
					opened++
					return roles.OpenedApplicationRuntime{}, test.openErr
				}),
				func(
					roles.OpenedApplicationRuntime,
					runtimeopening.ExternalEffects,
					factoryvisualization.Sink,
				) (factorysessions.BoundProcessComponents, error) {
					adapted++
					return factorysessions.BoundProcessComponents{}, nil
				},
				func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) { return lifecycle.Plan{}, nil },
			)
			if err != nil {
				t.Fatalf("New(): %v", err)
			}

			_, err = service.OpenApplication(
				context.Background(), roles.ApplicationOpeningRequest{Runtime: &factorysessions.RuntimeOpeningRequest{}}, zap.NewNop(), nil,
			)
			wantErr := test.resolveErr
			if wantErr == nil {
				wantErr = test.openErr
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.wantContext) {
				t.Fatalf("OpenApplication() error = %v, want %q wrapping %v", err, test.wantContext, wantErr)
			}
			if opened != test.wantOpens || adapted != 0 {
				t.Fatalf("downstream calls = open %d adapt %d, want %d and 0", opened, adapted, test.wantOpens)
			}
		})
	}
}
