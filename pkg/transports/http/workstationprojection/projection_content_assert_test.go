package workstationprojection

import (
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

func assertGeneratedWorkContentParts(t *testing.T, content *factoryapi.WorkContent, want []work.WorkContentPart) {
	t.Helper()
	if content == nil {
		t.Fatalf("content = nil, want %#v", want)
	}
	if len(*content) != len(want) {
		t.Fatalf("content count = %d, want %d", len(*content), len(want))
	}
	for i, wantPart := range want {
		assertGeneratedWorkContentPart(t, (*content)[i], i, wantPart)
	}
}

func assertGeneratedWorkContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	switch want.Type {
	case work.WorkContentPartTypeText:
		assertGeneratedTextContentPart(t, got, index, want)
	case work.WorkContentPartTypeImage:
		assertGeneratedImageContentPart(t, got, index, want)
	case work.WorkContentPartTypeAudio:
		assertGeneratedAudioContentPart(t, got, index, want)
	case work.WorkContentPartTypeJSON:
		assertGeneratedJSONContentPart(t, got, index, want)
	case work.WorkContentPartTypeBinary:
		assertGeneratedBinaryContentPart(t, got, index, want)
	default:
		t.Fatalf("unsupported expected content type %q", want.Type)
	}
}

func assertGeneratedTextContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode text: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeText || part.Text != want.Text {
		t.Fatalf("content[%d] = %#v, want text %q", index, part, want.Text)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedImageContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode image: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeImage || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want image url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedAudioContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode audio: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeAudio || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want audio url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedJSONContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode json: %v", index, err)
	}
	rawJSON, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("content[%d] encode json: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeJSON || string(rawJSON) != string(want.JSON) {
		t.Fatalf("content[%d] = %#v, want json %s", index, part, want.JSON)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedBinaryContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkBinaryContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode binary: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeBinary || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want binary url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedPartSharedFields(t *testing.T, index int, slot *string, label *string, role *string, contentType *string, artifactID *string, metadata *factoryapi.WorkContentMetadata, want work.WorkContentPart) {
	t.Helper()

	if derefString(slot) != want.Slot ||
		derefString(label) != want.Label ||
		derefString(role) != want.Role ||
		derefString(contentType) != want.ContentType ||
		derefString(artifactID) != want.ArtifactID {
		t.Fatalf("content[%d] shared fields mismatch for %#v", index, want)
	}
	gotMetadata, _ := json.Marshal(metadata)
	wantMetadata, _ := json.Marshal(want.Metadata)
	if string(gotMetadata) != string(wantMetadata) {
		t.Fatalf("content[%d] metadata = %s, want %s", index, gotMetadata, wantMetadata)
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func deprecatedGeneratedFile(file *factoryapi.WorkContentDeprecatedFileProperty) string {
	if file == nil {
		return ""
	}
	return string(*file)
}
