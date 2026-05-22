package workcontent

import (
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPartsFromGeneratedPreservesSupportedPartOrderAndValues(t *testing.T) {
	content := factoryapi.WorkContent{
		mustGeneratedTextPart(t, "alpha"),
		mustGeneratedImagePart(t, "fixtures/alpha.png"),
		mustGeneratedTextPart(t, "omega"),
	}

	got := PartsFromGenerated(&content)

	want := []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/alpha.png"},
		{Type: interfaces.WorkContentPartTypeText, Text: "omega"},
	}
	assertWorkContentPartsEqual(t, got, want)
}

func TestPartsFromGeneratedReturnsNilForNilOrEmptyContent(t *testing.T) {
	if got := PartsFromGenerated(nil); got != nil {
		t.Fatalf("PartsFromGenerated(nil) = %#v, want nil", got)
	}

	content := factoryapi.WorkContent{}
	if got := PartsFromGenerated(&content); got != nil {
		t.Fatalf("PartsFromGenerated(empty) = %#v, want nil", got)
	}
}

func TestGeneratedPtrFromPartsPreservesSupportedPartOrderAndValues(t *testing.T) {
	parts := []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/alpha.png"},
		{Type: interfaces.WorkContentPartTypeText, Text: "omega"},
	}

	got := GeneratedPtrFromParts(parts)

	if got == nil {
		t.Fatalf("GeneratedPtrFromParts(parts) = nil, want content")
	}
	assertGeneratedWorkContentPartsEqual(t, got, parts)
}

func TestGeneratedPtrFromPartsReturnsNilForNilOrEmptyContent(t *testing.T) {
	if got := GeneratedPtrFromParts(nil); got != nil {
		t.Fatalf("GeneratedPtrFromParts(nil) = %#v, want nil", got)
	}

	if got := GeneratedPtrFromParts([]interfaces.WorkContentPart{}); got != nil {
		t.Fatalf("GeneratedPtrFromParts(empty) = %#v, want nil", got)
	}
}

func TestGeneratedPtrFromPartsSkipsUnsupportedParts(t *testing.T) {
	parts := []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
		{Type: interfaces.WorkContentPartType("unsupported"), File: "fixtures/ignored.wav"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/alpha.png"},
	}

	got := GeneratedPtrFromParts(parts)

	if got == nil {
		t.Fatalf("GeneratedPtrFromParts(parts) = nil, want supported content")
	}
	assertGeneratedWorkContentPartsEqual(t, got, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/alpha.png"},
	})
}

func TestGeneratedPtrFromPartsReturnsNilWhenAllPartsAreUnsupported(t *testing.T) {
	parts := []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartType("unsupported"), File: "fixtures/ignored.wav"},
	}

	if got := GeneratedPtrFromParts(parts); got != nil {
		t.Fatalf("GeneratedPtrFromParts(unsupported) = %#v, want nil", got)
	}
}

