package restclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

// Adapter delegates functional REST calls to the generated response client.
// It owns no process, service, or endpoint-discovery state.
type Adapter struct {
	client *generatedclient.ClientWithResponses
}

// New constructs an adapter from dependencies owned by the caller.
func New(baseURL string, httpClient *http.Client) (*Adapter, error) {
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, fmt.Errorf("create generated REST adapter: HTTP client is required")
	}

	client, err := generatedclient.NewClientWithResponses(
		baseURL,
		generatedclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("create generated REST client: %w", err)
	}

	return &Adapter{client: client}, nil
}

// GetFactoryResponseEventsBySessionID executes the generated typed REST
// operation without translating its request parameters or response.
func (a *Adapter) GetFactoryResponseEventsBySessionID(
	ctx context.Context,
	sessionID generatedclient.SessionID,
	params *generatedclient.GetFactoryResponseEventsBySessionIdParams,
) (*generatedclient.GetFactoryResponseEventsBySessionIdClientResponse, error) {
	return a.client.GetFactoryResponseEventsBySessionIdWithResponse(ctx, sessionID, params)
}

func validateBaseURL(baseURL string) error {
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return fmt.Errorf("create generated REST adapter: invalid base URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("create generated REST adapter: base URL must be an absolute HTTP(S) URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("create generated REST adapter: base URL must not contain a query or fragment")
	}
	return nil
}
