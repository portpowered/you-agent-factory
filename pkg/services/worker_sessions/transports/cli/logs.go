package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func readLogs(config ReadConfig, jsonOutput bool) error {
	if config.Limit < 0 || config.Limit > 1000 {
		return emitReadCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_LOGS_INVALID", "--limit must be between 1 and 1000", nil))
	}
	endpoint, err := cliserver.RequestURL(config.Server, "/worker-sessions/"+url.PathEscape(config.WorkerSessionID)+"/logs")
	if err != nil {
		return emitReadCLIError(config, jsonOutput, err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, err)
	}
	query := parsed.Query()
	if config.Limit != 0 {
		query.Set("limit", strconv.Itoa(config.Limit))
	}
	if config.NextToken != "" {
		query.Set("nextToken", config.NextToken)
	}
	parsed.RawQuery = query.Encode()
	if config.ArtifactRef != "" {
		query.Set("artifactRef", config.ArtifactRef)
		parsed.RawQuery = query.Encode()
		return readLogsArtifact(config, parsed.String(), jsonOutput)
	}
	if config.Follow {
		return followLogs(config, *parsed)
	}
	return readLogsPage(config, parsed.String(), jsonOutput)
}

func readLogsPage(config ReadConfig, endpoint string, jsonOutput bool) error {
	var page factoryapi.WorkerSessionLogPage
	response, err := config.HTTP.GetJSON(config.Context, endpoint, &page)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, logsTransportError(config.Context, err))
	}
	if response.HTTP == nil {
		return emitReadCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_READ_FAILED", "Worker Session logs returned no HTTP response", nil))
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitReadCLIError(config, jsonOutput, workerSessionReadHTTPError(response.HTTP, response.HTTP.StatusCode))
	}
	if jsonOutput {
		return json.NewEncoder(config.Output).Encode(page)
	}
	if _, err := fmt.Fprintf(config.Output, "Worker Session %s: capture %s, committed position %d\n", page.WorkerSessionId, page.Health, page.CommittedPosition); err != nil {
		return err
	}
	for _, event := range page.Events {
		if err := json.NewEncoder(config.Output).Encode(event); err != nil {
			return err
		}
	}
	if page.NextToken != nil {
		_, err = fmt.Fprintf(config.Output, "Next token: %s\n", *page.NextToken)
	}
	return err
}

func readLogsArtifact(config ReadConfig, endpoint string, jsonOutput bool) error {
	req, err := http.NewRequestWithContext(config.Context, http.MethodGet, endpoint, nil)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, err)
	}
	response, err := config.HTTP.Execute(req)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, logsTransportError(config.Context, err))
	}
	if response.HTTP == nil {
		return emitReadCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_READ_FAILED", "Worker Session payload returned no HTTP response", nil))
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitReadCLIError(config, jsonOutput, workerSessionReadHTTPError(response.HTTP, response.HTTP.StatusCode))
	}
	_, err = io.Copy(config.Output, response.HTTP.Body)
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled)) {
		return logsTransportError(config.Context, err)
	}
	return err
}

func logsTransportError(ctx context.Context, err error) *CLIError {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_LOGS_INTERRUPTED", "worker session logs read interrupted", context.Canceled)
	}
	return newCLIError("FACTORY_UNREACHABLE", "factory not reachable for Worker Session logs", err)
}
