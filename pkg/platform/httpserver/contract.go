// Package httpserver defines the host-network effect used to serve an
// already-constructed HTTP handler.
package httpserver

import (
	"context"
	"net"
	"net/http"

	"go.uber.org/zap"
)

// Binding identifies the endpoint selected for one hosted HTTP runtime.
type Binding struct {
	Host string
	Port int
}

// BoundObserver receives the selected endpoint after binding succeeds and
// before the server begins accepting requests.
type BoundObserver func(Binding)

// StartRequest contains the exact host inputs for one runtime.
type StartRequest struct {
	Handler  http.Handler
	Host     string
	Port     int
	AutoPort bool
	Logger   *zap.Logger
	OnBound  BoundObserver
}

// Starter owns listener selection and serving until its context is cancelled.
type Starter func(context.Context, StartRequest) error

// ListenerFactory binds the exact TCP listener used by a Starter. Supplying
// one keeps listener allocation replaceable in owner-local tests. It is a
// required constructor dependency; only Wire may select net.Listen.
type ListenerFactory func(network, address string) (net.Listener, error)
