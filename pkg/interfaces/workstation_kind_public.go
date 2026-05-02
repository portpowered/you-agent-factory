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

// CanonicalPublicWorkstationKind returns the public factory-config enum value
// for a runtime workstation kind while preserving unknown values verbatim.
func CanonicalPublicWorkstationKind(kind WorkstationKind) string {
	switch strings.TrimSpace(string(kind)) {
	case "":
		return ""
	case string(WorkstationKindStandard), publicFactoryWorkstationKindStandard:
		return publicFactoryWorkstationKindStandard
	case string(WorkstationKindRepeater), publicFactoryWorkstationKindRepeater:
		return publicFactoryWorkstationKindRepeater
	case string(WorkstationKindCron), publicFactoryWorkstationKindCron:
		return publicFactoryWorkstationKindCron
	default:
		return strings.TrimSpace(string(kind))
	}
}

// GeneratedPublicWorkstationKind returns the generated workstation kind enum.
func GeneratedPublicWorkstationKind(kind WorkstationKind) factoryapi.WorkstationKind {
	return factoryapi.WorkstationKind(CanonicalPublicWorkstationKind(kind))
}

// GeneratedPublicWorkstationKindPtr returns the generated workstation kind enum when non-empty.
func GeneratedPublicWorkstationKindPtr(kind WorkstationKind) *factoryapi.WorkstationKind {
	if strings.TrimSpace(string(kind)) == "" {
		return nil
	}
	enumValue := GeneratedPublicWorkstationKind(kind)
	return &enumValue
}
