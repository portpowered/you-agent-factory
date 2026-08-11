package providercatalog

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type capabilityFact struct {
	ref          string
	support      string
	transport    string
	condition    string
	evidenceRefs []string
	modality     string
	output       bool
	toolMediated bool
}

type capabilityEvidence struct {
	ids     map[string]struct{}
	facts   map[string][]string
	present bool
}

type harnessFacts struct {
	kind             string
	resourceDelivery string
	facts            []capabilityFact
}

func validateManifestCapabilityFacts(manifest map[string]any) error {
	evidence, err := indexCapabilityEvidence(manifest)
	if err != nil {
		return err
	}
	// Existing manifests remain readable during the additive migration; once a
	// manifest opts into any expanded fact, every non-unknown claim is evidenced.
	requireEvidence := evidence.present || hasExpandedCapabilityMetadata(manifest)
	facts, err := collectManifestCapabilityFacts(manifest)
	if err != nil {
		return err
	}
	if err := validateCapabilityFactClosure(manifest, facts, evidence, requireEvidence); err != nil {
		return err
	}
	return rejectDirectToolOutputContradictions(manifest, facts)
}

func collectManifestCapabilityFacts(manifest map[string]any) ([]capabilityFact, error) {
	if rawPosture, ok := manifest["modelCatalogPosture"]; ok {
		posture, _ := rawPosture.(string)
		if !isModelCatalogPosture(posture) {
			return nil, fmt.Errorf("provider %q: unknown model catalog posture %q", manifest["id"], posture)
		}
	}
	harness, err := collectHarnessFacts(manifest)
	if err != nil {
		return nil, err
	}
	facts := append([]capabilityFact(nil), harness.facts...)
	routes, err := collectHarnessRouteFacts(manifest, harness)
	if err != nil {
		return nil, err
	}
	facts = append(facts, routes...)
	models, err := collectModelCapabilityFacts(manifest)
	if err != nil {
		return nil, err
	}
	facts = append(facts, models...)
	tools, err := collectToolCapabilityFacts(manifest)
	if err != nil {
		return nil, err
	}
	return append(facts, tools...), nil
}

func collectHarnessFacts(manifest map[string]any) (harnessFacts, error) {
	result := harnessFacts{}
	rawHarness, ok := manifest["harness"]
	if !ok {
		if _, hasRoutes := manifest["harnessRoutes"]; hasRoutes {
			return result, fmt.Errorf("provider %q: harnessRoutes require harness metadata", manifest["id"])
		}
		return result, nil
	}
	harness, ok := rawHarness.(map[string]any)
	if !ok {
		return result, fmt.Errorf("provider %q: harness must be an object", manifest["id"])
	}
	result.kind, _ = harness["kind"].(string)
	if result.kind != "native_cli" && result.kind != "acp" {
		return result, fmt.Errorf("provider %q: unknown harness kind %q", manifest["id"], result.kind)
	}
	rawACP, hasACP := harness["acpSupport"]
	if !hasACP {
		if result.kind == "acp" {
			return result, fmt.Errorf("provider %q: acp harness requires typed acpSupport", manifest["id"])
		}
		return result, nil
	}
	if result.kind != "acp" {
		return result, fmt.Errorf("provider %q: acpSupport is only valid for an acp harness", manifest["id"])
	}
	acp, ok := rawACP.(map[string]any)
	if !ok {
		return result, fmt.Errorf("provider %q: harness acpSupport must be an object", manifest["id"])
	}
	facts, resourceDelivery, err := collectACPCapabilityFacts(manifest, acp)
	if err != nil {
		return result, err
	}
	result.facts = facts
	result.resourceDelivery = resourceDelivery
	return result, nil
}

