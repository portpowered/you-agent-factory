package details

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIProviderSessionDetailsUseGoldenExpectedMetadata proves HTTP/API Provider
// Session detail activates through the public GET /provider-sessions/detail endpoint
// after runtime lifecycle starts on a process composed only via
// support.StartFunctionalAPIServer (root.BuildProcess + edges.Edges). It loads a
// sanitized Codex success rollout and proves identity/provider/kind plus readable
// transcript structurally match checked-in expected Provider Session metadata.
//golden: docs/temp/functional/provider-sessions/codex/success/manifest.json
func TestAPIProviderSessionDetailsUseGoldenExpectedMetadata(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "codex-message-tool-success" {
		t.Fatalf("manifest.ID = %q, want codex-message-tool-success", loaded.Manifest.ID)
	}

	var request struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.SessionID == "" {
		t.Fatal("request.session_id must be non-empty")
	}

	rolloutPath := filepath.Join(caseDir, codexGoldenRolloutFile)
	rolloutContent, err := os.ReadFile(rolloutPath)
	if err != nil {
		t.Fatalf("read %s: %v", codexGoldenRolloutFile, err)
	}

	homeDir := t.TempDir()
	writeCodexGoldenRolloutFixture(t, codexSessionsRoot(homeDir), request.SessionID, string(rolloutContent))

	server := startAPIProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		codexProviderSessionDetailURL(server.URL(), request.SessionID),
	)
	assertProviderSessionDetailIdentity(
		t,
		detail,
		request.SessionID,
		factoryapi.Codex,
		factoryapi.LoadableProviderSessionKindSessionID,
	)
	if len(detail.Transcript) == 0 {
		t.Fatal("provider session detail transcript is empty, want readable success-session content")
	}

	observed := observeCodexProviderSessionDetailGolden(detail)
	if err := compareOrUpdateCodexProviderSessionDetailGolden(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("compareOrUpdateCodexProviderSessionDetailGolden: %v", err)
	}
}

// TestAPIProviderSessionRejectsRawFilesystemPathInput proves the public HTTP/API
// Provider Session detail endpoint rejects raw filesystem path input instead of
// treating host paths as session locators or returning fabricated detail from
// path traversal.
func TestAPIProviderSessionRejectsRawFilesystemPathInput(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success")))
	rolloutPath := filepath.Join(caseDir, codexGoldenRolloutFile)
	rolloutContent, err := os.ReadFile(rolloutPath)
	if err != nil {
		t.Fatalf("read %s: %v", codexGoldenRolloutFile, err)
	}

	homeDir := t.TempDir()
	validSessionID := "session_fixture_codex_api_path_rejection"
	writeCodexGoldenRolloutFixture(t, codexSessionsRoot(homeDir), validSessionID, string(rolloutContent))
	rolloutAbsolutePath := filepath.Join(
		codexSessionsRoot(homeDir),
		"2026",
		"07",
		"27",
		"rollout-"+validSessionID+".jsonl",
	)

	server := startAPIProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	for _, test := range []struct {
		name string
		id   string
	}{
		{name: "absolute host path", id: "/tmp/rollout-session.jsonl"},
		{name: "relative traversal", id: "../secret"},
		{name: "stored rollout absolute path", id: rolloutAbsolutePath},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := getCodexProviderSessionDetailErrorBody(
				t,
				server.URL(),
				test.id,
				http.StatusBadRequest,
			)
			if !strings.Contains(body, "path separators") {
				t.Fatalf("error body = %q, want path-separator rejection diagnostic", body)
			}

			var failure factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(body), &failure); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if failure.Code != factoryapi.ErrorResponseCodeBADREQUEST {
				t.Fatalf("error code = %q, want BAD_REQUEST", failure.Code)
			}
			if failure.Message == "" {
				t.Fatal("error message is empty, want customer-readable validation diagnostic")
			}
			if strings.Contains(body, `"transcript"`) || strings.Contains(body, `"providerSession"`) {
				t.Fatalf("validation error fabricated provider session detail: %s", body)
			}

			assertCodexProviderSessionErrorBodySafe(t, "api-path-rejection", body, homeDir)
		})
	}
}

// TestAPIUnsupportedProviderSessionKindReturnsTypedError proves the public HTTP/API
// Provider Session detail endpoint rejects unsupported provider session kinds with
// a typed BAD_REQUEST error instead of opaque server failures or fabricated detail
// bodies that invent session identity or transcript content.
func TestAPIUnsupportedProviderSessionKindReturnsTypedError(t *testing.T) {
	homeDir := t.TempDir()
	server := startAPIProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	for _, test := range []struct {
		name     string
		provider string
		kind     string
		id       string
	}{
		{
			name:     "codex unsupported kind",
			provider: string(factoryapi.Codex),
			kind:     "path",
			id:       "sess-unsupported-kind-codex",
		},
		{
			name:     "cursor unsupported kind",
			provider: string(factoryapi.Cursor),
			kind:     "path",
			id:       "sess-unsupported-kind-cursor",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := getAPIProviderSessionDetailErrorBody(
				t,
				server.URL(),
				test.provider,
				test.kind,
				test.id,
				http.StatusBadRequest,
			)

			var failure factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(body), &failure); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if failure.Code != factoryapi.ErrorResponseCodeBADREQUEST {
				t.Fatalf("error code = %q, want BAD_REQUEST", failure.Code)
			}
			if failure.Message == "" {
				t.Fatal("error message is empty, want customer-readable validation diagnostic")
			}
			if strings.Contains(body, `"transcript"`) || strings.Contains(body, `"providerSession"`) {
				t.Fatalf("unsupported kind error fabricated provider session detail: %s", body)
			}

			assertCodexProviderSessionErrorBodySafe(t, "api-unsupported-kind", body, homeDir)
		})
	}
}

func providerSessionDetailURL(baseURL, provider, kind, id string) string {
	query := url.Values{}
	query.Set("provider", provider)
	query.Set("kind", kind)
	query.Set("id", id)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func getAPIProviderSessionDetailErrorBody(
	t *testing.T,
	baseURL, provider, kind, id string,
	wantStatus int,
) string {
	t.Helper()

	response, err := http.Get(providerSessionDetailURL(baseURL, provider, kind, id))
	if err != nil {
		t.Fatalf("GET provider session detail: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read provider session detail body: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"GET provider session detail status = %d, want %d: %s",
			response.StatusCode,
			wantStatus,
			strings.TrimSpace(string(body)),
		)
	}
	return string(body)
}

func startAPIProviderSessionDetailServer(
	t *testing.T,
	homeDir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()

	if edges.ProviderSessionResolveHomeDirectory == nil {
		edges.ProviderSessionResolveHomeDirectory = func() (string, error) { return homeDir, nil }
	}
	dir := support.ScaffoldSingleStepFactory(t, "api-provider-session-detail")
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
	})
}
