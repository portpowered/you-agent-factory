package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"go.uber.org/zap"
)

const maxAutoPortAttempts = 100

// NewStarter constructs an inert HTTP host operation. Listener binding and
// concrete server lifecycle occur only when the returned operation is called.
func NewStarter(listen ListenerFactory) (Starter, error) {
	if listen == nil {
		return nil, errors.New("construct HTTP starter: listener factory is required")
	}
	return func(ctx context.Context, request StartRequest) error {
		listener, port, err := bind(listen, request.Port, request.AutoPort)
		if err != nil {
			return err
		}
		if request.OnBound != nil {
			request.OnBound(Binding{Port: port})
		}
		return Serve(ctx, request.Handler, listener, request.Logger)
	}, nil
}

func bind(listen ListenerFactory, preferredPort int, autoPort bool) (net.Listener, int, error) {
	if preferredPort <= 0 || preferredPort > 65535 {
		return nil, 0, fmt.Errorf("bind HTTP listener: port %d is outside 1..65535", preferredPort)
	}
	attempts := 1
	if autoPort {
		attempts = maxAutoPortAttempts
	}
	var firstErr error
	for candidate, attempt := preferredPort, 0; candidate <= 65535 && attempt < attempts; candidate, attempt = candidate+1, attempt+1 {
		listener, err := listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return listener, candidate, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, 0, fmt.Errorf("resolve open HTTP listener port from %d: %w", preferredPort, firstErr)
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
