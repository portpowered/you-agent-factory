package main

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContentClass is the canonical purpose of an authored text span.
type ContentClass string

const (
	ContentClassLabel       ContentClass = "label"
	ContentClassProcedural  ContentClass = "procedural"
	ContentClassDescriptive ContentClass = "descriptive"
	ContentClassTechnical   ContentClass = "technical"
	ContentClassTerm        ContentClass = "technical-term"
)

// RuleID identifies one deterministic rule from the repository writing standard.
type RuleID string

const (
	RuleProceduralSentenceLength   RuleID = "B-PROC-20"
	RuleDescriptiveSentenceLength  RuleID = "B-DESC-25"
	RuleDescriptiveParagraphLength RuleID = "B-PARA-6"
	RuleSemicolon                  RuleID = "B-SEMICOLON"
	RuleContraction                RuleID = "B-CONTRACTION"
	RulePublicTerm                 RuleID = "B-TERM-PUBLIC"
	RuleTermCase                   RuleID = "B-TERM-CASE"
	RuleLiteral                    RuleID = "B-LITERAL"
	RuleSuppression                RuleID = "B-SUPPRESSION"
	RuleBaselineStale              RuleID = "B-BASELINE-STALE"
	RuleBaselineNew                RuleID = "B-BASELINE-NEW"
	RuleParse                      RuleID = "B-PARSE"
)

// Severity is intentionally expressed in the standard's blocking/advisory
// vocabulary. Story 001 only emits blocking deterministic findings.
type Severity string

const SeverityBlocking Severity = "blocking"

// Rule describes the policy data needed by the pure evaluator. The rule ID,
// applicability, and threshold are checked against the canonical writing
// standard while loading the policy.
type Rule struct {
	ID        RuleID
	Severity  Severity
	Supported bool
	Classes   []ContentClass
	Limit     int
}

// Alternative is one terminology-register spelling that must be replaced on
// customer-facing prose surfaces.
type Alternative struct {
	Text        string
	Status      string
	Replacement string
}

// Term is the evaluator's immutable view of one terminology-register record.
type Term struct {
	Canonical               string
	Category                string
	ApprovedForms           map[string]string
	DiscouragedAlternatives []Alternative
	Surfaces                []string
}

const (
	surfaceCustomerDocumentation         = "customer documentation"
	surfaceCLIHelp                       = "cli help"
	surfaceCLIReferenceDocumentation     = "cli and reference documentation"
	surfaceCLIAndCustomerDocumentation   = "cli and customer documentation"
	surfaceCLIAndAPIDocumentation        = "cli and api documentation"
	surfaceProviderCLIAndAPIDoc          = "provider cli and api documentation"
	surfaceModelCLIAndCustomerDoc        = "model cli and customer documentation"
	surfaceInternalArchitecture          = "explicitly internal architecture material"
	surfaceImplementationRuntimePackages = "implementation-focused runtime packages"
)

// Policy is the canonical policy snapshot used by Analyze. It contains no
// filesystem handles or other mutable runtime state.
type Policy struct {
	ContentClasses map[ContentClass]string
	Rules          map[RuleID]Rule
	Terms          []Term
}

type ruleSpec struct {
	id        RuleID
	classes   []ContentClass
	limit     int
	supported bool
}

func canonicalRuleSpecs() []ruleSpec {
	natural := []ContentClass{
		ContentClassLabel,
		ContentClassProcedural,
		ContentClassDescriptive,
	}
	return []ruleSpec{
		{id: RuleProceduralSentenceLength, classes: []ContentClass{ContentClassProcedural}, limit: 20, supported: true},
		{id: RuleDescriptiveSentenceLength, classes: []ContentClass{ContentClassDescriptive}, limit: 25, supported: true},
		{id: RuleDescriptiveParagraphLength, classes: []ContentClass{ContentClassDescriptive}, limit: 6, supported: true},
		{id: RuleSemicolon, classes: natural, supported: true},
		{id: RuleContraction, classes: natural, supported: true},
		{id: RulePublicTerm, classes: natural, supported: true},
		{id: RuleTermCase, classes: natural, supported: true},
		{id: RuleLiteral, supported: false},
		{id: RuleSuppression, supported: true},
		{id: RuleBaselineStale, supported: false},
		{id: RuleBaselineNew, supported: false},
		{id: RuleParse, supported: true},
	}
}

