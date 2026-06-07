package invocations

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	Content []interfaces.WorkContentPart
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
		if *sources.StdinText == "" {
			return ResolvedInput{}, &InputError{
				Code:    InputErrorCodeEmpty,
				Message: "invocation stdin input is empty",
				Source:  InputSourceStdinText,
			}
		}
		return resolvedTextInput(InputSourceStdinText, *sources.StdinText), nil
	}

	if sources.PositionalText != nil {
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

func resolvedTextInput(source InputSourceLabel, text string) ResolvedInput {
	return ResolvedInput{
		Source: source,
		Text:   text,
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: text,
		}},
	}
}
