package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	publicFactoryWorkstationKindStandard = "STANDARD"
	publicFactoryWorkstationKindRepeater = "REPEATER"
	publicFactoryWorkstationKindCron     = "CRON"
)

func normalizePublicWorkstationKind(value string, acceptInternal bool, preserveUnknown bool) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "":
		return ""
	case publicFactoryWorkstationKindStandard:
		return publicFactoryWorkstationKindStandard
	case publicFactoryWorkstationKindRepeater:
		return publicFactoryWorkstationKindRepeater
	case publicFactoryWorkstationKindCron:
		return publicFactoryWorkstationKindCron
	default:
		if acceptInternal {
			switch trimmed {
			case string(WorkstationKindStandard):
				return publicFactoryWorkstationKindStandard
			case string(WorkstationKindRepeater):
				return publicFactoryWorkstationKindRepeater
			case string(WorkstationKindCron):
				return publicFactoryWorkstationKindCron
			}
		}
		if preserveUnknown {
			return trimmed
		}
		return ""
	}
}

// CanonicalPublicWorkstationKind returns the public factory-config enum value
// for a runtime workstation kind while preserving unknown values verbatim.
func CanonicalPublicWorkstationKind(kind WorkstationKind) string {
	return normalizePublicWorkstationKind(string(kind), true, true)
}

// StrictPublicWorkstationKind canonicalizes supported public workstation kinds and rejects unknown values.
func StrictPublicWorkstationKind(value string) string {
	return normalizePublicWorkstationKind(value, false, false)
}

// GeneratedPublicWorkstationKind returns the generated workstation kind enum.
func GeneratedPublicWorkstationKind(kind WorkstationKind) factoryapi.WorkstationKind {
	return factoryapi.WorkstationKind(CanonicalPublicWorkstationKind(kind))
}

// GeneratedPublicWorkstationKindPtr returns the generated workstation kind enum when non-empty.
func GeneratedPublicWorkstationKindPtr(kind WorkstationKind) *factoryapi.WorkstationKind {
	return generatedPublicFactoryEnumPtr(string(kind), func(value string) factoryapi.WorkstationKind {
		return GeneratedPublicWorkstationKind(WorkstationKind(value))
	})
}
