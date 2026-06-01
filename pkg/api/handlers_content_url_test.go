package api

import (
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSubmitWork_RejectsUnsupportedContentURLScheme(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"bad-url","workTypeName":"prd","content":[{"type":"image","url":"ftp://example.com/ui.png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].url url scheme must be one of file, http, https, or data")
}

func TestSubmitWork_RejectsURLAndFileConflict(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"conflict","workTypeName":"prd","content":[{"type":"image","url":"file://fixtures/ui.png","file":"fixtures/ui.png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].url and file cannot both be set on the same content part")
}

func TestSubmitWork_AcceptsLegacyFileOnlyContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"legacy-file","workTypeName":"prd","content":[{"type":"image","file":"fixtures/ui.png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 1 {
		t.Fatalf("submitted = %#v, want one normalized image part", mf.Submitted)
	}
	part := mf.Submitted[0].Content[0]
	if part.Type != interfaces.WorkContentPartTypeImage || part.URL != "file://fixtures/ui.png" {
		t.Fatalf("content[0] = %#v, want image with normalized url", part)
	}
	if part.File != "" {
		t.Fatalf("content[0].file = %q, want empty canonical file field", part.File)
	}
}
