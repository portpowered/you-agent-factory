// Package namevalue owns the shared localized customer-facing metadata
// contract used by Factory definitions and their graph entities.
package namevalue

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

const TypeLocalizableAsset = "LOCALIZABLE_ASSET"

// Config carries customer-facing copy with a required base fallback and
// optional exact locale overrides. ID remains opaque metadata.
type Config struct {
	Type    string            `json:"type" yaml:"type"`
	Value   string            `json:"value" yaml:"value"`
	Locales []string          `json:"locales,omitempty" yaml:"locales,omitempty"`
	Values  map[string]string `json:"values,omitempty" yaml:"values,omitempty"`
	ID      string            `json:"id,omitempty" yaml:"id,omitempty"`
}

// ValidationError identifies the exact NameValue field that failed semantic
// validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate enforces the semantic constraints that OpenAPI 3.0 cannot express
// for locale-keyed maps.
func Validate(value Config) error {
	if value.Type != TypeLocalizableAsset {
		return &ValidationError{Field: "type", Message: fmt.Sprintf("unsupported value %q; use %q", value.Type, TypeLocalizableAsset)}
	}
	if strings.TrimSpace(value.Value) == "" {
		return &ValidationError{Field: "value", Message: "base fallback must not be empty"}
	}
	if err := validateLocaleList(value.Locales); err != nil {
		return err
	}
	return validateLocaleValues(value.Values)
}

// Validate checks this value's discriminator, fallback, and locale metadata.
func (value Config) Validate() error {
	return Validate(value)
}

// Resolve returns only an exact canonical override. Any absent, differently
// cased, or less-specific locale falls back to the base value.
func Resolve(value Config, locale string) string {
	if localized, ok := value.Values[locale]; ok && isCanonicalBCP47(locale) {
		return localized
	}
	return value.Value
}

func validateLocaleList(locales []string) error {
	seen := make(map[string]string, len(locales))
	for index, locale := range locales {
		field := fmt.Sprintf("locales[%d]", index)
		canonical, err := canonicalBCP47(locale)
		if err != nil {
			return &ValidationError{Field: field, Message: err.Error()}
		}
		if previous, ok := seen[canonical]; ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("locale %q collides with %s after normalization", locale, previous)}
		}
		seen[canonical] = field
		if locale != canonical {
			return &ValidationError{Field: field, Message: fmt.Sprintf("locale %q is not canonical; use %q", locale, canonical)}
		}
	}
	return nil
}

func validateLocaleValues(values map[string]string) error {
	keys := make([]string, 0, len(values))
	for locale := range values {
		keys = append(keys, locale)
	}
	sort.Strings(keys)

	seen := make(map[string]string, len(keys))
	for _, locale := range keys {
		field := fmt.Sprintf("values[%q]", locale)
		canonical, err := canonicalBCP47(locale)
		if err != nil {
			return &ValidationError{Field: field, Message: err.Error()}
		}
		if previous, ok := seen[canonical]; ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("locale %q collides with %s after normalization", locale, previous)}
		}
		seen[canonical] = field
		if locale != canonical {
			return &ValidationError{Field: field, Message: fmt.Sprintf("locale %q is not canonical; use %q", locale, canonical)}
		}
	}
	return nil
}

func canonicalBCP47(locale string) (string, error) {
	if strings.TrimSpace(locale) == "" {
		return "", fmt.Errorf("locale must not be empty")
	}
	for index, character := range locale {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			(character == '-' && index > 0 && index < len(locale)-1) {
			continue
		}
		return "", fmt.Errorf("locale %q is not a valid BCP 47 tag", locale)
	}
	if strings.Contains(locale, "--") {
		return "", fmt.Errorf("locale %q is not a valid BCP 47 tag", locale)
	}
	tag, err := language.Parse(locale)
	if err != nil {
		return "", fmt.Errorf("locale %q is not a valid BCP 47 tag", locale)
	}
	return tag.String(), nil
}

func isCanonicalBCP47(locale string) bool {
	canonical, err := canonicalBCP47(locale)
	return err == nil && locale == canonical
}
