package main

import (
	"fmt"
	"strings"
)

const suppressionPrefix = "prosecheck:ignore"

// ParseSuppressionDirective parses the bounded machine-readable exception
// comment accepted by the analyzer. It intentionally does not decide whether
// the rule exists; that requires the canonical Policy supplied to Analyze.
func ParseSuppressionDirective(raw string) (Suppression, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "<!--") || !strings.HasSuffix(trimmed, "-->") {
		return Suppression{}, fmt.Errorf("suppression directive must be an HTML comment")
	}
	inner := strings.TrimSpace(trimmed[len("<!--") : len(trimmed)-len("-->")])
	if len(inner) < len(suppressionPrefix) || !strings.EqualFold(inner[:len(suppressionPrefix)], suppressionPrefix) {
		return Suppression{}, fmt.Errorf("suppression directive must start with %q", suppressionPrefix)
	}
	remaining := strings.TrimSpace(inner[len(suppressionPrefix):])
	if remaining == "" {
		return Suppression{}, fmt.Errorf("suppression directive is missing a rule ID")
	}
	rule, remaining := takeToken(remaining)
	if !validRuleToken(rule) {
		return Suppression{}, fmt.Errorf("suppression directive has invalid rule ID %q", rule)
	}
	attributes, err := parseSuppressionAttributes(remaining)
	if err != nil {
		return Suppression{}, err
	}
	for _, key := range []string{"reason", "owner", "review"} {
		if strings.TrimSpace(attributes[key]) == "" {
			return Suppression{}, fmt.Errorf("suppression directive is missing %s", key)
		}
	}
	return Suppression{
		RuleID:      RuleID(rule),
		Reason:      attributes["reason"],
		Owner:       attributes["owner"],
		Review:      attributes["review"],
		StartOffset: -1,
		EndOffset:   -1,
	}, nil
}

func parseInlineDirectives(span Span, policy Policy) ([]directiveOccurrence, []Finding) {
	var directives []directiveOccurrence
	var findings []Finding
	for search := 0; search < len(span.Text); {
		start := strings.Index(span.Text[search:], "<!--")
		if start < 0 {
			break
		}
		start += search
		endRelative := strings.Index(span.Text[start+len("<!--"):], "-->")
		if endRelative < 0 {
			findings = append(findings, suppressionFinding(span, "unterminated suppression directive", start, len(span.Text)))
			break
		}
		end := start + len("<!--") + endRelative + len("-->")
		raw := span.Text[start:end]
		inner := strings.ToLower(raw)
		if strings.Contains(inner, "prosecheck:") {
			suppression, err := ParseSuppressionDirective(raw)
			if err != nil {
				findings = append(findings, suppressionFinding(span, err.Error(), start, end))
			} else {
				directives = append(directives, directiveOccurrence{
					start:       start,
					end:         end,
					suppression: suppression,
				})
			}
		}
		search = end
	}
	return directives, findings
}

func validateSuppressions(span Span, policy Policy, directives []directiveOccurrence) ([]Suppression, []Finding) {
	suppressions := append([]Suppression(nil), span.Suppressions...)
	for _, directive := range directives {
		suppression := directive.suppression
		suppression.StartOffset = directive.end
		suppression.EndOffset = len(span.Text)
		suppressions = append(suppressions, suppression)
	}

	valid := make([]Suppression, 0, len(suppressions))
	var findings []Finding
	for _, suppression := range suppressions {
		if err := validateSuppression(suppression, policy, len(span.Text)); err != nil {
			start := suppression.StartOffset
			if start < 0 || start >= len(span.Text) {
				start = 0
			}
			end := suppression.EndOffset
			if end <= start || end > len(span.Text) {
				end = minInt(len(span.Text), start+1)
			}
			findings = append(findings, suppressionFinding(span, err.Error(), start, end))
			continue
		}
		valid = append(valid, suppression)
	}
	return valid, findings
}

func validateSuppression(suppression Suppression, policy Policy, textLength int) error {
	if _, ok := policy.rule(suppression.RuleID); !ok {
		return fmt.Errorf("suppression names unknown or unsupported rule %q", suppression.RuleID)
	}
	if suppression.StartOffset < 0 || suppression.EndOffset <= suppression.StartOffset || suppression.EndOffset > textLength {
		return fmt.Errorf("suppression for %s must cover one bounded source span", suppression.RuleID)
	}
	if strings.TrimSpace(suppression.Reason) == "" || strings.TrimSpace(suppression.Owner) == "" || strings.TrimSpace(suppression.Review) == "" {
		return fmt.Errorf("suppression for %s must include reason, owner, and review", suppression.RuleID)
	}
	return nil
}

func directiveRanges(directives []directiveOccurrence) []TextRange {
	ranges := make([]TextRange, 0, len(directives))
	for _, directive := range directives {
		ranges = append(ranges, TextRange{Start: directive.start, End: directive.end})
	}
	return ranges
}

func suppressed(item candidate, suppressions []Suppression) bool {
	for _, suppression := range suppressions {
		if suppression.RuleID == item.ruleID && item.start >= suppression.StartOffset && item.end <= suppression.EndOffset {
			return true
		}
	}
	return false
}

func suppressionFinding(span Span, message string, start, end int) Finding {
	item := candidate{
		ruleID: RuleSuppression, class: span.Class, start: start, end: end,
		excerpt:  span.Text[start:end],
		guidance: "Use one known rule, a bounded span, and a reason, owner, and review point.",
	}
	if strings.TrimSpace(item.excerpt) == "" {
		item.excerpt = message
	}
	return findingFromCandidate(span, item)
}

func parseSuppressionAttributes(remaining string) (map[string]string, error) {
	attributes := make(map[string]string)
	remaining = strings.TrimSpace(remaining)
	for remaining != "" {
		key, afterKey := takeToken(remaining)
		if key == "" || afterKey == remaining || !strings.HasPrefix(afterKey, "=") {
			return nil, fmt.Errorf("suppression attribute must use key=\"value\" syntax")
		}
		if _, exists := attributes[key]; exists {
			return nil, fmt.Errorf("suppression attribute %q is repeated", key)
		}
		valueText := strings.TrimSpace(afterKey[1:])
		if !strings.HasPrefix(valueText, "\"") {
			return nil, fmt.Errorf("suppression attribute %q must use a quoted value", key)
		}
		endQuote := strings.IndexByte(valueText[1:], '"')
		if endQuote < 0 {
			return nil, fmt.Errorf("suppression attribute %q has an unterminated value", key)
		}
		endQuote++
		value := valueText[1:endQuote]
		if key != "reason" && key != "owner" && key != "review" {
			return nil, fmt.Errorf("suppression attribute %q is unknown", key)
		}
		attributes[key] = value
		remaining = strings.TrimSpace(valueText[endQuote+1:])
	}
	return attributes, nil
}

func takeToken(value string) (string, string) {
	value = strings.TrimSpace(value)
	for index, runeValue := range value {
		if runeValue == '=' || runeValue == ' ' || runeValue == '\t' || runeValue == '\r' || runeValue == '\n' {
			return value[:index], value[index:]
		}
	}
	return value, ""
}

func validRuleToken(value string) bool {
	if value == "" {
		return false
	}
	for _, runeValue := range value {
		if (runeValue < 'A' || runeValue > 'Z') && runeValue != '-' && (runeValue < '0' || runeValue > '9') {
			return false
		}
	}
	return true
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
