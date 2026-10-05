package cli

import (
	"encoding/json"
	"fmt"
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
	var page factoryapi.WorkerSessionLogPage
	response, err := config.HTTP.GetJSON(config.Context, parsed.String(), &page)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, newCLIError("FACTORY_UNREACHABLE", "factory not reachable for Worker Session logs", err))
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
