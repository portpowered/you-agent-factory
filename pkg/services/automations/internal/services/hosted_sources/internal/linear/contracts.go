package linear

import (
	"context"
	"net/http"
	"time"
)

// HTTPDoer performs the Linear adapter's external network request.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// RuntimePaths is the minimum runtime view needed to resolve a hosted source
// credential from the Factory or runtime directory tree.
type RuntimePaths interface {
	FactoryDir() string
	RuntimeBaseDir() string
}

// SecretResolver resolves the external credential used by the Linear adapter.
type SecretResolver func(
	context.Context,
	RuntimePaths,
	string,
) (string, error)

// Clock schedules hosted Linear poller waits.
type Clock interface {
	After(time.Duration) <-chan time.Time
}
