// Package session implements factory-session lifecycle command behavior.
package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// LifecycleControlConfig holds parameters for session pause and resume commands.
type LifecycleControlConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	RequestID   string
	Reason      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

// Service exposes Factory Session CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Show(ShowConfig) error
	Pause(LifecycleControlConfig) error
	Resume(LifecycleControlConfig) error
	ListDispatches(DispatchesConfig) error
	Create(CreateConfig) error
	Delete(DeleteConfig) error
}

// Operations carries the accepted per-command operations used to build Service.
type Operations struct {
	List           func(ListConfig) error
	Show           func(ShowConfig) error
	Pause          func(LifecycleControlConfig) error
	Resume         func(LifecycleControlConfig) error
	ListDispatches func(DispatchesConfig) error
	Create         func(CreateConfig) error
	Delete         func(DeleteConfig) error
}

type service struct {
	list           func(ListConfig) error
	show           func(ShowConfig) error
	pause          func(LifecycleControlConfig) error
	resume         func(LifecycleControlConfig) error
	listDispatches func(DispatchesConfig) error
	create         func(CreateConfig) error
	delete         func(DeleteConfig) error
}

// Bind constructs the typed Sessions CLI service from injected operations.
func Bind(ops Operations) Service {
	return &service{
		list:           ops.List,
		show:           ops.Show,
		pause:          ops.Pause,
		resume:         ops.Resume,
		listDispatches: ops.ListDispatches,
		create:         ops.Create,
		delete:         ops.Delete,
	}
}

func (service *service) List(cfg ListConfig) error {
	if service == nil || service.list == nil {
		return fmt.Errorf("session list service is required")
	}
	return service.list(cfg)
}

func (service *service) Show(cfg ShowConfig) error {
	if service == nil || service.show == nil {
		return fmt.Errorf("session show service is required")
	}
	return service.show(cfg)
}

func (service *service) Pause(cfg LifecycleControlConfig) error {
	if service == nil || service.pause == nil {
		return fmt.Errorf("session pause service is required")
	}
	return service.pause(cfg)
}

func (service *service) Resume(cfg LifecycleControlConfig) error {
	if service == nil || service.resume == nil {
		return fmt.Errorf("session resume service is required")
	}
	return service.resume(cfg)
}

func (service *service) ListDispatches(cfg DispatchesConfig) error {
	if service == nil || service.listDispatches == nil {
		return fmt.Errorf("session dispatches service is required")
	}
	return service.listDispatches(cfg)
}

func (service *service) Create(cfg CreateConfig) error {
	if service == nil || service.create == nil {
		return fmt.Errorf("session create service is required")
	}
	return service.create(cfg)
}

func (service *service) Delete(cfg DeleteConfig) error {
	if service == nil || service.delete == nil {
		return fmt.Errorf("session delete service is required")
	}
	return service.delete(cfg)
}

func NewPause(transport clihttp.Protocol) func(LifecycleControlConfig) error {
	return func(cfg LifecycleControlConfig) error { cfg.HTTP = transport; return Pause(cfg) }
}

func NewResume(transport clihttp.Protocol) func(LifecycleControlConfig) error {
	return func(cfg LifecycleControlConfig) error { cfg.HTTP = transport; return Resume(cfg) }
}

// LifecycleControlRejectedError reports a typed lifecycle-control rejection returned
// by the API with a FactorySessionLifecycleControlResponse body.
type LifecycleControlRejectedError struct {
	Response factoryapi.FactorySessionLifecycleControlResponse
}

func (e *LifecycleControlRejectedError) Error() string {
	if e == nil {
		return ""
	}
	detail := ""
	if e.Response.Detail != nil && strings.TrimSpace(*e.Response.Detail) != "" {
		detail = ": " + strings.TrimSpace(*e.Response.Detail)
	}
	return fmt.Sprintf(
		"factory session %s %s rejected (%s)%s",
		e.Response.SessionId,
		strings.ToLower(string(e.Response.Operation)),
		e.Response.Outcome,
		detail,
	)
}

// Pause requests pause for one factory session through POST /factory-sessions/{session_id}/pause.
func Pause(cfg LifecycleControlConfig) error {
	return invokeLifecycleControl(cfg, factoryapi.FactorySessionLifecycleControlKindPause, "pause")
}

