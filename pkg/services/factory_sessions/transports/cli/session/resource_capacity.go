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
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ResourceCapacityConfig holds parameters for the session resource set command.
type ResourceCapacityConfig struct {
	Context          context.Context
	Server           string
	SessionID        string
	ResourceID       string
	Capacity         int
	ExpectedRevision int
	RequestID        string
	Reason           string
	JSON             bool
	Verbose          bool
	Debug            bool
	Output           io.Writer
	Diagnostics      io.Writer
	HTTP             clihttp.Protocol
}

// ResourceCapacityRejectedError reports a typed API rejection from resource
// capacity admission while preserving the public error code and message.
type ResourceCapacityRejectedError struct {
	StatusCode int
	Response   factoryapi.ErrorResponse
}

// CLIErrorCode preserves the server's typed capacity rejection code through
// the root CLI process boundary.
func (e *ResourceCapacityRejectedError) CLIErrorCode() string {
	if e == nil || e.Response.Code == "" {
		return "RESOURCE_CAPACITY_REQUEST_FAILED"
	}
	return string(e.Response.Code)
}

// CLIErrorMessage returns the safe, operation-scoped rejection diagnostic.
func (e *ResourceCapacityRejectedError) CLIErrorMessage() string {
	if e == nil {
		return "resource capacity request rejected"
	}
	return e.Error()
}

func (e *ResourceCapacityRejectedError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Response.Message)
	if message == "" {
		message = "resource capacity request rejected"
	}
	if e.Response.Code != "" {
		return fmt.Sprintf("resource capacity request failed (%d, %s): %s", e.StatusCode, e.Response.Code, message)
	}
	return fmt.Sprintf("resource capacity request failed (%d): %s", e.StatusCode, message)
}

// NewSetResourceCapacity constructs the injected CLI operation.
func NewSetResourceCapacity(transport clihttp.Protocol) func(ResourceCapacityConfig) error {
	return func(cfg ResourceCapacityConfig) error { cfg.HTTP = transport; return SetResourceCapacity(cfg) }
}

// SetResourceCapacity applies one live resource-capacity change through the
// session-scoped REST operation.
func SetResourceCapacity(cfg ResourceCapacityConfig) error {
	normalized, err := normalizeResourceCapacityConfig(cfg)
	if err != nil {
		return err
	}
	cfg = normalized
	resourceID := cfg.ResourceID
	body, err := json.Marshal(factoryapi.FactorySessionResourceCapacityRequest{
		Capacity:         cfg.Capacity,
		ExpectedRevision: cfg.ExpectedRevision,
		RequestId:        cfg.RequestID,
		Reason:           optionalString(cfg.Reason),
	})
	if err != nil {
		return fmt.Errorf("marshal resource capacity request: %w", err)
	}
	endpoint, err := resourceCapacityEndpoint(cfg.Server, cfg.SessionID, resourceID)
	if err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose,
		"session resource capacity request endpointPath=%s endpoint=%s session=%s resourceId=%s capacity=%d expectedRevision=%d requestId=%s",
		endpoint.Path, endpoint.String(), clidiag.SessionLabel(cfg.SessionID), resourceID, cfg.Capacity, cfg.ExpectedRevision, cfg.RequestID)
	req, err := http.NewRequestWithContext(cfg.Context, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build resource capacity request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-You-Source", "cli")
	transportResponse, err := cfg.HTTP.Execute(req)
	if err != nil {
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	if transportResponse.HTTP == nil {
		return fmt.Errorf("factory sessions endpoint returned no response")
	}
	resp := transportResponse.HTTP
	defer resp.Body.Close()
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose,
		"session resource capacity response endpointPath=%s status=%d durationMillis=%d",
		endpoint.Path, resp.StatusCode, transportResponse.Duration.Milliseconds())
	if resp.StatusCode != http.StatusOK {
		response, ok := clihttp.DecodeAPIError(resp)
		if !ok {
			response = factoryapi.ErrorResponse{Message: "resource capacity request rejected"}
		}
		return &ResourceCapacityRejectedError{StatusCode: resp.StatusCode, Response: response}
	}
	var result factoryapi.FactorySessionResourceCapacityResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse resource capacity response: %w", err)
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return renderResourceCapacityHuman(cfg.Output, result)
}

func normalizeResourceCapacityConfig(cfg ResourceCapacityConfig) (ResourceCapacityConfig, error) {
	if cfg.Context == nil {
		return ResourceCapacityConfig{}, fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return ResourceCapacityConfig{}, fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return ResourceCapacityConfig{}, fmt.Errorf("CLI HTTP protocol is required")
	}
	cfg.ResourceID = strings.TrimSpace(cfg.ResourceID)
	if cfg.ResourceID == "" {
		return ResourceCapacityConfig{}, fmt.Errorf("resource id is required")
	}
	if cfg.Capacity < 0 {
		return ResourceCapacityConfig{}, fmt.Errorf("capacity must be a non-negative integer")
	}
	cfg.RequestID = strings.TrimSpace(cfg.RequestID)
	if cfg.RequestID == "" {
		cfg.RequestID = fmt.Sprintf("cli-resource-capacity-%d", time.Now().UTC().UnixNano())
	}
	cfg.Reason = strings.TrimSpace(cfg.Reason)
	return cfg, nil
}

func resourceCapacityEndpoint(server, sessionID, resourceID string) (url.URL, error) {
	path := sessionpath.ScopedPath("/resources/"+url.PathEscape(resourceID)+"/capacity", sessionID)
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return url.URL{}, fmt.Errorf("resolve resource capacity endpoint: %w", err)
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse resource capacity endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderResourceCapacityHuman(output io.Writer, response factoryapi.FactorySessionResourceCapacityResponse) error {
	rows := []struct {
		label string
		value string
	}{
		{"Resource ID", response.ResourceId},
		{"Previous capacity", fmt.Sprintf("%d", response.PreviousCapacity)},
		{"Requested capacity", fmt.Sprintf("%d", response.RequestedCapacity)},
		{"Effective capacity", fmt.Sprintf("%d", response.EffectiveCapacity)},
		{"In-use count", fmt.Sprintf("%d", response.InUseCount)},
		{"Available count", fmt.Sprintf("%d", response.AvailableCount)},
		{"Minimum capacity", fmt.Sprintf("%d", response.MinimumCapacity)},
		{"Outcome", string(response.Outcome)},
		{"Revision", fmt.Sprintf("%d", response.Revision)},
		{"Request ID", response.RequestId},
		{"Change ID", response.ChangeId},
		{"Session", response.SessionId},
	}
	if response.ResourceName != nil && strings.TrimSpace(*response.ResourceName) != "" {
		rows = append(rows, struct {
			label string
			value string
		}{"Resource label", strings.TrimSpace(*response.ResourceName)})
	}
	if response.Links != nil {
		if response.Links.Session != nil {
			rows = append(rows, struct {
				label string
				value string
			}{"Session link", *response.Links.Session})
		}
		if response.Links.Events != nil {
			rows = append(rows, struct {
				label string
				value string
			}{"Events link", *response.Links.Events})
		}
		if response.Links.Status != nil {
			rows = append(rows, struct {
				label string
				value string
			}{"Status link", *response.Links.Status})
		}
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
