package namedfactorypath

import (
	"fmt"
	"net/url"
	"strings"
)

// LegacyLayoutSegment maps a canonical named-factory display name into the
// legacy single on-disk directory segment used under a factory root.
func LegacyLayoutSegment(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, scopedNamedFactoryPrefix) {
		if err := validateScopedNamedFactoryName(trimmed); err != nil {
			return "", err
		}
		segment := encodeScopedLegacyLayoutSegment(trimmed)
		if _, err := safeSegment("factory", segment); err != nil {
			return "", err
		}
		return segment, nil
	}
	return safeSegment("factory", trimmed)
}

func encodeScopedLegacyLayoutSegment(name string) string {
	return strings.NewReplacer("%", "%25", "/", "%2F").Replace(name)
}

// LegacyLayoutSegmentToName maps a legacy on-disk named-factory directory
// segment back to the canonical display name.
func LegacyLayoutSegmentToName(segment string) (string, error) {
	safeSegmentValue, err := safeSegment("factory", segment)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(safeSegmentValue, scopedNamedFactoryPrefix) {
		return safeSegmentValue, nil
	}

	name, err := url.PathUnescape(safeSegmentValue)
	if err != nil {
		return "", fmt.Errorf("decode factory layout segment %q: %w", segment, err)
	}
	encoded, err := LegacyLayoutSegment(name)
	if err != nil {
		return "", err
	}
	if encoded != safeSegmentValue {
		return "", fmt.Errorf("factory layout segment %q is not canonical for %q", segment, name)
	}
	return name, nil
}
