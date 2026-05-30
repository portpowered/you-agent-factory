package validation

import (
	"fmt"
	"sort"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// CanonicalTargetSignatures returns sorted stable signatures for cross-path equivalence.
func CanonicalTargetSignatures(targets []Target) []string {
	signatures := make([]string, 0, len(targets))
	for _, target := range targets {
		signatures = append(signatures, canonicalTargetSignature(
			target.Code,
			string(target.Subject.Type),
			target.Subject.ID,
			string(target.Subject.Location),
		))
	}
	sort.Strings(signatures)
	return signatures
}

// CanonicalAPITargetSignatures returns sorted signatures for API validation targets.
func CanonicalAPITargetSignatures(targets []factoryapi.FactoryValidationTarget) []string {
	signatures := make([]string, 0, len(targets))
	for _, target := range targets {
		signatures = append(signatures, canonicalTargetSignature(
			target.Code,
			string(target.Subject.Type),
			target.Subject.Id,
			string(target.Subject.Location),
		))
	}
	sort.Strings(signatures)
	return signatures
}

// EquivalentCanonicalTargetSignatures reports whether two signature lists match.
func EquivalentCanonicalTargetSignatures(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	for index := range want {
		if want[index] != got[index] {
			return false
		}
	}
	return true
}

func canonicalTargetSignature(code, subjectType, subjectID, location string) string {
	return fmt.Sprintf("%s|%s|%s|%s", code, subjectType, subjectID, location)
}
