package operatorsettings

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ACPFactoryTargetNamespace is the required unversioned namespace prefix for
// every ACP Agent profile target reference. Factory Definitions owns
// enumeration and canonical reference resolution; Operator Settings validates
// local shape only.
const ACPFactoryTargetNamespace = "factory:"

// acpFactoryTargetReferencePattern is the local lexical grammar for the
// portion of a reference following the factory: namespace prefix: one or
// more lowercase kebab-case path segments separated by '/', with an optional
// leading '@' scope marker on the first segment (e.g. "@you/factory-builder",
// "local/software-auto"). It rejects blank segments, internal whitespace,
// control characters, and any shape outside this grammar; it never
// enumerates or resolves actual Factory targets.
var acpFactoryTargetReferencePattern = regexp.MustCompile(
	`^@?[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)+$`,
)

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
//
// An empty AllowedTargets means unrestricted: every installed Factory is
// selectable, and DefaultTarget merely selects which one starts current.
// AllowedTargets is therefore a purely opt-in restriction, and an operator
// document with no authored profile resolves to an unrestricted profile.
// Authoring an explicitly empty allowlist is rejected by Normalize, so
// omitting the field is the only way to express "no restriction".
type ACPAgentProfile struct {
	DefaultTarget  string
	AllowedTargets []string
}

// IsUnrestricted reports that the profile places no restriction on which
// installed Factory targets are selectable.
func (profile ACPAgentProfile) IsUnrestricted() bool {
	return len(profile.AllowedTargets) == 0
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
// factory: namespace, duplicate entries after normalization, and a default
// absent from a non-empty normalized allowlist.
//
// An empty allowlist normalizes to an unrestricted profile rather than an
// error. Callers that decode authored documents are responsible for rejecting
// an explicitly authored empty array before calling Normalize; the schema's
// minItems keeps that shape off the wire, so reaching here with no entries
// means the operator omitted the restriction entirely.
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
		return ACPAgentProfile{DefaultTarget: defaultTarget}, nil
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

// DefaultACPAgentProfile returns the detached profile used when an operator
// document has no authored profile: unrestricted, so every installed Factory
// is selectable, with Factory Builder starting as the current target.
func DefaultACPAgentProfile() ACPAgentProfile {
	return ACPAgentProfile{DefaultTarget: DefaultACPAgentProfileTarget}
}

func isACPFactoryTargetReference(value string) bool {
	if !strings.HasPrefix(value, ACPFactoryTargetNamespace) {
		return false
	}
	suffix := strings.TrimPrefix(value, ACPFactoryTargetNamespace)
	return acpFactoryTargetReferencePattern.MatchString(suffix)
}
