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

func TestSubmitWork_RejectsMissingContentURL(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"missing-url","workTypeName":"prd","content":[{"type":"image","file":"fixtures/ui.png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].url is required for image content parts")
}