// Resume requests resume for one factory session through POST /factory-sessions/{session_id}/resume.
func Resume(cfg LifecycleControlConfig) error {
	return invokeLifecycleControl(cfg, factoryapi.FactorySessionLifecycleControlKindResume, "resume")
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func invokeLifecycleControl(
	cfg LifecycleControlConfig,
	operation factoryapi.FactorySessionLifecycleControlKind,
	operationLabel string,
) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}

	endpoint, err := lifecycleControlEndpoint(cfg, operationLabel)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session %s request endpointPath=%s endpoint=%s server=%s session=%s",
		operationLabel,
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
	)

	body, err := lifecycleControlRequestBody(cfg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(cfg.Context, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("build factory session %s request: %w", operationLabel, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	transportResponse, err := cfg.HTTP.Execute(req)
	if err != nil {
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session %s response endpointPath=%s error=unreachable durationMillis=%d",
			operationLabel,
			endpoint.Path,
			transportResponse.Duration.Milliseconds(),
		)
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := transportResponse.HTTP
	if resp == nil {
		return fmt.Errorf("factory sessions endpoint returned no response")
	}
	defer resp.Body.Close()

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session %s response endpointPath=%s status=%d durationMillis=%d",
		operationLabel,
		endpoint.Path,
		resp.StatusCode,
		transportResponse.Duration.Milliseconds(),
	)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted:
		var response factoryapi.FactorySessionLifecycleControlResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return fmt.Errorf("parse factory session %s response: %w", operationLabel, err)
		}
		return renderLifecycleControlOutcome(cfg, response)
	case http.StatusConflict:
		var response factoryapi.FactorySessionLifecycleControlResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return lifecycleControlStatusError(operationLabel, resp)
		}
		if writeErr := writeLifecycleControlResponse(cfg, response); writeErr != nil {
			return writeErr
		}
		return &LifecycleControlRejectedError{Response: response}
	case http.StatusNotFound:
		return lifecycleControlNotFoundError(cfg.SessionID, resp)
	default:
		return lifecycleControlStatusError(operationLabel, resp)
	}
}

func lifecycleControlEndpoint(cfg LifecycleControlConfig, operationLabel string) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/"+operationLabel, cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session %s endpoint: %w", operationLabel, err)
	}
	return *endpoint, nil
}

func lifecycleControlRequestBody(cfg LifecycleControlConfig) (io.Reader, error) {
	requestID := strings.TrimSpace(cfg.RequestID)
	reason := strings.TrimSpace(cfg.Reason)
	if requestID == "" && reason == "" {
		return nil, nil
	}
	req := factoryapi.FactorySessionLifecycleControlRequest{}
	if requestID != "" {
		req.RequestId = &requestID
	}
	if reason != "" {
		req.Reason = &reason
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal lifecycle control request: %w", err)
	}
	return bytes.NewReader(encoded), nil
}

func renderLifecycleControlOutcome(
	cfg LifecycleControlConfig,
	response factoryapi.FactorySessionLifecycleControlResponse,
) error {
	if err := writeLifecycleControlResponse(cfg, response); err != nil {
		return err
	}
	switch response.Outcome {
	case factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
		factoryapi.FactorySessionLifecycleControlOutcomeNoOp:
		return nil
	default:
		return &LifecycleControlRejectedError{Response: response}
	}
}

func writeLifecycleControlResponse(
	cfg LifecycleControlConfig,
	response factoryapi.FactorySessionLifecycleControlResponse,
) error {
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	_, err := fmt.Fprintln(cfg.Output, lifecycleControlHumanLine(response))
	return err
}

func lifecycleControlNotFoundError(sessionID string, resp *http.Response) error {
	label := resolvedLifecycleControlSessionID(sessionID)
	if errResp, ok := decodeLifecycleControlAPIError(resp); ok {
		return fmt.Errorf("factory session %q not found: %s", label, errResp.Message)
	}
	return fmt.Errorf("factory session %q not found", label)
}

func lifecycleControlStatusError(operationLabel string, resp *http.Response) error {
	if errResp, ok := decodeLifecycleControlAPIError(resp); ok {
		return fmt.Errorf("factory session %s failed (%d): %s", operationLabel, resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("factory session %s failed (%d)", operationLabel, resp.StatusCode)
}

func decodeLifecycleControlAPIError(resp *http.Response) (factoryapi.ErrorResponse, bool) {
	if resp == nil || resp.Body == nil {
		return factoryapi.ErrorResponse{}, false
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return factoryapi.ErrorResponse{}, false
	}
	if errResp.Message == "" {
		return errResp, false
	}
	return errResp, true
}

func resolvedLifecycleControlSessionID(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return sessionpath.DefaultFactorySessionID
	}
	return id
}
