package workstation

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func assertGeneratedWorkContentParts(
	t *testing.T,
	content *[]work.WorkContentPart,
	want []work.WorkContentPart,
) {
	t.Helper()
	if content == nil {
		t.Fatalf("content = nil, want %#v", want)
	}
	gotJSON, err := json.Marshal(*content)
	if err != nil {
		t.Fatalf("marshal projected content: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal expected content: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("content = %s, want %s", gotJSON, wantJSON)
	}
}
