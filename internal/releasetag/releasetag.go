package releasetag

import (
	"fmt"
	"regexp"
	"strings"
)

const semverPatternText = `^v\d+\.\d+\.\d+$`

var semverPattern = regexp.MustCompile(semverPatternText)

func RequireSemver(tag string) (string, error) {
	normalized := strings.TrimSpace(tag)
	if normalized == "" {
		return "", fmt.Errorf("release tag is required and must match vMAJOR.MINOR.PATCH")
	}
	if !semverPattern.MatchString(normalized) {
		return "", fmt.Errorf("release tag %q must match vMAJOR.MINOR.PATCH", normalized)
	}
	return normalized, nil
}

func FilterSemver(tags []string) []string {
	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if normalized, err := RequireSemver(tag); err == nil {
			filtered = append(filtered, normalized)
		}
	}
	return filtered
}
