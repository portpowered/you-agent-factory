package invocationreturnpolicy

import (
	"fmt"
	"strings"
)

type InputSourceLabel string

const (
	// InputSourcePositionalText identifies text supplied as CLI positional input.
	InputSourcePositionalText InputSourceLabel = "positional_text"

	// InputSourceStdinText identifies text supplied through stdin.
	InputSourceStdinText InputSourceLabel = "stdin_text"
)

// InputErrorCode is the stable machine-readable failure code for invocation
// input resolution.
type InputErrorCode string

const (
	// InputErrorCodeSourceConflict reports that more than one source supplied
	// the same logical invocation input.
	InputErrorCodeSourceConflict InputErrorCode = "INVOCATION_INPUT_SOURCE_CONFLICT"

	// InputErrorCodeEmpty reports that the selected input source had no text.
	InputErrorCodeEmpty InputErrorCode = "INVOCATION_INPUT_EMPTY"
)

// TextInputSources carries text-first input observed by a transport adapter.
// A nil source was not supplied; a non-nil source was selected even when its
// value is empty.
type TextInputSources struct {
	PositionalText *string
	StdinText      *string
}

// ResolvedInput is the normalized text-first invocation input passed from
// transport adapters into runtime invocation code.
type ResolvedInput struct {
	Source  InputSourceLabel
	Text    string
	Content []ContentPart
}

// InputError describes a stable invocation input resolution failure.
type InputError struct {
	Code               InputErrorCode
	Message            string
	Source             InputSourceLabel
	ConflictingSources []InputSourceLabel
}

func (e *InputError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ResolveTextInput applies the canonical text-first invocation source rules
// shared by CLI and API adapters.
func ResolveTextInput(sources TextInputSources) (ResolvedInput, error) {
	selected := suppliedTextSources(sources)
	if len(selected) > 1 {
		conflicts := make([]string, 0, len(selected))
		for _, source := range selected {
			conflicts = append(conflicts, string(source))
		}
		return ResolvedInput{}, &InputError{
			Code:               InputErrorCodeSourceConflict,
			Message:            fmt.Sprintf("invocation input sources conflict: %s", strings.Join(conflicts, ", ")),
			ConflictingSources: selected,
		}
	}

	if sources.StdinText != nil {
		if isLogicallyEmptyText(*sources.StdinText) {
			return ResolvedInput{}, &InputError{
				Code:    InputErrorCodeEmpty,
				Message: "invocation stdin input is empty",
				Source:  InputSourceStdinText,
			}
		}
		return resolvedTextInput(InputSourceStdinText, *sources.StdinText), nil
	}

	if sources.PositionalText != nil {
		if isLogicallyEmptyText(*sources.PositionalText) {
			return ResolvedInput{}, &InputError{
				Code:    InputErrorCodeEmpty,
				Message: "invocation input is empty",
				Source:  InputSourcePositionalText,
			}
		}
		return resolvedTextInput(InputSourcePositionalText, *sources.PositionalText), nil
	}

	return ResolvedInput{}, &InputError{
		Code:    InputErrorCodeEmpty,
		Message: "invocation input is empty",
	}
}

func suppliedTextSources(sources TextInputSources) []InputSourceLabel {
	var selected []InputSourceLabel
	if sources.PositionalText != nil {
		selected = append(selected, InputSourcePositionalText)
	}
	if sources.StdinText != nil {
		selected = append(selected, InputSourceStdinText)
	}
	return selected
}

func isLogicallyEmptyText(text string) bool {
	return strings.TrimSpace(text) == ""
}

// TextContentValidationError reports API invocation content that is not valid
// text-first input.
type TextContentValidationError struct {
	Message string
}

func (e *TextContentValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ResolveAPITextInputContent applies canonical text-first invocation rules to
// API text content parts joined with newlines.
func ResolveAPITextInputContent(parts []ContentPart) (ResolvedInput, error) {
	if len(parts) == 0 {
		return ResolvedInput{}, &InputError{
			Code:    InputErrorCodeEmpty,
			Message: "invocation input is empty",
		}
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != ContentPartTypeText {
			return ResolvedInput{}, &TextContentValidationError{
				Message: "content must contain only text parts when sourceKind is text",
			}
		}
		textParts = append(textParts, part.Text)
	}

	text := strings.Join(textParts, "\n")
	return ResolveTextInput(TextInputSources{
		PositionalText: &text,
	})
}

func resolvedTextInput(source InputSourceLabel, text string) ResolvedInput {
	return ResolvedInput{
		Source: source,
		Text:   text,
		Content: []ContentPart{{
			Type: ContentPartTypeText,
			Text: text,
		}},
	}
}
