package details

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIProviderSessionDetailsUseGoldenExpectedMetadata loads a sanitized Codex
// Provider Session through the public HTTP/API detail endpoint served by
// root.BuildProcess + Process.Execute and proves the response structurally
// matches checked-in expected Provider Session metadata.
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
	if detail.ProviderSession.Id != request.SessionID {
		t.Fatalf("detail provider session id = %q, want %q", detail.ProviderSession.Id, request.SessionID)
	}
	if detail.ProviderSession.Provider != factoryapi.Codex {
		t.Fatalf("detail provider = %q, want codex", detail.ProviderSession.Provider)
	}
	if detail.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("detail kind = %q, want session_id", detail.ProviderSession.Kind)
	}
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
