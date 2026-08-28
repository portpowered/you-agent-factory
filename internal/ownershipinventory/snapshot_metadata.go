package ownershipinventory

func operatorSettingsRootGoNote(fileName string) string {
	switch fileName {
	case "acp_agent_profile.go":
		return "Peer-facing detached ACPAgentProfile value with normalization and validation for the L1 ACP default Factory target and allowlist."
	case "acp_agent_profile_test.go":
		return "Root-contract tests for ACPAgentProfile normalization, validation, and copy-isolation."
	case "acp_integrations.go":
		return "Peer-facing ACP integration projection and validation helpers retained at the thin Operator Settings root."
	case "acp_integrations_test.go":
		return "Root-contract tests for ACP integration projection and validation."
	case "backend_scope.go":
		return "Peer-facing backend-scope identity types and EnsureLocalBackendScope thin surface delegating to private implementation."
	case "config_document.go":
		return "Peer-facing ConfigDocument and ConfigDocumentService thin surface delegating to the private document subservice."
	case "construction_ports_contract.go":
		return "Thin construction-port aliases and func types for owner wire; canonical interface definitions live under operator_settings/internal."
	case "defaults_contract.go":
		return "Peer-facing operator defaults/config vocabulary retained at the thin Operator Settings root."
	case "defaults_resolution.go":
		return "Peer-facing defaults-resolution operations thin surface delegating to the private resolution subservice."
	case "del_set_proof_gate_test.go":
		return "DEL-SET story 005 gate proving structure, ownership, package-target, and root reconciliation verification pass after transitional public package deletion and baseline burn-down."
	case "doc.go":
		return "Package documentation for the committed thin Operator Settings root contract surface."
	case "document_contract.go":
		return "Peer-facing document request, result, value, and typed-error contracts."
	case "input_inventory_contract.go":
		return "Peer-facing operator-config input inventory types and ProjectInputInventory thin surface delegating to private implementation."
	case "packaged_root_shape_test.go":
		return "DEL-SET story 005 seal proving Operator Settings ships only wire/, internal/, transports/, and INV-retained test-only testdata/ package directories plus thin root contract files."
	case "resolution_contract.go":
		return "Peer-facing effective-resolution request, result, value, and typed-error contracts."
	case "root_contract_legacy_preservation_test.go":
		return "Legacy root-contract preservation proofs for transitional Settings vocabulary."
	case "root_wire_behavioral_boundary_test.go":
		return "Wire-constructed behavioral proof that published Service LoadDocument, ApplyDocumentUpdate, and ResolveEffective preserve observables and typed failures at the peer root boundary."
	case "service_contract.go":
		return "Peer Service interface for document operations and effective resolution."
	case "service_root_contract_invariants_test.go":
		return "Peer-shaped Service seal tests using only published root contracts."
	default:
		return ""
	}
}

func operatorSettingsTopLevelNote(directory, classification string) string {
	if directory == "testdata" && classification == OperatorSettingsTopLevelTestOnlyRetain {
		return "Owner-root shared Operator Settings contract and resolution fixtures for package tests; not a public product surface."
	}
	return ""
}

func providerSessionsRootGoNote(fileName string) string {
	switch fileName {
	case "contracts.go":
		return "Peer Service interface plus Inspect/Project/Details request, result, value, and typed-error contracts."
	case "doc.go":
		return "Package documentation for the committed thin Provider Sessions root contract surface."
	default:
		return ""
	}
}
