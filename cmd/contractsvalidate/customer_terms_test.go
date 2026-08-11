package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const customerTermsPath = "docs/internal/standards/writing/customer-technical-terms.yaml"

type customerTermsRegister struct {
	Version int                  `yaml:"version"`
	Schema  map[string]any       `yaml:"schema"`
	Terms   []customerTermRecord `yaml:"terms"`
}

type customerTermRecord struct {
	Canonical               string                    `yaml:"canonical"`
	Definition              string                    `yaml:"definition"`
	Category                string                    `yaml:"category"`
	ApprovedForms           map[string]*string        `yaml:"approvedForms"`
	DiscouragedAlternatives []customerTermAlternative `yaml:"discouragedAlternatives"`
	Example                 string                    `yaml:"example"`
	Surfaces                []string                  `yaml:"surfaces"`
	Owner                   string                    `yaml:"owner"`
}

type customerTermAlternative struct {
	Text        string `yaml:"text"`
	Status      string `yaml:"status"`
	Replacement string `yaml:"replacement"`
}

func TestCustomerTechnicalTermsRegisterIsValid(t *testing.T) {
	root := repositoryRoot(t)
	register := loadCustomerTermsRegister(t, filepath.Join(root, customerTermsPath))

	if err := validateCustomerTermsRegister(register); err != nil {
		t.Fatalf("customer terminology register is invalid: %v", err)
	}
}

func TestCustomerTechnicalTermsRegisterRejectsInvalidReplacementPolicy(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		want        string
	}{
		{name: "self replacement", replacement: "Factory", want: "must differ from replacement"},
		{name: "unregistered replacement", replacement: "Unregistered term", want: "is not a registered canonical or approved form"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			register := customerTermsRegister{
				Version: 1,
				Schema:  map[string]any{"termRecord": map[string]any{"canonical": "documented"}},
				Terms: []customerTermRecord{
					{
						Canonical:     "Factory",
						Definition:    "A customer-facing factory definition.",
						Category:      "product-term",
						ApprovedForms: map[string]*string{"singular": stringPointer("Factory"), "plural": stringPointer("Factories"), "verb": nil},
						DiscouragedAlternatives: []customerTermAlternative{{
							Text: "Factory", Status: "discouraged", Replacement: test.replacement,
						}},
						Example:  "Save the Factory.",
						Surfaces: []string{"Customer documentation"},
						Owner:    "docs/architecture/data-model.md",
					},
				},
			}

			err := validateCustomerTermsRegister(register)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCustomerTermsRegister() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func loadCustomerTermsRegister(t *testing.T, path string) customerTermsRegister {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var register customerTermsRegister
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&register); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return register
}

func validateCustomerTermsRegister(register customerTermsRegister) error {
	if register.Version != 1 {
		return fmt.Errorf("version = %d, want 1", register.Version)
	}
	if len(register.Schema) == 0 {
		return fmt.Errorf("schema is empty")
	}
	if len(register.Terms) == 0 {
		return fmt.Errorf("terms is empty")
	}

	allowedCategories := map[string]bool{
		"product-term":      true,
		"technical-noun":    true,
		"technical-verb":    true,
		"acronym":           true,
		"command":           true,
		"protected-literal": true,
	}
	registeredForms := make(map[string]struct{}, len(register.Terms)*4)
	for index, term := range register.Terms {
		if strings.TrimSpace(term.Canonical) == "" {
			return fmt.Errorf("term %d has empty canonical", index)
		}
		if _, exists := registeredForms[term.Canonical]; exists {
			return fmt.Errorf("term %d duplicates canonical %q", index, term.Canonical)
		}
		registeredForms[term.Canonical] = struct{}{}
		if strings.TrimSpace(term.Definition) == "" || strings.TrimSpace(term.Category) == "" ||
			strings.TrimSpace(term.Example) == "" || strings.TrimSpace(term.Owner) == "" {
			return fmt.Errorf("term %q has an empty required scalar", term.Canonical)
		}
		if !allowedCategories[term.Category] {
			return fmt.Errorf("term %q has unsupported category %q", term.Canonical, term.Category)
		}
		if len(term.ApprovedForms) != 3 {
			return fmt.Errorf("term %q approvedForms must contain singular, plural, and verb", term.Canonical)
		}
		for _, formName := range []string{"singular", "plural", "verb"} {
			form, present := term.ApprovedForms[formName]
			if !present {
				return fmt.Errorf("term %q approvedForms omits %s", term.Canonical, formName)
			}
			if form != nil && strings.TrimSpace(*form) != "" {
				registeredForms[*form] = struct{}{}
			}
		}
		if len(term.Surfaces) == 0 {
			return fmt.Errorf("term %q has no surfaces", term.Canonical)
		}
		if len(term.DiscouragedAlternatives) == 0 {
			return fmt.Errorf("term %q has no discouraged alternatives", term.Canonical)
		}
	}

	for _, term := range register.Terms {
		for alternativeIndex, alternative := range term.DiscouragedAlternatives {
			text := strings.TrimSpace(alternative.Text)
			replacement := strings.TrimSpace(alternative.Replacement)
			if text == "" || replacement == "" {
				return fmt.Errorf("term %q alternative %d has empty text or replacement", term.Canonical, alternativeIndex)
			}
			if text == replacement {
				return fmt.Errorf("term %q alternative %d text must differ from replacement", term.Canonical, alternativeIndex)
			}
			if alternative.Status != "discouraged" && alternative.Status != "prohibited" {
				return fmt.Errorf("term %q alternative %d has unsupported status %q", term.Canonical, alternativeIndex, alternative.Status)
			}
			if _, ok := registeredForms[replacement]; !ok {
				return fmt.Errorf("term %q alternative %d replacement %q is not a registered canonical or approved form", term.Canonical, alternativeIndex, replacement)
			}
		}
	}

	return nil
}

func stringPointer(value string) *string {
	return &value
}
