package factorysessionsse

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func newFactorySessionSSEHarness(t *testing.T, timeout time.Duration) *FactorySessionSSEHarness {
	t.Helper()
	return NewFactorySessionSSEHarness(t, timeout, &http.Client{}, context.Background())
}
