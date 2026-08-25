package capture

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const invocationSensitiveRedactionMarker = "<redacted>"

func redactInvocationSensitiveSpans(
	object map[string]any,
	spans []factorydefinitions.InvocationSensitiveJSONSpan,
) (map[string]any, error) {
	if len(spans) == 0 {
		return object, nil
	}
	byPointer, pointers, err := groupInvocationSensitiveSpans(spans)
	if err != nil {
		return nil, err
	}
	for _, pointer := range pointers {
		tokens, err := decodeInvocationJSONPointer(pointer)
		if err != nil {
			return nil, err
		}
		text, err := lookupInvocationSensitiveText(object, tokens, pointer)
		if err != nil {
			return nil, err
		}
		text, err = redactInvocationSensitiveText(pointer, text, byPointer[pointer])
		if err != nil {
			return nil, err
		}
		if err := setInvocationJSONValue(object, tokens, text); err != nil {
			return nil, fmt.Errorf("replace sensitive span at JSON pointer %q: %w", pointer, err)
		}
	}
	return object, nil
}

func groupInvocationSensitiveSpans(
	spans []factorydefinitions.InvocationSensitiveJSONSpan,
) (map[string][]factorydefinitions.InvocationSensitiveJSONSpan, []string, error) {
	byPointer := make(map[string][]factorydefinitions.InvocationSensitiveJSONSpan)
	for _, span := range spans {
		if strings.TrimSpace(span.JSONPointer) == "" || !strings.HasPrefix(span.JSONPointer, "/") {
			return nil, nil, fmt.Errorf("sensitive span has invalid JSON pointer %q", span.JSONPointer)
		}
		byPointer[span.JSONPointer] = append(byPointer[span.JSONPointer], span)
	}
	pointers := make([]string, 0, len(byPointer))
	for pointer := range byPointer {
		pointers = append(pointers, pointer)
	}
	sort.Strings(pointers)
	return byPointer, pointers, nil
}

func lookupInvocationSensitiveText(document any, tokens []string, pointer string) (string, error) {
	value, ok := lookupInvocationJSONValue(document, tokens)
	if !ok {
		return "", fmt.Errorf("sensitive span JSON pointer %q is not present", pointer)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("sensitive span JSON pointer %q does not identify a string", pointer)
	}
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("sensitive span JSON pointer %q identifies invalid UTF-8", pointer)
	}
	return text, nil
}

func redactInvocationSensitiveText(
	pointer string,
	text string,
	spans []factorydefinitions.InvocationSensitiveJSONSpan,
) (string, error) {
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].Start != spans[right].Start {
			return spans[left].Start < spans[right].Start
		}
		return spans[left].End < spans[right].End
	})
	previousEnd := 0
	for index, span := range spans {
		if err := validateInvocationSensitiveSpan(pointer, text, index, previousEnd, span); err != nil {
			return "", err
		}
		previousEnd = span.End
	}
	for index := len(spans) - 1; index >= 0; index-- {
		span := spans[index]
		text = text[:span.Start] + invocationSensitiveRedactionMarker + text[span.End:]
	}
	return text, nil
}

func validateInvocationSensitiveSpan(
	pointer string,
	text string,
	index int,
	previousEnd int,
	span factorydefinitions.InvocationSensitiveJSONSpan,
) error {
	if span.Start < 0 || span.End <= span.Start || span.End > len(text) {
		return fmt.Errorf(
			"sensitive span JSON pointer %q has invalid byte range [%d,%d)",
			pointer, span.Start, span.End,
		)
	}
	if !invocationUTF8Boundary(text, span.Start) || !invocationUTF8Boundary(text, span.End) {
		return fmt.Errorf(
			"sensitive span JSON pointer %q has a non-UTF-8 boundary [%d,%d)",
			pointer, span.Start, span.End,
		)
	}
	if index > 0 && span.Start < previousEnd {
		return fmt.Errorf("sensitive spans overlap at JSON pointer %q", pointer)
	}
	return nil
}

func invocationUTF8Boundary(value string, offset int) bool {
	return offset == 0 || offset == len(value) || utf8.RuneStart(value[offset])
}

func decodeInvocationJSONPointer(pointer string) ([]string, error) {
	if pointer == "" || pointer[0] != '/' {
		return nil, fmt.Errorf("sensitive span JSON pointer %q must start with '/'", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	for index, part := range parts {
		var builder strings.Builder
		for cursor := 0; cursor < len(part); cursor++ {
			if part[cursor] != '~' {
				builder.WriteByte(part[cursor])
				continue
			}
			if cursor+1 >= len(part) || (part[cursor+1] != '0' && part[cursor+1] != '1') {
				return nil, fmt.Errorf("sensitive span JSON pointer %q has invalid escape in token %d", pointer, index)
			}
			if part[cursor+1] == '0' {
				builder.WriteByte('~')
			} else {
				builder.WriteByte('/')
			}
			cursor++
		}
		parts[index] = builder.String()
	}
	return parts, nil
}

func lookupInvocationJSONValue(document any, tokens []string) (any, bool) {
	current := document
	for _, token := range tokens {
		switch value := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = value[token]
			if !ok {
				return nil, false
			}
		case []any:
			index, ok := invocationJSONIndex(token, len(value))
			if !ok {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func setInvocationJSONValue(document any, tokens []string, replacement string) error {
	if len(tokens) == 0 {
		return fmt.Errorf("root string replacement is unsupported")
	}
	parent := document
	for _, token := range tokens[:len(tokens)-1] {
		switch value := parent.(type) {
		case map[string]any:
			child, ok := value[token]
			if !ok {
				return fmt.Errorf("parent token %q is missing", token)
			}
			parent = child
		case []any:
			index, ok := invocationJSONIndex(token, len(value))
			if !ok {
				return fmt.Errorf("parent array token %q is invalid", token)
			}
			parent = value[index]
		default:
			return fmt.Errorf("parent token %q is not traversable", token)
		}
	}

	last := tokens[len(tokens)-1]
	switch value := parent.(type) {
	case map[string]any:
		if _, ok := value[last]; !ok {
			return fmt.Errorf("target token %q is missing", last)
		}
		value[last] = replacement
	case []any:
		index, ok := invocationJSONIndex(last, len(value))
		if !ok {
			return fmt.Errorf("target array token %q is invalid", last)
		}
		value[index] = replacement
	default:
		return fmt.Errorf("target token %q is not writable", last)
	}
	return nil
}

func invocationJSONIndex(token string, length int) (int, bool) {
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, false
	}
	index, err := strconv.Atoi(token)
	if err != nil || index < 0 || index >= length {
		return 0, false
	}
	return index, true
}
