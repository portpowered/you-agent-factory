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
