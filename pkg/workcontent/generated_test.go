package workcontent

import (
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
		{Type: interfaces.WorkContentPartType("audio"), File: "fixtures/ignored.wav"},
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
		{Type: interfaces.WorkContentPartType("audio"), File: "fixtures/ignored.wav"},
	}

	if got := GeneratedPtrFromParts(parts); got != nil {
		t.Fatalf("GeneratedPtrFromParts(unsupported) = %#v, want nil", got)
	}
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

func assertWorkContentPartsEqual(t *testing.T, got, want []interfaces.WorkContentPart) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(parts) = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
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
