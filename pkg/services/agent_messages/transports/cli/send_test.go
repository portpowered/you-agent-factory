package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSendOptionRefusalsBeforeHTTP(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*SendConfig)
	}{
		{name: "missing address", mutate: func(config *SendConfig) { config.WorkerSessionID = "" }},
		{name: "delivery spelling", mutate: func(config *SendConfig) { config.IfEnded = "unknown" }},
		{name: "short expiry", mutate: func(config *SendConfig) { value := 59 * time.Second; config.Expires = &value }},
		{name: "long expiry", mutate: func(config *SendConfig) { value := 7*24*time.Hour + time.Second; config.Expires = &value }},
		{name: "fractional expiry", mutate: func(config *SendConfig) { value := time.Minute + time.Millisecond; config.Expires = &value }},
		{name: "output format", mutate: func(config *SendConfig) { config.OutputFormat = "unknown" }},
		{name: "userinfo", mutate: func(config *SendConfig) { config.Server = "http://private:secret@selected.test" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewAdapter(protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				t.Fatal("invalid command reached HTTP")
				return clihttp.Response{}, nil
			}}, nil, nil)
			config := SendConfig{Config: workerConfig(io.Discard), WorkerSessionID: "lead", RequestID: "retry", BodyArgs: []string{"safe"}}
			test.mutate(&config)
			err := adapter.Send(config)
			var typed *clihttp.APIError
			if !errors.Is(err, agentmessages.ErrBadRequest) || !errors.As(err, &typed) || typed.Response.Code != factoryapi.ErrorResponseCodeMESSAGEINVALIDREQUEST {
				t.Fatal("local validation lost the shared CLI/HTTP error representation")
			}
		})
	}
}

func TestSendPreservesServerDefaultsAndExplicitRetry(t *testing.T) {
	t.Parallel()
	adapter := NewAdapter(nil, nil, func() string { panic("explicit retry must not allocate an ID") })
	request, err := adapter.sendRequest(SendConfig{WorkerSessionID: "list", RequestID: "same-request"}, "safe")
	if err != nil || request.RequestID != "same-request" || request.ExpiresInSeconds != nil || request.IfEnded != "" || request.ReplyIfEnded != "" || request.Delivery != "" {
		t.Fatal("omitted options overrode server configuration or request identity")
	}
	if request.To == nil || request.To.WorkerSessionID != "list" {
		t.Fatal("explicit reserved-word Worker Session address was changed")
	}
}

func TestDeclaredBodyOmittedFromRemoteDiagnostics(t *testing.T) {
	t.Parallel()
	secret := "private\n\"quoted\" body"
	response := factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCodeMESSAGEREQUESTCONFLICT, Family: factoryapi.ErrorFamilyConflict,
		Message: "refused " + secret, Details: map[string]any{"items": []any{secret}, "dimension": "request"}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter(protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
		return clihttp.Response{HTTP: &http.Response{StatusCode: 409, Body: io.NopCloser(strings.NewReader(string(encoded)))}}, nil
	}}, nil, nil)
	err = adapter.Send(SendConfig{Config: workerConfig(io.Discard), WorkerSessionID: "lead", RequestID: "same-request", Body: &secret, BodySecret: true})
	var typed *clihttp.APIError
	if !errors.As(err, &typed) || typed.Response.Code != response.Code || strings.Contains(typed.Response.Message, secret) {
		t.Fatal("declared body escaped diagnostics or typed code changed")
	}
	details, _ := json.Marshal(typed.Response.Details)
	if strings.Contains(string(details), "private") || !strings.Contains(string(details), "request") {
		t.Fatal("nested secret disclosure or nonsecret detail loss")
	}
}
