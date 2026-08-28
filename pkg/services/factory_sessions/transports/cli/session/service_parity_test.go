package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func constructedService(t *testing.T) Service {
	t.Helper()
	service := New(testHTTPProtocol(t), canonicalListRequestPreparation)
	if service == nil {
		t.Fatal("New() = nil, want Sessions CLI service")
	}
	return service
}

func assertConstructedServiceParity(
	t *testing.T,
	runService func(io.Writer) error,
	runCommand func(io.Writer) error,
) {
	t.Helper()

	var serviceOut, commandOut bytes.Buffer
	serviceErr := runService(&serviceOut)
	commandErr := runCommand(&commandOut)

	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceErr != nil && commandErr != nil {
		var serviceRejected, commandRejected *LifecycleControlRejectedError
		if errors.As(serviceErr, &serviceRejected) || errors.As(commandErr, &commandRejected) {
			if !errors.As(serviceErr, &serviceRejected) || !errors.As(commandErr, &commandRejected) {
				t.Fatalf("typed rejection mismatch: service = %v, command = %v", serviceErr, commandErr)
			}
			if serviceRejected.Response.Outcome != commandRejected.Response.Outcome {
				t.Fatalf("rejection outcome = %q, want %q", serviceRejected.Response.Outcome, commandRejected.Response.Outcome)
			}
		} else if serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
}

func TestConstructedService_ShowHumanAndJSONMatchPackageCommands(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(sessionShowTestHandler(t, nil))
	t.Cleanup(func() { srv.Close() })

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	baseCfg := ShowConfig{
		Context:   context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
	}
	for name, jsonMode := range map[string]bool{"human": false, "json": true} {
		name, jsonMode := name, jsonMode
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := baseCfg
			cfg.JSON = jsonMode
			assertConstructedServiceParity(t,
				func(output io.Writer) error {
					cfg.Output = output
					return service.Show(cfg)
				},
				func(output io.Writer) error {
					cfg.Output = output
					return NewShow(protocol)(cfg)
				},
			)
		})
	}
}

func TestConstructedService_ListHumanMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		})
	}))
	defer srv.Close()

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	cfg := ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
	}
	assertConstructedServiceParity(t,
		func(output io.Writer) error {
			cfg.Output = output
			return service.List(cfg)
		},
		func(output io.Writer) error {
			cfg.Output = output
			return NewList(protocol, canonicalListRequestPreparation)(cfg)
		},
	)
}

func TestConstructedService_ResumeJSONMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	cfg := LifecycleControlConfig{
		Context:   context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
	}
	assertConstructedServiceParity(t,
		func(output io.Writer) error {
			cfg.Output = output
			return service.Resume(cfg)
		},
		func(output io.Writer) error {
			cfg.Output = output
			return NewResume(protocol)(cfg)
		},
	)
}

func TestConstructedService_CreateHumanAndJSONMatchPackageCommands(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPtr("beta"),
				},
			},
		})
	}))
	t.Cleanup(func() { srv.Close() })

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	baseCfg := CreateConfig{
		Port: serverPort(t, srv),
		Dir:  "/workspace/fleet",
	}
	for name, jsonMode := range map[string]bool{"human": false, "json": true} {
		name, jsonMode := name, jsonMode
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := baseCfg
			cfg.JSON = jsonMode
			assertConstructedServiceParity(t,
				func(output io.Writer) error {
					cfg.Output = output
					return service.Create(cfg)
				},
				func(output io.Writer) error {
					cfg.Output = output
					return NewCreate(protocol)(cfg)
				},
			)
		})
	}
}

func TestConstructedService_DeleteHumanAndJSONMatchPackageCommands(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(func() { srv.Close() })

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	baseCfg := DeleteConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
	}
	for name, jsonMode := range map[string]bool{"human": false, "json": true} {
		name, jsonMode := name, jsonMode
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := baseCfg
			cfg.JSON = jsonMode
			assertConstructedServiceParity(t,
				func(output io.Writer) error {
					cfg.Output = output
					return service.Delete(cfg)
				},
				func(output io.Writer) error {
					cfg.Output = output
					return NewDelete(protocol)(cfg)
				},
			)
		})
	}
}

func TestConstructedService_DeleteNotFoundMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "factory session not found",
			Code:    "NOT_FOUND",
		})
	}))
	defer srv.Close()

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	cfg := DeleteConfig{
		Server:    srv.URL,
		SessionID: "missing-session",
		Output:    ioDiscardWriter{t},
	}
	assertConstructedServiceParity(t,
		func(output io.Writer) error {
			cfg.Output = output
			return service.Delete(cfg)
		},
		func(output io.Writer) error {
			cfg.Output = output
			return NewDelete(protocol)(cfg)
		},
	)
}

func TestConstructedService_PauseTerminalRejectionMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-petri-success-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		})
	}))
	defer srv.Close()

	service := constructedService(t)
	protocol := testHTTPProtocol(t)
	cfg := LifecycleControlConfig{
		Context:   context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
	}
	assertConstructedServiceParity(t,
		func(output io.Writer) error {
			cfg.Output = output
			return service.Pause(cfg)
		},
		func(output io.Writer) error {
			cfg.Output = output
			return NewPause(protocol)(cfg)
		},
	)
}

func TestConstructedService_ListVerboseDiagnosticsMatchPackageCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		})
	}))
	defer srv.Close()

	protocol := testHTTPProtocol(t)
	cfg := ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
		JSON:    true,
		Verbose: true,
	}
	var serviceDiag, commandDiag bytes.Buffer
	var serviceOut, commandOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	serviceCfg.Diagnostics = &serviceDiag
	if err := constructedService(t).List(serviceCfg); err != nil {
		t.Fatalf("service.List() error = %v", err)
	}

	commandCfg := cfg
	commandCfg.Output = &commandOut
	commandCfg.Diagnostics = &commandDiag
	if err := NewList(protocol, canonicalListRequestPreparation)(commandCfg); err != nil {
		t.Fatalf("NewList() error = %v", err)
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service JSON = %q, command JSON = %q", serviceOut.String(), commandOut.String())
	}
	for _, want := range []string{
		"session list request",
		"endpointPath=/factory-sessions",
		"session list response",
		"status=200",
	} {
		if !strings.Contains(serviceDiag.String(), want) {
			t.Fatalf("service diagnostics missing %q:\n%s", want, serviceDiag.String())
		}
		if !strings.Contains(commandDiag.String(), want) {
			t.Fatalf("command diagnostics missing %q:\n%s", want, commandDiag.String())
		}
	}
}

func TestConstructedService_ShowHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	blocking := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocking
	}))
	defer srv.Close()
	defer close(blocking)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := constructedService(t)
	err := service.Show(ShowConfig{
		Context:   ctx,
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &bytes.Buffer{},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Show() error = %v, want context.Canceled", err)
	}

	var commandOut bytes.Buffer
	commandErr := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context:   ctx,
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &commandOut,
	})
	if !errors.Is(commandErr, context.Canceled) {
		t.Fatalf("NewShow() error = %v, want context.Canceled", commandErr)
	}
	if commandOut.Len() != 0 {
		t.Fatalf("canceled command output = %q, want empty", commandOut.String())
	}
}
