package factorysessionsse

import (
	"io"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	"go.uber.org/zap"
)

func newAPITestServer(f *testutil.MockFactory) *api.Server {
	logger, _ := zap.NewDevelopment()
	return api.NewServer(f, 8080, logger)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
