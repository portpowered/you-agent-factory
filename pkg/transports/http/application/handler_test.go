package application

import (
	"errors"
	"net/http"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

type validationRole struct {
	factorydefinitions.SubmittedDefinitionValidationOperation
}

type invocationWorkTypeRole struct {
	factorydefinitions.InvocationWorkTypeService
}

type requestPreparationRole struct {
	factorysessionshttp.RequestPreparation
}

type durableExecutionRole struct {
	factorysessions.Service
}

type testHandler struct{}

func (*testHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func newApplicationHandler(t *testing.T, binding RuntimeBinding) *Handler {
	t.Helper()
	handler, err := NewHandler(
		binding,
		&validationRole{},
		&invocationWorkTypeRole{},
		&requestPreparationRole{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func TestHandlerDelegatesLiveBindingToWireOwnerComposition(t *testing.T) {
	t.Parallel()

	called := false
	want := &testHandler{}
	handler := newApplicationHandler(t, func(Binding) (http.Handler, error) {
		called = true
		return want, nil
	})

	got, err := handler.Bind(Binding{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got != want || !called {
		t.Fatalf("Bind = (%T, called=%t), want the Wire-provided handler and one invocation", got, called)
	}
}

func TestHandlerPropagatesWireBindingError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("owner adapter failed")
	handler := newApplicationHandler(t, func(Binding) (http.Handler, error) {
		return nil, wantErr
	})
	if got, err := handler.Bind(Binding{}); !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("Bind = (%T, %v), want (%T, %v)", got, err, got, wantErr)
	}
}

func TestNewHandlerRequiresWireBindingAndDurablePolicies(t *testing.T) {
	t.Parallel()

	valid := func(Binding) (http.Handler, error) { return http.NotFoundHandler(), nil }
	cases := map[string]func() (*Handler, error){
		"runtime binding": func() (*Handler, error) {
			return NewHandler(nil, &validationRole{}, &invocationWorkTypeRole{}, &requestPreparationRole{})
		},
		"validation": func() (*Handler, error) {
			return NewHandler(valid, nil, &invocationWorkTypeRole{}, &requestPreparationRole{})
		},
		"invocation work type": func() (*Handler, error) {
			return NewHandler(valid, &validationRole{}, nil, &requestPreparationRole{})
		},
		"request preparation": func() (*Handler, error) {
			return NewHandler(valid, &validationRole{}, &invocationWorkTypeRole{}, nil)
		},
	}
	for name, construct := range cases {
		t.Run(name, func(t *testing.T) {
			if handler, err := construct(); err == nil || handler != nil {
				t.Fatalf("NewHandler = (%T, %v), want a construction error", handler, err)
			}
		})
	}
}

func TestHandlerBindsStandaloneDurableExecution(t *testing.T) {
	t.Parallel()

	handler := newApplicationHandler(t, func(Binding) (http.Handler, error) {
		return http.NotFoundHandler(), nil
	})
	bound, err := handler.BindDurableExecution(&durableExecutionRole{}, nil)
	if err != nil || bound == nil {
		t.Fatalf("BindDurableExecution = (%T, %v), want a handler", bound, err)
	}
}

func TestHandlerRejectsIncompleteDurableExecutionBinding(t *testing.T) {
	t.Parallel()

	handler := newApplicationHandler(t, func(Binding) (http.Handler, error) {
		return http.NotFoundHandler(), nil
	})
	for name, bind := range map[string]func() (http.Handler, error){
		"handler": func() (http.Handler, error) {
			return (*Handler)(nil).BindDurableExecution(&durableExecutionRole{}, nil)
		},
		"execution": func() (http.Handler, error) {
			return handler.BindDurableExecution(nil, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if bound, err := bind(); err == nil || bound != nil {
				t.Fatalf("BindDurableExecution = (%T, %v), want a construction error", bound, err)
			}
		})
	}
}

var _ factorysession.DurableExecution = (*durableExecutionRole)(nil)