func TestPartsFromGeneratedAcceptsUppercaseAndExtendedContentShapes(t *testing.T) {
	jsonMetadata := factoryapi.WorkContentMetadata{"voice": "alloy"}
	content := factoryapi.WorkContent{
		mustGeneratedUpperTextPart(t, "Alpha", "input"),
		mustGeneratedAudioPart(t, "fixtures/output.wav", &jsonMetadata),
		mustGeneratedJSONPart(t, map[string]any{"voice": "alloy"}),
		mustGeneratedBinaryPart(t, "fixtures/blob.bin"),
	}

	got := PartsFromGenerated(&content)

	assertWorkContentPartsEqual(t, got, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Alpha", Label: "input"},
		{Type: interfaces.WorkContentPartTypeAudio, File: "fixtures/output.wav", Metadata: map[string]any{"voice": "alloy"}},
		{Type: interfaces.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"voice":"alloy"}`)},
		{Type: interfaces.WorkContentPartTypeBinary, File: "fixtures/blob.bin"},
	})
}

func TestGeneratedPtrFromPartsPreservesExtendedContentFields(t *testing.T) {
	parts := []interfaces.WorkContentPart{
		{
			Type:        interfaces.WorkContentPartTypeAudio,
			File:        "fixtures/output.wav",
			Label:       "speech",
			Role:        "assistant",
			ContentType: "audio/wav",
			ArtifactID:  "artifact-audio-1",
			Metadata:    map[string]any{"voice": "alloy"},
		},
		{
			Type:        interfaces.WorkContentPartTypeJSON,
			JSON:        json.RawMessage(`{"voice":"alloy","speed":1}`),
			ContentType: "application/json",
		},
	}

	got := GeneratedPtrFromParts(parts)
	if got == nil {
		t.Fatalf("GeneratedPtrFromParts(parts) = nil, want content")
	}
	assertGeneratedWorkContentPartsEqual(t, got, parts)
}

func mustGeneratedTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("encode text work content part: %v", err)
	}
	return part
}

func mustGeneratedImagePart(t *testing.T, file string) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
		Type: factoryapi.WorkContentPartTypeImage,
		File: file,
	}); err != nil {
		t.Fatalf("encode image work content part: %v", err)
	}
	return part
}

func mustGeneratedUpperTextPart(t *testing.T, text string, label string) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type:  factoryapi.WorkContentPartTypeTextUpper,
		Text:  text,
		Label: testStringPtr(label),
	}); err != nil {
		t.Fatalf("encode uppercase text work content part: %v", err)
	}
	return part
}

func mustGeneratedAudioPart(t *testing.T, file string, metadata *factoryapi.WorkContentMetadata) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkAudioContentPart(factoryapi.WorkAudioContentPart{
		Type:     factoryapi.WorkContentPartTypeAudio,
		File:     file,
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("encode audio work content part: %v", err)
	}
	return part
}

func mustGeneratedJSONPart(t *testing.T, value any) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkJsonContentPart(factoryapi.WorkJsonContentPart{
		Type: factoryapi.WorkContentPartTypeJSON,
		Json: value,
	}); err != nil {
		t.Fatalf("encode json work content part: %v", err)
	}
	return part
}

func mustGeneratedBinaryPart(t *testing.T, file string) factoryapi.WorkContentPart {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkBinaryContentPart(factoryapi.WorkBinaryContentPart{
		Type: factoryapi.WorkContentPartTypeBinary,
		File: file,
	}); err != nil {
		t.Fatalf("encode binary work content part: %v", err)
	}
	return part
}

func assertWorkContentPartsEqual(t *testing.T, got, want []interfaces.WorkContentPart) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(parts) = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if !workContentPartEqual(got[i], want[i]) {
			t.Fatalf("part[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertGeneratedWorkContentPartsEqual(t *testing.T, got *factoryapi.WorkContent, want []interfaces.WorkContentPart) {
	t.Helper()

	if got == nil {
		t.Fatalf("generated work content = nil, want %#v", want)
	}
	if len(*got) != len(want) {
		t.Fatalf("len(generated parts) = %d, want %d", len(*got), len(want))
	}
	roundTrip := PartsFromGenerated(got)
	assertWorkContentPartsEqual(t, roundTrip, want)
}

func workContentPartEqual(left, right interfaces.WorkContentPart) bool {
	if left.Type != right.Type ||
		left.Text != right.Text ||
		left.File != right.File ||
		left.Label != right.Label ||
		left.Role != right.Role ||
		left.ContentType != right.ContentType ||
		left.ArtifactID != right.ArtifactID {
		return false
	}
	if !rawJSONEqual(left.JSON, right.JSON) {
		return false
	}
	leftMetadata, _ := json.Marshal(left.Metadata)
	rightMetadata, _ := json.Marshal(right.Metadata)
	return string(leftMetadata) == string(rightMetadata)
}

func rawJSONEqual(left, right json.RawMessage) bool {
	if len(left) == 0 || len(right) == 0 {
		return len(left) == len(right)
	}
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	leftNormalized, _ := json.Marshal(leftValue)
	rightNormalized, _ := json.Marshal(rightValue)
	return string(leftNormalized) == string(rightNormalized)
}

func testStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