func collectACPCapabilityFacts(manifest map[string]any, acp map[string]any) ([]capabilityFact, string, error) {
	providerID := manifest["id"]
	support, _ := acp["support"].(string)
	if support == "" {
		return nil, "", fmt.Errorf("provider %q: acp harness requires typed acpSupport", providerID)
	}
	resourceDelivery, err := readACPResourceDelivery(acp)
	if err != nil {
		return nil, "", fmt.Errorf("provider %q: %w", providerID, err)
	}
	supportValue := cloneMap(acp)
	if resourceDelivery == "conditional" && support != "conditional" {
		delete(supportValue, "condition")
	}
	supportFact, err := capabilityFactFromSupport("harness/acp", supportValue, "support", "", "", "acp support")
	if err != nil {
		return nil, "", fmt.Errorf("provider %q: %w", providerID, err)
	}
	facts := []capabilityFact{supportFact}
	if resourceDelivery == "" {
		return facts, "", nil
	}
	if err := validateACPResourceDelivery(support, resourceDelivery); err != nil {
		return nil, "", fmt.Errorf("provider %q: %w", providerID, err)
	}
	resourceSupport := map[string]string{
		"implemented": "supported",
		"unsupported": "unsupported",
		"conditional": "conditional",
		"unknown":     "unknown",
	}[resourceDelivery]
	resourceValue := map[string]any{
		"support": resourceSupport,
	}
	if rawEvidenceRefs, ok := acp["evidenceRefs"]; ok {
		resourceValue["evidenceRefs"] = rawEvidenceRefs
	}
	if resourceDelivery == "conditional" {
		resourceValue["condition"] = acp["condition"]
	}
	resourceFact, err := capabilityFactFromSupport(
		"harness/acp/resource_delivery", resourceValue, "support", "", "", "ACP resource delivery",
	)
	if err != nil {
		return nil, "", fmt.Errorf("provider %q: %w", providerID, err)
	}
	return append(facts, resourceFact), resourceDelivery, nil
}

func readACPResourceDelivery(acp map[string]any) (string, error) {
	rawDelivery, ok := acp["resourceDelivery"]
	if !ok {
		return "", nil
	}
	delivery, ok := rawDelivery.(string)
	if !ok {
		return "", fmt.Errorf("resourceDelivery must be a string")
	}
	if delivery != "implemented" && delivery != "unsupported" && delivery != "conditional" && delivery != "unknown" {
		return "", fmt.Errorf("unknown ACP resource delivery %q", delivery)
	}
	return delivery, nil
}

