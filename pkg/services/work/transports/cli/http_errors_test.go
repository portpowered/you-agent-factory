package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

func TestListPageStatusErrorPreservesServerDetails(t *testing.T) {
	t.Parallel()
	for _, page := range []int{1, 2} {
		t.Run(fmt.Sprint(page), func(t *testing.T) {
			t.Parallel()
			response := &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"code":"NOT_FOUND","family":"NOT_FOUND","message":"factory session not found","details":"server guidance"}`))}
			err := listPageStatusError(ListConfig{SessionID: "explicit"}, url.URL{}, workdomain.ListOptions{}, page, clihttp.Response{}, response)
			var apiErr *clihttp.APIError
			if !errors.As(fmt.Errorf("wrapped: %w", err), &apiErr) || apiErr.Response.Details != "server guidance" || apiErr.Response.Message != "factory session not found" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

type statusDoer struct{ status int }

func (doer statusDoer) Do(request *http.Request) (*http.Response, error) {
	return &http.Response{Request: request, StatusCode: doer.status, Body: io.NopCloser(strings.NewReader(`{"code":"NOT_FOUND","family":"NOT_FOUND","message":"synthetic rejection"}`))}, nil
}

func TestWatchTypedStatusPreservesRetryPolicy(t *testing.T) {
	t.Parallel()
	for _, status := range []int{404, 408, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			transport, err := clihttp.NewProtocol(statusDoer{status}, watchTestHTTPClock{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = openHTTPWatchEventStream(context.Background(), transport, "http://factory.test", "explicit", nil, nil, false)
			var apiErr *clihttp.APIError
			var statusErr *watchHTTPStatusError
			if !errors.As(err, &apiErr) || !errors.As(err, &statusErr) || apiErr.Response.Code != "NOT_FOUND" || statusErr.status != status {
				t.Fatalf("error = %#v", err)
			}
			if isRetryableWatchError(err) != (status != 404) || isRetryableWatchError(context.Canceled) {
				t.Fatalf("retry policy changed for %d", status)
			}
		})
	}
}
