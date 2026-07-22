package orchestratorcontract

import "fmt"

// ValidateCapability checks one host capability against the effective policy.
// It returns a denial diagnostic when the capability is not permitted.
func ValidateCapability(policy EffectivePolicy, capability Capability) *Diagnostic {
	if diagnostic := readOnlyCapabilityDenial(policy, capability); diagnostic != nil {
		return diagnostic
	}
	return nil
}

// DeniedCapabilitiesForReadOnly returns the denied capability diagnostics for
// one read-only effective policy.
func DeniedCapabilitiesForReadOnly(policy EffectivePolicy) []Diagnostic {
	if policy.Mode != ModeReadOnly {
		return nil
	}
	capabilities := []Capability{
		CapabilityWorkspaceWrite,
		CapabilityFilesystemWrite,
		CapabilityShellProcess,
		CapabilityNetwork,
		CapabilityConnectors,
		CapabilityDangerFullAccess,
	}
	denied := make([]Diagnostic, 0, len(capabilities))
	for _, capability := range capabilities {
		if diagnostic := ValidateCapability(policy, capability); diagnostic != nil {
			denied = append(denied, *diagnostic)
		}
	}
	return denied
}

func readOnlyCapabilityDenial(policy EffectivePolicy, capability Capability) *Diagnostic {
	if policy.Mode != ModeReadOnly {
		return nil
	}
	switch capability {
	case CapabilityWorkspaceWrite:
		return deniedDiagnostic(capability, "workspace-write workers are denied when policy.mode is READ_ONLY")
	case CapabilityFilesystemWrite:
		return deniedDiagnostic(capability, "direct workflow filesystem writes are denied when policy.mode is READ_ONLY")
	case CapabilityShellProcess:
		return deniedDiagnostic(capability, "direct shell/process access is denied when policy.mode is READ_ONLY")
	case CapabilityNetwork:
		if policy.AllowNetwork {
			return nil
		}
		return deniedDiagnostic(capability, "direct network access is denied when policy.mode is READ_ONLY")
	case CapabilityConnectors:
		if policy.AllowConnectors {
			return nil
		}
		return deniedDiagnostic(capability, "connectors are denied when policy.mode is READ_ONLY")
	case CapabilityDangerFullAccess:
		if policy.AllowDangerFullAccess {
			return nil
		}
		return deniedDiagnostic(capability, "danger-full-access is denied when policy.mode is READ_ONLY")
	default:
		return nil
	}
}

func deniedDiagnostic(capability Capability, message string) *Diagnostic {
	return &Diagnostic{
		Code:    CodeDeniedCapability,
		Message: fmt.Sprintf("%s (%s)", message, capability),
	}
}
