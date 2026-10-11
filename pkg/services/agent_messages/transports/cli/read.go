package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type GetConfig struct {
	Config
	MessageID string
}

type ListConfig struct {
	Config
	ToMe                bool
	ToWorkerSessionID   string
	FromWorkerSessionID string
	ThreadID            string
	Correlation         agentmessages.Correlation
	Statuses            []string
	MaxResults          *int
	NextToken           string
	MarkRead            bool
	Follow              bool
}

func (a *Adapter) Get(config GetConfig) (resultErr error) {
	defer func() { resultErr = a.commandError(resultErr) }()
	if config.MessageID == "" {
		return agentmessages.ErrBadRequest
	}
	request, err := a.request(config.Config, http.MethodGet, "/messages/"+url.PathEscape(config.MessageID), nil)
	if err != nil {
		return err
	}
	response, err := a.execute(request, http.StatusOK)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	var result factoryapi.AgentMessage
	if err := decodeJSON(response.Body, &result); err != nil {
		return err
	}
	if config.OutputFormat == "json" {
		return json.NewEncoder(config.Output).Encode(result)
	}
	return a.writeMessage(config.Output, result)
}

func (a *Adapter) List(config ListConfig) (resultErr error) {
	defer func() { resultErr = a.commandError(resultErr) }()
	request, err := a.request(config.Config, http.MethodGet, "/messages", nil)
	if err != nil {
		return err
	}
	query := request.URL.Query()
	scopeQuery(config.Config, query)
	a.listQuery(config, query)
	request.URL.RawQuery = query.Encode()
	if config.Follow {
		request.Header.Set("Accept", "text/event-stream")
	}
	response, err := a.execute(request, http.StatusOK)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if config.Follow {
		return a.follow(config.Config, response, request)
	}
	var result factoryapi.AgentMessageListResponse
	if err := decodeJSON(response.Body, &result); err != nil {
		return err
	}
	if result.Messages == nil {
		result.Messages = []factoryapi.AgentMessage{}
	}
	if config.OutputFormat == "json" {
		return json.NewEncoder(config.Output).Encode(result)
	}
	for _, message := range result.Messages {
		if err := a.writeMessage(config.Output, message); err != nil {
			return err
		}
	}
	if result.NextToken != nil {
		_, err = fmt.Fprintf(config.Output, "Next token: %s\n", *result.NextToken)
	}
	return err
}

func (a *Adapter) listQuery(config ListConfig, query url.Values) {
	for name, value := range map[string]string{
		"toWorkerSessionId": config.ToWorkerSessionID, "fromWorkerSessionId": config.FromWorkerSessionID,
		"threadId": config.ThreadID, "correlationWorkId": config.Correlation.WorkID,
		"correlationFactorySessionId": config.Correlation.FactorySessionID, "nextToken": config.NextToken,
	} {
		if value != "" {
			query.Set(name, value)
		}
	}
	for name, value := range map[string]bool{"toMe": config.ToMe, "markRead": config.MarkRead, "follow": config.Follow} {
		if value {
			query.Set(name, "true")
		}
	}
	for _, status := range config.Statuses {
		query.Add("status", status)
	}
	if config.MaxResults != nil {
		query.Set("maxResults", strconv.Itoa(*config.MaxResults))
	}
}

func (a *Adapter) writeMessage(output io.Writer, message factoryapi.AgentMessage) error {
	_, err := fmt.Fprintf(output, "%s %s %s -> %s\n%s\n", message.MessageId, message.Status,
		message.From.WorkerSessionId, message.To.WorkerSessionId, message.Body)
	return err
}
