// Package session implements factory-session lifecycle command behavior.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListConfig holds parameters for the session list command.
type ListConfig struct {
	Context       context.Context
	Server        string
	Port          int
	Scope         string
	JSON          bool
	Verbose       bool
	Debug         bool
	Output        io.Writer
	Diagnostics   io.Writer
	DurableLister durableSessionLister
	DurableCloser io.Closer
	HTTP          clihttp.Protocol
	Preparation   RequestPreparation
}

type RequestPreparation interface {
	PrepareListSessions(fse.ListSessionsRequest) (fse.ListSessionsRequest, error)
}

func NewList(transport clihttp.Protocol, prepare RequestPreparation) func(ListConfig) error {
	return func(cfg ListConfig) error {
		cfg.HTTP = transport
		cfg.Preparation = prepare
		return List(cfg)
	}
}

// List requests factory sessions from a running host and, when scoped listing
// includes durable rows, from the deterministic fixture-backed provider.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func List(cfg ListConfig) (err error) {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.DurableCloser != nil {
		defer func() {
			if closeErr := cfg.DurableCloser.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close durable session listing: %w", closeErr))
			}
		}()
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}

	if cfg.Preparation == nil {
		return fmt.Errorf("Factory Session request preparation is required")
	}
	normalized, err := cfg.Preparation.PrepareListSessions(fse.ListSessionsRequest{
		Scope: fse.SessionListScope(strings.TrimSpace(cfg.Scope)),
	})
	if err != nil {
		return err
	}

	needsLive := normalized.Scope == fse.SessionListScopeLive || normalized.Scope == fse.SessionListScopeAll
	needsDurable := normalized.Scope == fse.SessionListScopePersisted || normalized.Scope == fse.SessionListScopeAll

	if needsDurable && !needsLive {
		scoped, err := mergeScopedListResult(cfg.Context, cfg, normalized, nil)
		if err != nil {
			return err
		}
		return emitListResult(cfg, listResponseFromScopedResult(scoped), len(scoped.LiveSessions), len(scoped.DurableSessions))
	}

	if !needsLive {
		return fmt.Errorf("unsupported session list scope %q", normalized.Scope)
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}

	liveSessions, httpResult, err := fetchLiveSessions(cfg)
	if err != nil {
		return err
	}
	if !needsDurable {
		if cfg.JSON {
			encoder := json.NewEncoder(cfg.Output)
			return encoder.Encode(httpResult)
		}
		return renderListResult(cfg.Output, httpResult)
	}

	scoped, err := mergeScopedListResult(cfg.Context, cfg, normalized, liveSessions)
	if err != nil {
		return err
	}
	return emitListResult(cfg, listResponseFromScopedResult(scoped), len(scoped.LiveSessions), len(scoped.DurableSessions))
}

func emitListResult(cfg ListConfig, result factoryapi.ListFactorySessionsResponse, liveCount, durableCount int) error {
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session list response scope=%s liveSessionCount=%d durableSessionCount=%d",
		scopeLabel(result.Scope),
		liveCount,
		durableCount,
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderListResult(cfg.Output, result)
}

func scopeLabel(scope *factoryapi.FactorySessionListScope) string {
	if scope == nil {
		return string(fse.DefaultSessionListScope)
	}
	return string(*scope)
}

func fetchLiveSessions(cfg ListConfig) ([]fse.LiveSessionSummary, factoryapi.ListFactorySessionsResponse, error) {
	endpoint, err := listEndpoint(cfg)
	if err != nil {
		return nil, factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("resolve factory sessions endpoint: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session list request endpointPath=%s endpoint=%s port=%d scope=live",
		endpoint.Path,
		endpoint.String(),
		cfg.Port,
	)

	var result factoryapi.ListFactorySessionsResponse
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&result,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session list response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return nil, factoryapi.ListFactorySessionsResponse{}, fmt.Errorf(
			"factory sessions endpoint not reachable at %s: %w",
			endpoint.String(),
			err,
		)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session list response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return nil, factoryapi.ListFactorySessionsResponse{}, fmt.Errorf(
				"list factory sessions failed (%d): %s",
				resp.StatusCode,
				errResp.Message,
			)
		}
		return nil, factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("list factory sessions failed (%d)", resp.StatusCode)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session list response endpointPath=%s status=%d durationMillis=%d sessionCount=%d",
		endpoint.Path,
		resp.StatusCode,
		response.Duration.Milliseconds(),
		len(result.Sessions),
	)

	liveSessions := make([]fse.LiveSessionSummary, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		liveSessions = append(liveSessions, fse.LiveSessionSummary{
			ID:         session.Id,
			FactoryDir: session.FactoryDir,
			FolderPath: session.FolderPath,
			Project:    session.Project,
			IsDefault:  session.IsDefault,
		})
	}
	return liveSessions, result, nil
}

func listEndpoint(cfg ListConfig) (url.URL, error) {
	if strings.TrimSpace(cfg.Server) == "" {
		return url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("localhost:%d", cfg.Port),
			Path:   "/factory-sessions",
		}, nil
	}

	endpointURL, err := cliserver.RequestURL(cfg.Server, "/factory-sessions")
	if err != nil {
		return url.URL{}, err
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session list endpoint: %w", err)
	}
	return *parsed, nil
}

func defaultMarker(isDefault bool) string {
	if isDefault {
		return "yes"
	}
	return "no"
}

func targetName(name *string) string {
	if name == nil {
		return ""
	}
	return strings.TrimSpace(*name)
}

func orchestratorKindLabel(session factoryapi.FactorySessionSummary) string {
	if session.Runtime == nil {
		return ""
	}
	return string(session.Runtime.OrchestratorKind)
}
