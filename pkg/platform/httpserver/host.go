package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

const defaultLoopbackHost = "localhost"

// BindError reports that no requested loopback endpoint could be bound.
type BindError struct {
	Host          string
	PreferredPort int
	Cause         error
}

func (err *BindError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(
		"bind HTTP listener on %s from port %d through 65535: %v",
		err.Host,
		err.PreferredPort,
		err.Cause,
	)
}

func (err *BindError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// IsBindError reports whether err contains a terminal listener-binding failure.
func IsBindError(err error) bool {
	var bindErr *BindError
	return errors.As(err, &bindErr)
}

// NewStarter constructs an inert HTTP host operation. Listener binding and
// concrete server lifecycle occur only when the returned operation is called.
func NewStarter(listen ListenerFactory) (Starter, error) {
	if listen == nil {
		return nil, errors.New("construct HTTP starter: listener factory is required")
	}
	return func(ctx context.Context, request StartRequest) error {
		listener, host, port, err := bind(listen, request.Host, request.Port, request.AutoPort)
		if err != nil {
			return err
		}
		if request.OnBound != nil {
			request.OnBound(Binding{Host: host, Port: port})
		}
		return Serve(ctx, request.Handler, listener, request.Logger)
	}, nil
}

func bind(
	listen ListenerFactory,
	host string,
	preferredPort int,
	autoPort bool,
) (net.Listener, string, int, error) {
	if host == "" {
		host = defaultLoopbackHost
	}
	if !isLoopbackHost(host) {
		cause := fmt.Errorf("host %q is not loopback", host)
		return nil, "", 0, &BindError{Host: host, PreferredPort: preferredPort, Cause: cause}
	}
	if preferredPort <= 0 || preferredPort > 65535 {
		cause := fmt.Errorf("port %d is outside 1..65535", preferredPort)
		return nil, "", 0, &BindError{Host: host, PreferredPort: preferredPort, Cause: cause}
	}
	var firstErr error
	for candidate := preferredPort; candidate <= 65535; candidate++ {
		listener, err := listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", candidate)))
		if err == nil {
			return listener, host, candidate, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !autoPort {
			break
		}
	}
	return nil, "", 0, &BindError{Host: host, PreferredPort: preferredPort, Cause: firstErr}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// Serve owns one already-bound listener and the concrete net/http server until
// cancellation or a terminal serve failure. Cancellation closes the server
// and joins its serve loop before returning.
func Serve(
	ctx context.Context,
	handler http.Handler,
	listener net.Listener,
	logger *zap.Logger,
) error {
	if ctx == nil {
		return errors.New("serve HTTP: context is required")
	}
	if handler == nil {
		return errors.New("serve HTTP: handler is required")
	}
	if listener == nil {
		return errors.New("serve HTTP: listener is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	server := &http.Server{Handler: handler}
	exit := make(chan error, 1)
	go func() {
		logger.Info("API server starting", zap.String("addr", listener.Addr().String()))
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		exit <- err
	}()

	select {
	case err := <-exit:
		return err
	case <-ctx.Done():
		closeErr := server.Close()
		serveErr := <-exit
		return errors.Join(closeErr, serveErr)
	}
}
