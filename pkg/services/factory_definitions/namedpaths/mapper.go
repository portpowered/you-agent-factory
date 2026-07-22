// Package namedfactorypath maps canonical named-factory display names to
// hierarchical on-disk directories under a factories root.
package namedpaths

import (
	"fmt"
	"path/filepath"
	"strings"
)

const scopedNamedFactoryPrefix = "@"

// MapDir maps a canonical named-factory display name to its hierarchical
// directory under factoriesRoot. Scoped names such as @you/goal map to
// validated path segments joined under the root; unscoped names map to a
// single segment.
func MapDir(factoriesRoot, name string) (string, error) {
	root := strings.TrimSpace(factoriesRoot)
	if root == "" {
		return "", fmt.Errorf("factory root is required")
	}
	segments, err := PathSegments(name)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, root)
	parts = append(parts, segments...)
	return filepath.Join(parts...), nil
}

// NameFromPathSegments reconstructs the canonical named-factory display name from
// validated hierarchical path segments returned by PathSegments.
func NameFromPathSegments(segments []string) (string, error) {
	if len(segments) == 0 {
		return "", fmt.Errorf("factory path segments are required")
	}
	switch len(segments) {
	case 1:
		segment := segments[0]
		if strings.HasPrefix(segment, scopedNamedFactoryPrefix) {
			return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
		}
		if _, err := safeSegment("factory", segment); err != nil {
			return "", err
		}
		return segment, nil
	case 2:
		if !strings.HasPrefix(segments[0], scopedNamedFactoryPrefix) {
			return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
		}
		name := segments[0] + "/" + segments[1]
		if err := validateScopedNamedFactoryName(name); err != nil {
			return "", err
		}
		return name, nil
	default:
		return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
	}
}

// PathSegments returns the validated hierarchical path segments for a canonical
// named-factory display name without joining them to a factories root.
func PathSegments(name string) ([]string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("factory name is required")
	}
	if strings.HasPrefix(trimmed, scopedNamedFactoryPrefix) {
		if err := validateScopedNamedFactoryName(trimmed); err != nil {
			return nil, err
		}
		parts := strings.Split(trimmed, "/")
		segments := make([]string, 0, len(parts))
		for i, part := range parts {
			kind := "factory"
			if i == 0 {
				kind = "factory scope"
			}
			segment, err := safeSegment(kind, part)
			if err != nil {
				return nil, err
			}
			segments = append(segments, segment)
		}
		return segments, nil
	}
	segment, err := safeSegment("factory", trimmed)
	if err != nil {
		return nil, err
	}
	return []string{segment}, nil
}

func validateScopedNamedFactoryName(name string) error {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] == scopedNamedFactoryPrefix || parts[1] == "" {
		return fmt.Errorf("factory name %q must be scoped as @scope/name", name)
	}
	scope := strings.TrimPrefix(parts[0], scopedNamedFactoryPrefix)
	if _, err := safeSegment("factory scope", scope); err != nil {
		return err
	}
	if _, err := safeSegment("factory", parts[1]); err != nil {
		return err
	}
	return nil
}

func safeSegment(kind, name string) (string, error) {
	segment := strings.TrimSpace(name)
	if segment == "" {
		return "", fmt.Errorf("%s name is required for factory config layout", kind)
	}
	if filepath.IsAbs(segment) || filepath.VolumeName(segment) != "" || strings.ContainsAny(segment, `/\`) {
		return "", fmt.Errorf("%s name %q cannot contain path separators", kind, name)
	}
	if segment == "." || segment == ".." {
		return "", fmt.Errorf("%s name %q is not a valid directory name", kind, name)
	}
	return segment, nil
}
