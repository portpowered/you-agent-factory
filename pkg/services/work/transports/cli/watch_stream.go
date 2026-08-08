package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type watchEventStream interface {
	Next(context.Context) (factoryapi.FactoryEvent, error)
	Close() error
}

type watchEventOpener interface {
	Open(context.Context) (watchEventStream, error)
}

type watchEventOpenFunc func(context.Context) (watchEventStream, error)

func (open watchEventOpenFunc) Open(ctx context.Context) (watchEventStream, error) {
	if open == nil {
		return nil, fmt.Errorf("work watch event opener is required")
	}
	return open(ctx)
}

// NewWatch binds the CLI HTTP protocol to the Work watch operation. The
// protocol is injected by Wire so the stream still uses the process-owned
// external-effect boundary.
func NewWatch(transport clihttp.Protocol) func(WatchConfig) error {
	return func(cfg WatchConfig) error {
		cfg.HTTP = transport
		return Watch(cfg)
	}
}

// Watch consumes the selected session's canonical Factory Event SSE stream.
// It does not query Work snapshots or schedule a polling interval.
func Watch(cfg WatchConfig) error {
	if err := ValidateWatchConfig(cfg); err != nil {
		return err
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	sessionID := watchSessionID(cfg)
	return watchWithSource(cfg, watchEventOpenFunc(func(ctx context.Context) (watchEventStream, error) {
		return openHTTPWatchEventStream(ctx, cfg.HTTP, cfg.Server, sessionID, cfg.Diagnostics, cfg.Verbose)
	}))
}

func watchSessionID(cfg WatchConfig) string {
	if strings.TrimSpace(cfg.SessionID) == "" {
		return sessionpath.DefaultFactorySessionID
	}
	return cfg.SessionID
}

func watchWithSource(cfg WatchConfig, opener watchEventOpener) error {
	if opener == nil {
		return fmt.Errorf("work watch event opener is required")
	}
	sessionID := watchSessionID(cfg)
	stream, err := opener.Open(cfg.Context)
	if err != nil {
		return fmt.Errorf("open work watch stream for session %q: %w", sessionID, err)
	}
	if stream == nil {
		return fmt.Errorf("open work watch stream for session %q: stream is unavailable", sessionID)
	}
	defer stream.Close()

	reducer := newWatchReducer(sessionID)
	for {
		event, err := stream.Next(cfg.Context)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if errors.Is(err, io.EOF) {
				if reducer.Completed() && !cfg.Follow {
					return nil
				}
				return fmt.Errorf("work watch stream for session %q closed before finite completion", sessionID)
			}
			return fmt.Errorf("read work watch stream for session %q: %w", sessionID, err)
		}
		transition, emit, completed, err := reducer.Accept(event)
		if err != nil {
			return err
		}
		if emit {
			if err := RenderWatchTransition(cfg.Output, transition); err != nil {
				return err
			}
		}
		if completed && !cfg.Follow {
			return nil
		}
	}
}

type httpWatchEventStream struct {
	reader    *bufio.Reader
	body      io.ReadCloser
	closeOnce sync.Once
}

func openHTTPWatchEventStream(
	ctx context.Context,
	transport clihttp.Protocol,
	server string,
	sessionID string,
	diagnostics io.Writer,
	verbose bool,
) (watchEventStream, error) {
	if transport == nil {
		return nil, fmt.Errorf("CLI HTTP protocol is required")
	}
	endpointURL, err := cliserver.RequestURL(server, sessionpath.FactoryEventsPath(sessionID))
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("parse work watch endpoint: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build work watch request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	clidiag.Printf(
		diagnostics,
		verbose,
		"work watch stream open endpointPath=%s endpoint=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		sessionID,
	)
	response, err := transport.Execute(request)
	if err != nil {
		return nil, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	if response.HTTP == nil {
		return nil, fmt.Errorf("work watch stream returned no HTTP response")
	}
	resp := response.HTTP
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return nil, fmt.Errorf("watch work failed for session %q (%d): %s", sessionID, resp.StatusCode, errResp.Message)
		}
		return nil, fmt.Errorf("watch work failed for session %q (%d)", sessionID, resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		defer resp.Body.Close()
		return nil, fmt.Errorf("watch work stream for session %q returned content type %q", sessionID, resp.Header.Get("Content-Type"))
	}
	return &httpWatchEventStream{reader: bufio.NewReader(resp.Body), body: resp.Body}, nil
}

func (stream *httpWatchEventStream) Next(ctx context.Context) (factoryapi.FactoryEvent, error) {
	if stream == nil || stream.reader == nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("work watch HTTP stream is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.FactoryEvent{}, err
	}
	event, err := readWatchSSEEvent(stream.reader)
	if err != nil && ctx.Err() != nil {
		return factoryapi.FactoryEvent{}, ctx.Err()
	}
	return event, err
}

func (stream *httpWatchEventStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	var err error
	stream.closeOnce.Do(func() { err = stream.body.Close() })
	return err
}

func readWatchSSEEvent(reader *bufio.Reader) (factoryapi.FactoryEvent, error) {
	var data []string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if err != nil && len(line) == 0 {
			if len(data) == 0 {
				return factoryapi.FactoryEvent{}, err
			}
		}
		if line == "" {
			if len(data) == 0 {
				if err != nil {
					return factoryapi.FactoryEvent{}, err
				}
				continue
			}
			return decodeWatchSSEEvent(data)
		}
		if strings.HasPrefix(line, ":") {
			if err != nil {
				return factoryapi.FactoryEvent{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}
		if err != nil {
			return decodeWatchSSEEvent(data)
		}
	}
}

func decodeWatchSSEEvent(data []string) (factoryapi.FactoryEvent, error) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("decode canonical Factory Event SSE data: %w", err)
	}
	return event, nil
}
