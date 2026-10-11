// Package cli adapts Agent Message commands to the public HTTP contract.
// Caller environment values are credentials only; the server owns authority.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/agentmessage"
)

// Config carries invocation-local inputs, including an explicit environment
// lookup. The adapter never discovers the host environment itself.
type Config struct {
	Context      context.Context
	Server       string
	SessionID    string
	OutputFormat string
	Output       io.Writer
	LookupEnv    func(string) (string, bool)
}

type Adapter struct {
	transport clihttp.Protocol
	streaming clihttp.Protocol
	newID     func() string
}

func NewAdapter(transport, streaming clihttp.Protocol, newID func() string) *Adapter {
	return &Adapter{transport: transport, streaming: streaming, newID: newID}
}

func (a *Adapter) request(config Config, method, path string, body io.Reader) (*http.Request, error) {
	if config.Context == nil || config.Output == nil || a.transport == nil ||
		(config.OutputFormat != "" && config.OutputFormat != "human" && config.OutputFormat != "json") {
		return nil, agentmessages.ErrBadRequest
	}
	server := config.Server
	if server == "" && config.LookupEnv != nil {
		server, _ = config.LookupEnv("YOU_SERVER")
	}
	endpoint, err := cliserver.RequestURL(server, path)
	if err != nil {
		// The URI may contain credentials; never attach parser diagnostics.
		return nil, agentmessages.ErrBadRequest
	}
	request, err := http.NewRequestWithContext(config.Context, method, endpoint, body)
	if err != nil {
		return nil, agentmessages.ErrBadRequest
	}
	if config.LookupEnv != nil {
		if id, present := config.LookupEnv("YOU_WORKER_SESSION_ID"); present {
			request.Header.Set("X-You-Worker-Session-Id", id)
		}
		if token, present := config.LookupEnv("YOU_WORKER_SESSION_TOKEN"); present {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if _, err := apisurface.WorkerSessionCallerFromHeaders(request.Header); err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	return request, nil
}

func (a *Adapter) execute(request *http.Request, status int, privateBody ...string) (*http.Response, error) {
	transport := a.transport
	if request.Header.Get("Accept") == "text/event-stream" {
		transport = a.streaming
		if transport == nil {
			return nil, agentmessages.ErrStreamUnavailable
		}
	}
	response, err := transport.Execute(request)
	if err != nil || response.HTTP == nil || response.HTTP.Body == nil {
		if response.HTTP != nil && response.HTTP.Body != nil {
			_ = response.HTTP.Body.Close()
		}
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		// Collaborator errors may quote the token, headers or submitted body.
		return nil, agentmessages.ErrStoreUnavailable
	}
	if response.HTTP.StatusCode == status {
		return response.HTTP, nil
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if decoded, ok := clihttp.DecodeAPIError(response.HTTP); ok {
		return nil, safeAPIError(response.HTTP.StatusCode, decoded, request, privateBody...)
	}
	return nil, agentmessages.ErrStoreUnavailable
}

func safeAPIError(status int, response factoryapi.ErrorResponse, request *http.Request, privateBody ...string) error {
	// Preserve typed ambiguity/quota details while removing credentials from
	// every response field, including nested diagnostic strings.
	encoded, err := json.Marshal(response)
	if err != nil {
		return agentmessages.ErrStoreUnavailable
	}
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if token != "" {
		encoded = []byte(strings.ReplaceAll(string(encoded), token, "[REDACTED]"))
	}
	if json.Unmarshal(encoded, &response) != nil {
		return agentmessages.ErrStoreUnavailable
	}
	for _, secret := range privateBody {
		if secret != "" {
			response.Message = strings.ReplaceAll(response.Message, secret, "[REDACTED]")
			response.Details = redactDiagnosticValue(response.Details, secret)
		}
	}
	return clihttp.NewAPIError(status, response, "", nil)
}

func redactDiagnosticValue(value any, secret string) any {
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, secret, "[REDACTED]")
	case map[string]any:
		for key, item := range typed {
			typed[key] = redactDiagnosticValue(item, secret)
		}
	case []any:
		for index, item := range typed {
			typed[index] = redactDiagnosticValue(item, secret)
		}
	}
	return value
}

func scopeQuery(config Config, query url.Values) {
	if config.SessionID != "" {
		query.Set("factorySessionId", config.SessionID)
	}
}

func decodeJSON(reader io.Reader, result any) error {
	decoder := json.NewDecoder(reader)
	if decoder.Decode(result) != nil || decoder.Decode(new(any)) != io.EOF {
		return agentmessages.ErrStoreUnavailable
	}
	return nil
}

// commandError gives local validation the same representation as HTTP
// validation, retaining safe sentinel identity and the central CLI renderer.
func (a *Adapter) commandError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var remote *clihttp.APIError
	if errors.As(err, &remote) {
		return err
	}
	status, response := (agentmessage.Mapper{}).Error(err)
	if status == http.StatusInternalServerError {
		return err
	}
	return clihttp.NewAPIError(status, response, "", err)
}
