package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const maxBodyBytes = 8192

type SendConfig struct {
	Config
	WorkerSessionID string
	InReplyTo       string
	RequestID       string
	Body            *string
	BodyArgs        []string
	Stdin           io.Reader
	StdinIsTTY      bool
	BodySecret      bool
	IfEnded         string
	ReplyHold       bool
	Interrupt       bool
	Correlation     agentmessages.Correlation
	Expires         *time.Duration
}

// Send handles both new messages and replies. Defaults that depend on server
// configuration remain omitted; only the server decides policy and sender.
func (a *Adapter) Send(config SendConfig) (resultErr error) {
	defer func() { resultErr = a.commandError(resultErr) }()
	request, err := a.request(config.Config, http.MethodPost, "/messages", nil)
	if err != nil {
		return err
	}
	caller, err := apisurface.WorkerSessionCallerFromHeaders(request.Header)
	if err != nil {
		return err
	}
	if caller == nil {
		return agentmessages.ErrNotPermitted
	}
	body, err := a.sendBody(config)
	if err != nil {
		return err
	}
	payload, err := a.sendRequest(config, body)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return agentmessages.ErrBadRequest
	}
	request.Body = io.NopCloser(bytes.NewReader(encoded))
	request.ContentLength = int64(len(encoded))
	request.Header.Set("Content-Type", "application/json")
	query := request.URL.Query()
	scopeQuery(config.Config, query)
	request.URL.RawQuery = query.Encode()
	privateBody := ""
	if config.BodySecret {
		privateBody = body
	}
	response, err := a.execute(request, http.StatusAccepted, privateBody)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	var result factoryapi.AgentMessageSendResponse
	if err := decodeJSON(response.Body, &result); err != nil {
		return err
	}
	if config.OutputFormat == "json" {
		return json.NewEncoder(config.Output).Encode(result)
	}
	return a.writeMessage(config.Output, result.Message)
}

func (a *Adapter) sendBody(config SendConfig) (string, error) {
	var body string
	switch {
	case config.Body != nil:
		if len(config.BodyArgs) != 0 {
			return "", agentmessages.ErrBadRequest
		}
		body = *config.Body
	case len(config.BodyArgs) != 0:
		body = strings.Join(config.BodyArgs, " ")
	case config.Stdin != nil && !config.StdinIsTTY:
		encoded, err := io.ReadAll(io.LimitReader(config.Stdin, maxBodyBytes+1))
		if err != nil {
			return "", agentmessages.ErrBadRequest
		}
		body = string(encoded)
	default:
		return "", agentmessages.ErrBadRequest
	}
	if len(body) > maxBodyBytes {
		return "", agentmessages.ErrLimitExceeded
	}
	if !utf8.ValidString(body) || strings.TrimSpace(body) == "" {
		return "", agentmessages.ErrBadRequest
	}
	return body, nil
}

func (a *Adapter) sendRequest(config SendConfig, body string) (agentmessages.SendRequest, error) {
	if (config.WorkerSessionID == "" && config.InReplyTo == "") ||
		(config.IfEnded != "" && config.IfEnded != "revive" && config.IfEnded != "hold") {
		return agentmessages.SendRequest{}, agentmessages.ErrBadRequest
	}
	request := agentmessages.SendRequest{
		RequestID: config.RequestID, Body: body, BodySecret: config.BodySecret,
		InReplyTo: config.InReplyTo, Correlation: config.Correlation,
		IfEnded: strings.ToUpper(config.IfEnded),
	}
	if request.RequestID == "" {
		if a.newID == nil {
			return request, agentmessages.ErrBadRequest
		}
		request.RequestID = a.newID()
	}
	if config.WorkerSessionID != "" {
		request.To = &agentmessages.Address{WorkerSessionID: config.WorkerSessionID}
	}
	return a.sendOptions(config, request)
}

func (a *Adapter) sendOptions(config SendConfig, request agentmessages.SendRequest) (agentmessages.SendRequest, error) {
	if config.ReplyHold {
		request.ReplyIfEnded = "HOLD"
	}
	if config.Interrupt {
		request.Delivery = "INTERRUPT"
	}
	if config.Expires != nil {
		if *config.Expires < time.Minute || *config.Expires > 7*24*time.Hour || *config.Expires%time.Second != 0 {
			return request, agentmessages.ErrBadRequest
		}
		seconds := int(*config.Expires / time.Second)
		request.ExpiresInSeconds = &seconds
	}
	if config.LookupEnv != nil {
		if request.Correlation.WorkID == "" {
			request.Correlation.WorkID, _ = config.LookupEnv("YOU_WORK_ID")
		}
		if request.Correlation.FactorySessionID == "" {
			request.Correlation.FactorySessionID, _ = config.LookupEnv("YOU_FACTORY_SESSION_ID")
		}
	}
	return request, nil
}
