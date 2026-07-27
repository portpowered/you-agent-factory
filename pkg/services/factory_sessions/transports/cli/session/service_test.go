package session

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNewRequiresHTTPAndPreparationDependencies(t *testing.T) {
	t.Parallel()

	if service := New(nil, canonicalListRequestPreparation); service != nil {
		t.Fatalf("New(nil, preparation) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(protocol, nil) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), canonicalListRequestPreparation); service == nil {
		t.Fatal("New(protocol, preparation) = nil, want Sessions CLI service")
	}
}

func TestConstructedService_RequiresCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	service := New(testHTTPProtocol(t), canonicalListRequestPreparation)
	if service == nil {
		t.Fatal("New() = nil, want Sessions CLI service")
	}

	tests := map[string]func() error{
		"create": func() error { return service.Create(CreateConfig{Dir: "."}) },
		"delete": func() error { return service.Delete(DeleteConfig{SessionID: "session-1"}) },
		"dispatches": func() error {
			return service.ListDispatches(DispatchesConfig{Context: context.Background()})
		},
		"pause": func() error {
			return service.Pause(LifecycleControlConfig{Context: context.Background()})
		},
		"resume": func() error {
			return service.Resume(LifecycleControlConfig{Context: context.Background()})
		},
		"list": func() error {
			return service.List(ListConfig{Context: context.Background()})
		},
		"show": func() error { return service.Show(ShowConfig{Context: context.Background()}) },
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}

func TestConstructedService_PauseMatchesPackageCommandOutcome(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/session-beta/pause"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	service := New(testHTTPProtocol(t), canonicalListRequestPreparation)
	if service == nil {
		t.Fatal("New() = nil, want Sessions CLI service")
	}

	var out bytes.Buffer
	if err := service.Pause(LifecycleControlConfig{
		Context:   context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if !strings.Contains(out.String(), "session-beta") {
		t.Fatalf("pause output = %q, want session id", out.String())
	}
}

func TestConstructedService_ListJSONMatchesPackageCommandOutcome(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{Id: "session-alpha", FactoryDir: "/factory"},
			},
		})
	}))
	defer srv.Close()

	service := New(testHTTPProtocol(t), canonicalListRequestPreparation)
	if service == nil {
		t.Fatal("New() = nil, want Sessions CLI service")
	}

	var serviceOut bytes.Buffer
	if err := service.List(ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
		JSON:    true,
		Output:  &serviceOut,
	}); err != nil {
		t.Fatalf("service.List() error = %v", err)
	}

	var commandOut bytes.Buffer
	if err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
		JSON:    true,
		Output:  &commandOut,
	}); err != nil {
		t.Fatalf("NewList() error = %v", err)
	}

	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service list JSON = %q, command list JSON = %q", serviceOut.String(), commandOut.String())
	}
}

func TestConstructedService_ShowRequiresContext(t *testing.T) {
	t.Parallel()

	service := New(testHTTPProtocol(t), canonicalListRequestPreparation)
	if service == nil {
		t.Fatal("New() = nil, want Sessions CLI service")
	}

	err := service.Show(ShowConfig{Output: &bytes.Buffer{}})
	if err == nil || err.Error() != "context is required" {
		t.Fatalf("error = %v, want context is required", err)
	}
}
