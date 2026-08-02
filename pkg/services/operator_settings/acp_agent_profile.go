package operatorsettings

import (
	"errors"
	"fmt"
	"strings"
)

// ACPFactoryTargetNamespace is the required unversioned namespace prefix for
// every ACP Agent profile target reference. Factory Definitions owns
// enumeration and canonical reference resolution; Operator Settings validates
// local shape only.
const ACPFactoryTargetNamespace = "factory:"

// DefaultACPAgentProfileTarget is the safe Factory Builder target used when an
// operator document has no authored ACP Agent profile.
const DefaultACPAgentProfileTarget = "factory:@you/factory-builder"

// ErrACPAgentProfileInvalid reports that an authored ACP Agent profile fails
// local shape or consistency validation.
var ErrACPAgentProfileInvalid = errors.New("ACP Agent profile is invalid")

// ACPAgentProfile is a detached Operator Settings value describing one
// operator's default Factory target and ordered Factory-target allowlist for
// L1 ACP sessions. Target references are unversioned factory:<ref> strings;
// Operator Settings never adds a version or digest field.
type ACPAgentProfile struct {
	DefaultTarget  string
	AllowedTargets []string
}

// Clone returns a detached copy whose AllowedTargets slice does not alias the
// receiver's backing array.
func (profile ACPAgentProfile) Clone() ACPAgentProfile {
	return ACPAgentProfile{
		DefaultTarget:  profile.DefaultTarget,
		AllowedTargets: append([]string(nil), profile.AllowedTargets...),
	}
}

// Normalize trims the default target and every allowlist entry, preserves
// authored allowlist order, and validates local shape and consistency. It
// rejects a blank default, blank allowlist entries, references outside the
// factory: namespace, an empty allowlist, duplicate entries after
// normalization, and a default absent from the normalized allowlist.
func (profile ACPAgentProfile) Normalize() (ACPAgentProfile, error) {
	defaultTarget := strings.TrimSpace(profile.DefaultTarget)
	if defaultTarget == "" {
		return ACPAgentProfile{}, fmt.Errorf("%w: default target must not be blank", ErrACPAgentProfileInvalid)
	}
	if !isACPFactoryTargetReference(defaultTarget) {
		return ACPAgentProfile{}, fmt.Errorf(
			"%w: default target %q must use the factory: namespace", ErrACPAgentProfileInvalid, defaultTarget,
		)
	}
	if len(profile.AllowedTargets) == 0 {
		return ACPAgentProfile{}, fmt.Errorf("%w: allowedTargets must not be empty", ErrACPAgentProfileInvalid)
	}

	normalizedAllowed := make([]string, len(profile.AllowedTargets))
	seen := make(map[string]struct{}, len(profile.AllowedTargets))
	for index, target := range profile.AllowedTargets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			return ACPAgentProfile{}, fmt.Errorf(
				"%w: allowedTargets[%d] must not be blank", ErrACPAgentProfileInvalid, index,
			)
		}
		if !isACPFactoryTargetReference(trimmed) {
			return ACPAgentProfile{}, fmt.Errorf(
				"%w: allowedTargets[%d] %q must use the factory: namespace", ErrACPAgentProfileInvalid, index, trimmed,
			)
		}
		if _, exists := seen[trimmed]; exists {
			return ACPAgentProfile{}, fmt.Errorf(
				"%w: allowedTargets[%d] %q is duplicated", ErrACPAgentProfileInvalid, index, trimmed,
			)
		}
		seen[trimmed] = struct{}{}
		normalizedAllowed[index] = trimmed
	}

	if _, ok := seen[defaultTarget]; !ok {
		return ACPAgentProfile{}, fmt.Errorf(
			"%w: default target %q must be present in allowedTargets", ErrACPAgentProfileInvalid, defaultTarget,
		)
	}

	return ACPAgentProfile{DefaultTarget: defaultTarget, AllowedTargets: normalizedAllowed}, nil
}

// DefaultACPAgentProfile returns the detached safe Factory Builder profile
// used when an operator document has no authored profile.
func DefaultACPAgentProfile() ACPAgentProfile {
	return ACPAgentProfile{
		DefaultTarget:  DefaultACPAgentProfileTarget,
		AllowedTargets: []string{DefaultACPAgentProfileTarget},
	}
}

func isACPFactoryTargetReference(value string) bool {
	return strings.HasPrefix(value, ACPFactoryTargetNamespace) && len(value) > len(ACPFactoryTargetNamespace)
}