// LoadPolicy parses the repository's normative Markdown standard and YAML
// terminology register. The evaluator receives this snapshot explicitly so
// rule analysis remains pure and does not discover policy through globals.
func LoadPolicy(standard, terminology []byte) (Policy, error) {
	classes, err := parseCanonicalContentClasses(string(standard))
	if err != nil {
		return Policy{}, err
	}
	ruleIDs, err := parseCanonicalRuleIDs(string(standard))
	if err != nil {
		return Policy{}, err
	}
	terms, err := parseTerminologyRegister(terminology)
	if err != nil {
		return Policy{}, err
	}

	rules := make(map[RuleID]Rule)
	for _, spec := range canonicalRuleSpecs() {
		if !ruleIDs[spec.id] {
			return Policy{}, fmt.Errorf("writing standard is missing rule %s", spec.id)
		}
		rules[spec.id] = Rule{
			ID:        spec.id,
			Severity:  SeverityBlocking,
			Supported: spec.supported,
			Classes:   append([]ContentClass(nil), spec.classes...),
			Limit:     spec.limit,
		}
	}

	return Policy{ContentClasses: classes, Rules: rules, Terms: terms}, nil
}

func parseCanonicalContentClasses(standard string) (map[ContentClass]string, error) {
	pattern := regexp.MustCompile(`(?mi)^###\s+([^\r\n]+?)\s*$`)
	classes := make(map[ContentClass]string)
	for _, match := range pattern.FindAllStringSubmatch(standard, -1) {
		name := strings.TrimSpace(match[1])
		var class ContentClass
		switch strings.ToLower(name) {
		case "labels":
			class = ContentClassLabel
		case "procedural prose":
			class = ContentClassProcedural
		case "descriptive prose":
			class = ContentClassDescriptive
		case "machine and technical text":
			class = ContentClassTechnical
		case "technical terms":
			class = ContentClassTerm
		default:
			continue
		}
		classes[class] = name
	}

	for _, class := range []ContentClass{
		ContentClassLabel,
		ContentClassProcedural,
		ContentClassDescriptive,
		ContentClassTechnical,
		ContentClassTerm,
	} {
		if _, ok := classes[class]; !ok {
			return nil, fmt.Errorf("writing standard is missing content class %q", class)
		}
	}
	return classes, nil
}

func parseCanonicalRuleIDs(standard string) (map[RuleID]bool, error) {
	pattern := regexp.MustCompile(`(?m)^[[:space:]]*-[[:space:]]+\*\*([A-Z][A-Z0-9-]*):\*\*`)
	ruleIDs := make(map[RuleID]bool)
	for _, match := range pattern.FindAllStringSubmatch(standard, -1) {
		ruleIDs[RuleID(match[1])] = true
	}
	if len(ruleIDs) == 0 {
		return nil, fmt.Errorf("writing standard contains no rule IDs")
	}
	return ruleIDs, nil
}

type terminologyDocument struct {
	Version int                 `yaml:"version"`
	Schema  yaml.Node           `yaml:"schema"`
	Terms   []terminologyRecord `yaml:"terms"`
}

type terminologyRecord struct {
	Canonical               string            `yaml:"canonical"`
	Definition              string            `yaml:"definition"`
	Category                string            `yaml:"category"`
	ApprovedForms           approvedForms     `yaml:"approvedForms"`
	DiscouragedAlternatives []termAlternative `yaml:"discouragedAlternatives"`
	Example                 string            `yaml:"example"`
	Surfaces                []string          `yaml:"surfaces"`
	Owner                   string            `yaml:"owner"`
}

type approvedForms struct {
	Singular *string `yaml:"singular"`
	Plural   *string `yaml:"plural"`
	Verb     *string `yaml:"verb"`
}

type termAlternative struct {
	Text        string `yaml:"text"`
	Status      string `yaml:"status"`
	Replacement string `yaml:"replacement"`
}

