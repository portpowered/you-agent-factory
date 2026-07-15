package climanifestparity_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
)

func TestProductionManifestHandlerBinding_SessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	mismatches := climanifestparity.CompareDeclaredHandler(record, climanifestparity.SessionShowHandlerID, climanifestparity.SessionShowOperationID)
	mismatches = append(mismatches, climanifestparity.CompareHandlerOpenAPIBinding(
		record,
		loadBundledOpenAPIContract(t),
		climanifestparity.SessionShowHTTPMethod,
		climanifestparity.SessionShowHTTPPath,
	)...)
	if len(mismatches) > 0 {
		t.Fatalf("contract handler/OpenAPI binding drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}
}

func TestProductionManifestHandlerBinding_SessionShowLiveServiceCall(t *testing.T) {
	var gotPaths []string
	var gotMethods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethods = append(gotMethods, r.Method)
		gotPaths = append(gotPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"session-beta","runtime":{"orchestratorKind":"JAVASCRIPT"}}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if len(gotPaths) == 0 || gotPaths[0] != "/factory-sessions/session-beta" {
		t.Fatalf("live HTTP paths = %#v, want primary GET /factory-sessions/session-beta for contracted %s", gotPaths, climanifestparity.SessionShowHTTPPath)
	}
	if len(gotMethods) == 0 || gotMethods[0] != climanifestparity.SessionShowHTTPMethod {
		t.Fatalf("live HTTP methods = %#v, want primary %s for contracted %s binding", gotMethods, climanifestparity.SessionShowHTTPMethod, climanifestparity.SessionShowOperationID)
	}
}

func loadBundledOpenAPIContract(t *testing.T) *openapi3.T {
	t.Helper()
	openAPIPath := testutil.MustRepoPath(t, "api/openapi.yaml")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(openAPIPath)
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}