func collectHarnessRouteFacts(manifest map[string]any, harness harnessFacts) ([]capabilityFact, error) {
	rawRoutes, ok := manifest["harnessRoutes"]
	if !ok {
		return nil, nil
	}
	routes, ok := rawRoutes.([]any)
	if !ok {
		return nil, fmt.Errorf("provider %q: harnessRoutes must be an array", manifest["id"])
	}
	facts := make([]capabilityFact, 0, len(routes))
	for _, rawRoute := range routes {
		route, _ := rawRoute.(map[string]any)
		direction, _ := route["direction"].(string)
		modality, _ := route["modality"].(string)
		fact, err := modalityFact("harness/"+direction+"/"+modality, rawRoute, false)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
		if err := validateHarnessRouteTransport(fact, harness); err != nil {
			return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func validateHarnessRouteTransport(fact capabilityFact, harness harnessFacts) error {
	if fact.transport != "acp_resource" {
		return nil
	}
	if harness.kind != "acp" {
		return fmt.Errorf("capability fact %q: acp_resource transport requires an acp harness", fact.ref)
	}
	if harness.resourceDelivery != "implemented" && harness.resourceDelivery != "conditional" {
		return fmt.Errorf("capability fact %q: acp_resource transport requires implemented or conditional resource delivery", fact.ref)
	}
	return nil
}

func collectModelCapabilityFacts(manifest map[string]any) ([]capabilityFact, error) {
	rawModels, ok := manifest["models"]
	if !ok {
		return nil, nil
	}
	models, ok := rawModels.([]any)
	if !ok {
		return nil, fmt.Errorf("provider %q: models must be an array", manifest["id"])
	}
	facts := make([]capabilityFact, 0)
	for _, rawModel := range models {
		model, ok := rawModel.(map[string]any)
		if !ok {
			continue
		}
		modelID, _ := model["id"].(string)
		rawModalities, ok := model["modalities"].([]any)
		if !ok {
			continue
		}
		for _, rawModality := range rawModalities {
			modality, ok := rawModality.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := modality["modality"].(string)
			direction, _ := modality["direction"].(string)
			fact, err := modalityFact("model/"+modelID+"/"+direction+"/"+kind, modality, true)
			if err != nil {
				return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
			}
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

func collectToolCapabilityFacts(manifest map[string]any) ([]capabilityFact, error) {
	rawTools, ok := manifest["tools"]
	if !ok {
		return nil, nil
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return nil, fmt.Errorf("provider %q: tools must be an array", manifest["id"])
	}
	facts := make([]capabilityFact, 0)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		fact, err := capabilityFactFromSupport("tool/"+name, tool, "support", "", "", "tool")
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
		if err := validateToolAvailability(tool, name); err != nil {
			return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
		facts = append(facts, fact)
		outputs, err := collectToolOutputFacts(manifest, name, tool)
		if err != nil {
			return nil, err
		}
		facts = append(facts, outputs...)
	}
	return facts, nil
}

func collectToolOutputFacts(manifest map[string]any, name string, tool map[string]any) ([]capabilityFact, error) {
	rawOutputs, ok := tool["outputModalities"]
	if !ok {
		return nil, nil
	}
	outputs, ok := rawOutputs.([]any)
	if !ok {
		return nil, fmt.Errorf("provider %q tool %q: outputModalities must be an array", manifest["id"], name)
	}
	facts := make([]capabilityFact, 0, len(outputs))
	for _, rawOutput := range outputs {
		output, ok := rawOutput.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("provider %q tool %q: output modality must be an object", manifest["id"], name)
		}
		modality, _ := output["modality"].(string)
		ref := "tool/" + name + "/output/" + modality
		fact, err := capabilityFactFromSupport(ref, output, "support", "output", modality, "tool output")
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
		if fact.support != "unknown" && fact.transport != "tool_mediated" {
			return nil, fmt.Errorf("provider %q tool %q output %q: supported tool output must use tool_mediated transport", manifest["id"], name, modality)
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func validateCapabilityFactClosure(manifest map[string]any, facts []capabilityFact, evidence capabilityEvidence, requireEvidence bool) error {
	knownFacts := make(map[string]struct{}, len(facts)+1)
	if _, ok := manifest["modelCatalogPosture"]; ok {
		knownFacts["model_catalog"] = struct{}{}
	}
	for _, fact := range facts {
		if _, duplicate := knownFacts[fact.ref]; duplicate {
			return fmt.Errorf("provider %q: duplicate capability fact %q", manifest["id"], fact.ref)
		}
		knownFacts[fact.ref] = struct{}{}
		if err := validateFactEvidence(fact, evidence, requireEvidence); err != nil {
			return fmt.Errorf("provider %q: %w", manifest["id"], err)
		}
	}
	for evidenceID, factRefs := range evidence.facts {
		for _, factRef := range factRefs {
			if _, exists := knownFacts[factRef]; !exists {
				return fmt.Errorf("provider %q evidence %q references out-of-bounds fact %q", manifest["id"], evidenceID, factRef)
			}
		}
	}
	return nil
}

func hasExpandedCapabilityMetadata(manifest map[string]any) bool {
	for _, field := range []string{"harness", "modelCatalogPosture", "harnessRoutes", "evidence"} {
		if _, ok := manifest[field]; ok {
			return true
		}
	}
	for _, rawTool := range sliceValue(manifest["tools"]) {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"availability", "defaultEnabled", "outputModalities"} {
			if _, ok := tool[field]; ok {
				return true
			}
		}
		for _, rawOutput := range sliceValue(tool["outputModalities"]) {
			if hasExpandedModalityFact(rawOutput) {
				return true
			}
		}
	}
	for _, rawModel := range sliceValue(manifest["models"]) {
		model, ok := rawModel.(map[string]any)
		if !ok {
			continue
		}
		for _, rawModality := range sliceValue(model["modalities"]) {
			if hasExpandedModalityFact(rawModality) {
				return true
			}
		}
	}
	return false
}

func hasExpandedModalityFact(raw any) bool {
	modality, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	for _, field := range []string{"condition", "evidenceRefs", "mediaConstraints"} {
		if _, exists := modality[field]; exists {
			return true
		}
	}
	support, _ := modality["support"].(string)
	if support == "conditional" || support == "unknown" {
		return true
	}
	transport, _ := modality["transport"].(string)
	return transport == "acp_resource" || transport == "tool_mediated"
}

func cloneMap(value map[string]any) map[string]any {
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func indexCapabilityEvidence(manifest map[string]any) (capabilityEvidence, error) {
	result := capabilityEvidence{ids: make(map[string]struct{}), facts: make(map[string][]string)}
	rawEvidence, present := manifest["evidence"]
	if !present {
		return result, nil
	}
	result.present = true
	evidence, ok := rawEvidence.([]any)
	if !ok {
		return result, fmt.Errorf("provider %q: evidence must be an array", manifest["id"])
	}
	for index, rawRecord := range evidence {
		record, ok := rawRecord.(map[string]any)
		if !ok {
			return result, fmt.Errorf("provider %q evidence %d is not an object", manifest["id"], index)
		}
		id, _ := record["id"].(string)
		if strings.TrimSpace(id) == "" {
			return result, fmt.Errorf("provider %q evidence %d has an empty id", manifest["id"], index)
		}
		if _, duplicate := result.ids[id]; duplicate {
			return result, fmt.Errorf("provider %q: duplicate evidence id %q", manifest["id"], id)
		}
		kind, _ := record["kind"].(string)
		if !isEvidenceKind(kind) {
			return result, fmt.Errorf("provider %q evidence %q: unknown kind %q", manifest["id"], id, kind)
		}
		verifiedOn, _ := record["verifiedOn"].(string)
		if _, err := time.Parse("2006-01-02", verifiedOn); err != nil {
			return result, fmt.Errorf("provider %q evidence %q: verifiedOn must be a calendar date", manifest["id"], id)
		}
		factRefs, err := readStringArray(record, "factRefs")
		if err != nil {
			return result, fmt.Errorf("provider %q evidence %q: %w", manifest["id"], id, err)
		}
		result.ids[id] = struct{}{}
		result.facts[id] = factRefs
	}
	return result, nil
}

func modalityFact(ref string, raw any, directModel bool) (capabilityFact, error) {
	modality, ok := raw.(map[string]any)
	if !ok {
		return capabilityFact{}, fmt.Errorf("capability fact %q is not an object", ref)
	}
	direction, _ := modality["direction"].(string)
	kind, _ := modality["modality"].(string)
	transport, _ := modality["transport"].(string)
	fact, err := capabilityFactFromSupport(ref, modality, "support", direction, kind, "modality")
	if err != nil {
		return capabilityFact{}, err
	}
	if directModel && direction == "output" && transport == "tool_mediated" {
		return capabilityFact{}, fmt.Errorf("capability fact %q: direct model output cannot use tool_mediated transport", ref)
	}
	return fact, nil
}

func capabilityFactFromSupport(ref string, value map[string]any, supportField, direction, modality, label string) (capabilityFact, error) {
	support, _ := value[supportField].(string)
	transport, _ := value["transport"].(string)
	condition, _ := value["condition"].(string)
	if !isCapabilitySupport(support) {
		return capabilityFact{}, fmt.Errorf("capability fact %q: unknown %s support %q", ref, label, support)
	}
	if transport != "" && !isModalityTransport(transport) {
		return capabilityFact{}, fmt.Errorf("capability fact %q: unknown transport %q", ref, transport)
	}
	if direction != "" || modality != "" || transport != "" {
		if err := validateModalityShape(ref, direction, modality); err != nil {
			return capabilityFact{}, err
		}
		if err := validateRouteSupport(ref, direction, modality, support, transport, value["condition"]); err != nil {
			return capabilityFact{}, err
		}
	} else if support == "conditional" && strings.TrimSpace(condition) == "" {
		return capabilityFact{}, fmt.Errorf("capability fact %q: conditional support requires a condition", ref)
	} else if support != "conditional" && strings.TrimSpace(condition) != "" {
		return capabilityFact{}, fmt.Errorf("capability fact %q: condition is only valid for conditional support", ref)
	}
	if rawConstraints, ok := value["mediaConstraints"]; ok && modality == "text" {
		if rawConstraints != nil {
			return capabilityFact{}, fmt.Errorf("capability fact %q: mediaConstraints are not applicable to text", ref)
		}
	}
	evidenceRefs, err := readStringArray(value, "evidenceRefs")
	if err != nil {
		return capabilityFact{}, fmt.Errorf("capability fact %q: %w", ref, err)
	}
	return capabilityFact{
		ref:          ref,
		support:      support,
		transport:    transport,
		condition:    condition,
		evidenceRefs: evidenceRefs,
		modality:     modality,
		output:       direction == "output" || label == "tool output",
		toolMediated: transport == "tool_mediated",
	}, nil
}

func validateModalityShape(ref, direction, modality string) error {
	if direction != "input" && direction != "output" {
		return fmt.Errorf("capability fact %q: unknown modality direction %q", ref, direction)
	}
	switch modality {
	case "text", "image", "audio", "video":
		return nil
	default:
		return fmt.Errorf("capability fact %q: unknown modality %q", ref, modality)
	}
}

func validateRouteSupport(ref, direction, modality, support, transport string, rawCondition any) error {
	condition, _ := rawCondition.(string)
	if support == "conditional" && strings.TrimSpace(condition) == "" {
		return fmt.Errorf("capability fact %q: conditional support requires a condition", ref)
	}
	if support != "conditional" && strings.TrimSpace(condition) != "" {
		return fmt.Errorf("capability fact %q: condition is only valid for conditional support", ref)
	}
	switch support {
	case "supported", "conditional":
		if transport == "" || transport == "none" {
			return fmt.Errorf("capability fact %q: %s support requires a non-none transport", ref, support)
		}
	case "unsupported":
		if transport != "none" {
			return fmt.Errorf("capability fact %q: unsupported support requires none transport", ref)
		}
	case "unknown":
		if transport != "" && transport != "none" {
			return fmt.Errorf("capability fact %q: unknown support cannot claim transport %q", ref, transport)
		}
	}
	if direction == "input" && transport == "tool_mediated" {
		return fmt.Errorf("capability fact %q: tool_mediated transport is output-only", ref)
	}
	return nil
}

func validateACPResourceDelivery(rawSupport any, delivery string) error {
	support, _ := rawSupport.(string)
	if delivery != "implemented" && delivery != "unsupported" && delivery != "conditional" && delivery != "unknown" {
		return fmt.Errorf("unknown ACP resource delivery %q", delivery)
	}
	if support == "unsupported" && delivery == "implemented" {
		return fmt.Errorf("ACP support cannot be unsupported while resource delivery is implemented")
	}
	return nil
}

func validateFactEvidence(fact capabilityFact, evidence capabilityEvidence, requireEvidence bool) error {
	if len(fact.evidenceRefs) == 0 {
		if requireEvidence && fact.support != "unknown" {
			return fmt.Errorf("capability fact %q: non-unknown support requires evidenceRefs", fact.ref)
		}
		return nil
	}
	if !evidence.present {
		return fmt.Errorf("capability fact %q: evidenceRefs require an evidence collection", fact.ref)
	}
	for _, evidenceRef := range fact.evidenceRefs {
		if _, exists := evidence.ids[evidenceRef]; !exists {
			return fmt.Errorf("capability fact %q references dangling evidence %q", fact.ref, evidenceRef)
		}
	}
	return nil
}

func validateToolAvailability(tool map[string]any, name string) error {
	rawAvailability, hasAvailability := tool["availability"]
	if !hasAvailability {
		return nil
	}
	availability, _ := rawAvailability.(string)
	if availability != "built_in" && availability != "optional" && availability != "operator_configured" && availability != "external" && availability != "unknown" {
		return fmt.Errorf("tool %q: unknown availability %q", name, availability)
	}
	if availability == "unknown" {
		if defaultEnabled, exists := tool["defaultEnabled"]; exists && defaultEnabled != nil {
			return fmt.Errorf("tool %q: unknown availability requires null defaultEnabled", name)
		}
	}
	return nil
}

func rejectDirectToolOutputContradictions(manifest map[string]any, facts []capabilityFact) error {
	direct := make(map[string]struct{})
	toolOutputs := make(map[string]struct{})
	for _, fact := range facts {
		if fact.support != "supported" && fact.support != "conditional" {
			continue
		}
		if strings.HasPrefix(fact.ref, "model/") && fact.output && !fact.toolMediated {
			direct[fact.modality] = struct{}{}
		}
		if strings.HasPrefix(fact.ref, "tool/") && strings.Contains(fact.ref, "/output/") {
			toolOutputs[fact.modality] = struct{}{}
		}
	}
	var contradictions []string
	for modality := range direct {
		if _, exists := toolOutputs[modality]; exists {
			contradictions = append(contradictions, modality)
		}
	}
	if len(contradictions) == 0 {
		return nil
	}
	sort.Strings(contradictions)
	return fmt.Errorf("provider %q: direct model output contradicts tool-mediated output for %s", manifest["id"], strings.Join(contradictions, ", "))
}

func isCapabilitySupport(value string) bool {
	switch value {
	case "supported", "unsupported", "conditional", "unknown":
		return true
	default:
		return false
	}
}

func isModalityTransport(value string) bool {
	switch value {
	case "inline", "file_path", "acp_resource", "tool_mediated", "none":
		return true
	default:
		return false
	}
}

func isEvidenceKind(value string) bool {
	switch value {
	case "primary_documentation", "protocol_probe", "conformance_fixture", "maintainer_assertion":
		return true
	default:
		return false
	}
}

func isModelCatalogPosture(value string) bool {
	switch value {
	case "exact", "runtime_discovered", "operator_selected", "unknown":
		return true
	default:
		return false
	}
}

func readStringArray(value map[string]any, field string) ([]string, error) {
	raw, ok := value[field]
	if !ok {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", field)
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("%s must contain non-empty strings", field)
		}
		if _, duplicate := seen[text]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate %q", field, text)
		}
		seen[text] = struct{}{}
		result = append(result, text)
	}
	return result, nil
}

func sliceValue(value any) []any {
	items, _ := value.([]any)
	return items
}