func parseTerminologyRegister(data []byte) ([]Term, error) {
	var document terminologyDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode terminology register: %w", err)
	}
	if document.Version != 1 {
		return nil, fmt.Errorf("unsupported terminology register version %d", document.Version)
	}
	if len(document.Terms) == 0 {
		return nil, fmt.Errorf("terminology register has no terms")
	}

	knownValues := make(map[string]bool)
	for _, record := range document.Terms {
		canonical := strings.TrimSpace(record.Canonical)
		if canonical == "" {
			return nil, fmt.Errorf("terminology register contains an empty canonical term")
		}
		if err := validateTerminologyRecord(record, canonical); err != nil {
			return nil, err
		}
		knownValues[canonical] = true
		for _, value := range []*string{record.ApprovedForms.Singular, record.ApprovedForms.Plural, record.ApprovedForms.Verb} {
			if value != nil && strings.TrimSpace(*value) != "" {
				knownValues[strings.TrimSpace(*value)] = true
			}
		}
	}

	terms := make([]Term, 0, len(document.Terms))
	seen := make(map[string]bool)
	for _, record := range document.Terms {
		canonical := strings.TrimSpace(record.Canonical)
		key := strings.ToLower(canonical)
		if seen[key] {
			return nil, fmt.Errorf("terminology register repeats canonical term %q", canonical)
		}
		seen[key] = true
		if !allowedTermCategory(record.Category) {
			return nil, fmt.Errorf("term %q has unsupported category %q", canonical, record.Category)
		}
		forms := make(map[string]string)
		for name, value := range map[string]*string{
			"singular": record.ApprovedForms.Singular,
			"plural":   record.ApprovedForms.Plural,
			"verb":     record.ApprovedForms.Verb,
		} {
			if value != nil {
				trimmed := strings.TrimSpace(*value)
				if trimmed == "" {
					return nil, fmt.Errorf("term %q has an empty %s form", canonical, name)
				}
				forms[name] = trimmed
			}
		}

		alternatives := make([]Alternative, 0, len(record.DiscouragedAlternatives))
		for _, alternative := range record.DiscouragedAlternatives {
			item := Alternative{
				Text:        strings.TrimSpace(alternative.Text),
				Status:      strings.TrimSpace(alternative.Status),
				Replacement: strings.TrimSpace(alternative.Replacement),
			}
			if item.Text == "" || item.Replacement == "" {
				return nil, fmt.Errorf("term %q has an incomplete alternative", canonical)
			}
			if item.Status != "discouraged" && item.Status != "prohibited" {
				return nil, fmt.Errorf("term %q alternative %q has unsupported status %q", canonical, item.Text, item.Status)
			}
			if item.Text == item.Replacement || !knownValues[item.Replacement] {
				return nil, fmt.Errorf("term %q alternative %q has invalid replacement %q", canonical, item.Text, item.Replacement)
			}
			alternatives = append(alternatives, item)
		}
		slices.SortStableFunc(alternatives, func(left, right Alternative) int {
			if len(left.Text) != len(right.Text) {
				return len(right.Text) - len(left.Text)
			}
			return strings.Compare(strings.ToLower(left.Text), strings.ToLower(right.Text))
		})
		terms = append(terms, Term{
			Canonical:               canonical,
			Category:                strings.TrimSpace(record.Category),
			ApprovedForms:           forms,
			DiscouragedAlternatives: alternatives,
			Surfaces:                append([]string(nil), record.Surfaces...),
		})
	}

	slices.SortStableFunc(terms, func(left, right Term) int {
		if len(left.Canonical) != len(right.Canonical) {
			return len(right.Canonical) - len(left.Canonical)
		}
		return strings.Compare(strings.ToLower(left.Canonical), strings.ToLower(right.Canonical))
	})
	return terms, nil
}

func validateTerminologyRecord(record terminologyRecord, canonical string) error {
	if strings.TrimSpace(record.Definition) == "" {
		return fmt.Errorf("term %q has an empty definition", canonical)
	}
	if strings.TrimSpace(record.Example) == "" {
		return fmt.Errorf("term %q has an empty example", canonical)
	}
	if len(record.Surfaces) == 0 {
		return fmt.Errorf("term %q has no valid surfaces", canonical)
	}
	for _, surface := range record.Surfaces {
		if strings.TrimSpace(surface) == "" {
			return fmt.Errorf("term %q has an empty surface", canonical)
		}
	}
	if strings.TrimSpace(record.Owner) == "" {
		return fmt.Errorf("term %q has an empty owner", canonical)
	}
	return nil
}

func allowedTermCategory(category string) bool {
	switch category {
	case "product-term", "technical-noun", "technical-verb", "acronym", "command", "protected-literal":
		return true
	default:
		return false
	}
}

func (policy Policy) rule(id RuleID) (Rule, bool) {
	rule, ok := policy.Rules[id]
	return rule, ok && rule.Supported
}

func (policy Policy) knownClass(class ContentClass) bool {
	_, ok := policy.ContentClasses[class]
	return ok
}
