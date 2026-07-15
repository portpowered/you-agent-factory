package invocation

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestResolveTextInput_PositionalOnly(t *testing.T) {
	text := "build the thing"

	got, err := ResolveTextInput(TextInputSources{PositionalText: &text})
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}

	assertResolvedTextInput(t, got, InputSourcePositionalText, text)
}

func TestResolveTextInput_StdinOnly(t *testing.T) {
	text := "build from stdin\n"

	got, err := ResolveTextInput(TextInputSources{StdinText: &text})
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}

	assertResolvedTextInput(t, got, InputSourceStdinText, text)
}

func TestResolveTextInput_RejectsPositionalAndStdinConflict(t *testing.T) {
	positional := "from args"
	stdin := "from stdin"

	_, err := ResolveTextInput(TextInputSources{
		PositionalText: &positional,
		StdinText:      &stdin,
	})

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeSourceConflict {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeSourceConflict)
	}
	want := []InputSourceLabel{InputSourcePositionalText, InputSourceStdinText}
	if len(inputErr.ConflictingSources) != len(want) {
		t.Fatalf("conflicting sources = %#v, want %#v", inputErr.ConflictingSources, want)
	}
	for i := range want {
		if inputErr.ConflictingSources[i] != want[i] {
			t.Fatalf("conflicting sources = %#v, want %#v", inputErr.ConflictingSources, want)
		}
	}
}

func TestResolveTextInput_RejectsEmptySelectedStdin(t *testing.T) {
	stdin := ""

	_, err := ResolveTextInput(TextInputSources{StdinText: &stdin})

	assertInputEmptyError(t, err, InputSourceStdinText)
}

func TestResolveTextInput_RejectsWhitespaceOnlyPositional(t *testing.T) {
	text := "   "

	_, err := ResolveTextInput(TextInputSources{PositionalText: &text})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func TestResolveTextInput_RejectsWhitespaceOnlyStdin(t *testing.T) {
	stdin := "  \n\t  "

	_, err := ResolveTextInput(TextInputSources{StdinText: &stdin})

	assertInputEmptyError(t, err, InputSourceStdinText)
}

func TestResolveAPITextInputContent_RejectsWhitespaceOnlyText(t *testing.T) {
	_, err := ResolveAPITextInputContent([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "   ",
	}})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func TestResolveAPITextInputContent_RejectsWhitespaceOnlyJoinedParts(t *testing.T) {
	_, err := ResolveAPITextInputContent([]work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "  "},
		{Type: work.WorkContentPartTypeText, Text: "\t"},
	})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func assertInputEmptyError(t *testing.T, err error, wantSource InputSourceLabel) {
	t.Helper()

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeEmpty {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeEmpty)
	}
	if inputErr.Source != wantSource {
		t.Fatalf("source = %q, want %q", inputErr.Source, wantSource)
	}
}

func assertResolvedTextInput(t *testing.T, got ResolvedInput, wantSource InputSourceLabel, wantText string) {
	t.Helper()

	if got.Source != wantSource {
		t.Fatalf("source = %q, want %q", got.Source, wantSource)
	}
	if got.Text != wantText {
		t.Fatalf("text = %q, want %q", got.Text, wantText)
	}
	if len(got.Content) != 1 {
		t.Fatalf("content = %#v, want one text part", got.Content)
	}
	if got.Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("content[0].type = %q, want %q", got.Content[0].Type, work.WorkContentPartTypeText)
	}
	if got.Content[0].Text != wantText {
		t.Fatalf("content[0].text = %q, want %q", got.Content[0].Text, wantText)
	}
}
