package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestGetProviderSessionDetails_LoadsTimestampPrefixedCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl", strings.Join([]string{
		`{"type":"session_meta","id":"019e44f4-580e-7f32-981e-1e54ec6907d6"}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=019e44f4-580e-7f32-981e-1e54ec6907d6", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl" || resp.ProviderSession.Id != "019e44f4-580e-7f32-981e-1e54ec6907d6" || resp.Parse.EventCount != 2 {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed session path", resp)
	}
}

func TestGetProviderSessionDetails_PrefersExactCodexSessionFileWhenSupportedLayoutsBothExist(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}
