package providerpackages

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Validate discovers and validates all repository-owned provider package
// directories. Native manifests remain valid without harness.yaml; an ACP
// manifest must have a complete provider-local runtime definition.
func Validate(source fs.FS, profiles []RuntimeProfile) ([]Package, error) {
	entries, err := fs.ReadDir(source, ProviderRoot)
	if err != nil {
		return nil, fmt.Errorf("read provider packages %s: %w", ProviderRoot, err)
	}
	profileIDs := indexProfiles(profiles)
	packages := make([]Package, 0, len(entries))
	identities := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		provider, err := loadPackage(source, entry.Name(), profileIDs)
		if err != nil {
			return nil, err
		}
		if err := indexIdentity(provider, identities); err != nil {
			return nil, err
		}
		packages = append(packages, provider)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].ID < packages[j].ID })
	return packages, nil
}

func indexProfiles(profiles []RuntimeProfile) map[string]struct{} {
	result := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if id := strings.TrimSpace(profile.ID); id != "" {
			result[id] = struct{}{}
		}
	}
	return result
}

func loadPackage(source fs.FS, directory string, profileIDs map[string]struct{}) (Package, error) {
	manifestPath := fsPath(ProviderRoot, directory, "provider.yaml")
	manifest, err := readYAMLMap(source, manifestPath)
	if err != nil {
		return Package{}, fmt.Errorf("read provider package %q: %w", directory, err)
	}
	id, err := requiredString(manifest, "id", manifestPath)
	if err != nil {
		return Package{}, err
	}
	if id != directory {
		return Package{}, fmt.Errorf("%s: provider id %q must match its directory name", manifestPath, id)
	}
	aliases, err := stringArray(manifest, "aliases", manifestPath)
	if err != nil {
		return Package{}, err
	}
	provider := Package{
		Directory: fsPath(ProviderRoot, directory),
		ID:        id,
		Aliases:   aliases,
		Manifest:  manifest,
	}
	harnessKind, err := manifestHarnessKind(manifest, manifestPath)
	if err != nil {
		return Package{}, err
	}
	harnessPath := fsPath(ProviderRoot, directory, HarnessFile)
	harnessPayload, err := fs.ReadFile(source, harnessPath)
	if errors.Is(err, fs.ErrNotExist) {
		if harnessKind == "acp" {
			return Package{}, fmt.Errorf("%s: ACP provider package requires %s", manifestPath, HarnessFile)
		}
		return provider, nil
	}
	if err != nil {
		return Package{}, fmt.Errorf("read provider package %q: %w", directory, err)
	}
	if harnessKind != "acp" {
		return Package{}, fmt.Errorf("%s: %s is only valid for an acp provider", manifestPath, HarnessFile)
	}
	var harness HarnessPackage
	decoder := yaml.NewDecoder(strings.NewReader(string(harnessPayload)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&harness); err != nil {
		return Package{}, fmt.Errorf("parse provider harness %s: %w", harnessPath, err)
	}
	if err := validateHarnessPackage(provider, harness, profileIDs); err != nil {
		return Package{}, err
	}
	provider.Harness = &harness
	return provider, nil
}

func manifestHarnessKind(manifest map[string]any, path string) (string, error) {
	raw, exists := manifest["harness"]
	if !exists {
		return "", nil
	}
	harness, ok := raw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s: harness must be an object", path)
	}
	kind, err := requiredString(harness, "kind", path+"#harness")
	if err != nil {
		return "", err
	}
	if kind != "native_cli" && kind != "acp" {
		return "", fmt.Errorf("%s: unknown harness kind %q", path, kind)
	}
	return kind, nil
}

func indexIdentity(provider Package, identities map[string]string) error {
	if owner, exists := identities[provider.ID]; exists {
		return fmt.Errorf("provider package identity collision: canonical id %q is owned by %q and %q", provider.ID, owner, provider.Directory)
	}
	identities[provider.ID] = provider.Directory
	for _, alias := range provider.Aliases {
		if alias == provider.ID {
			return fmt.Errorf("provider package %q: alias %q duplicates its canonical id", provider.ID, alias)
		}
		if owner, exists := identities[alias]; exists {
			return fmt.Errorf("provider package identity collision: %q is owned by %q and %q", alias, owner, provider.Directory)
		}
		identities[alias] = provider.Directory
	}
	return nil
}

func validateHarnessPackage(provider Package, harness HarnessPackage, profileIDs map[string]struct{}) error {
	path := fsPath(provider.Directory, HarnessFile)
	if harness.Implementation == nil {
		return fmt.Errorf("%s: implementation binding is required", path)
	}
	if harness.Implementation.Kind != ImplementationKindACPAgent {
		return fmt.Errorf("%s: unknown implementation kind %q", path, harness.Implementation.Kind)
	}
	if harness.Launch == nil {
		return fmt.Errorf("%s: launch definition is required", path)
	}
	launch := harness.Launch
	if !validLaunchPosture(launch.Posture) {
		return fmt.Errorf("%s: unknown launch posture %q", path, launch.Posture)
	}
	if launch.Transport != TransportStdio {
		return fmt.Errorf("%s: unsupported launch transport %q", path, launch.Transport)
	}
	selectable := launch.Posture != LaunchPostureCatalogOnly
	profile := strings.TrimSpace(harness.Implementation.Profile)
	if selectable {
		if profile == "" {
			return fmt.Errorf("%s: selectable launch requires an implementation profile", path)
		}
		if _, exists := profileIDs[profile]; !exists {
			return fmt.Errorf("%s: implementation profile %q is not registered", path, profile)
		}
	} else if profile != "" {
		if _, exists := profileIDs[profile]; !exists {
			return fmt.Errorf("%s: catalog-only implementation profile %q is not registered", path, profile)
		}
	}
	if err := validateLaunchData(*launch, selectable, path); err != nil {
		return err
	}
	if err := validateProviderPosture(provider.Manifest, launch.Posture, path); err != nil {
		return err
	}
	return validateProviderFacts(provider.Manifest, selectable, path)
}

func validLaunchPosture(posture LaunchPosture) bool {
	switch posture {
	case LaunchPostureBundled, LaunchPosturePackageRunner, LaunchPostureInstalledExecutable, LaunchPostureCatalogOnly:
		return true
	default:
		return false
	}
}

func validateLaunchData(launch LaunchDefinition, selectable bool, path string) error {
	command := strings.TrimSpace(launch.Command)
	if selectable {
		if command == "" {
			return fmt.Errorf("%s: selectable launch command is required", path)
		}
		if command != launch.Command || strings.IndexFunc(command, unicode.IsSpace) >= 0 || strings.ContainsRune(command, '\x00') {
			return fmt.Errorf("%s: launch command must be one shell-free executable token", path)
		}
		if strings.ContainsAny(command, "|;&<>`$()") {
			return fmt.Errorf("%s: launch command contains shell syntax", path)
		}
	} else if strings.ContainsRune(launch.Command, '\x00') {
		return fmt.Errorf("%s: launch command contains a NUL byte", path)
	}
	for index, argument := range launch.Arguments {
		if strings.TrimSpace(argument) == "" || strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("%s: launch argument %d is empty or contains a NUL byte", path, index)
		}
	}
	return nil
}

func validateProviderPosture(manifest map[string]any, posture LaunchPosture, path string) error {
	availability, _ := manifest["implementationAvailability"].(string)
	want := map[LaunchPosture]string{
		LaunchPostureBundled:             "bundled",
		LaunchPosturePackageRunner:       "externally-supplied",
		LaunchPostureInstalledExecutable: "externally-supplied",
		LaunchPostureCatalogOnly:         "catalog-only",
	}[posture]
	if availability != want {
		return fmt.Errorf("%s: launch posture %q requires implementationAvailability %q, got %q", path, posture, want, availability)
	}
	return nil
}

func validateProviderFacts(manifest map[string]any, selectable bool, path string) error {
	providerID, _ := manifest["id"].(string)
	harness, ok := manifest["harness"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s: ACP provider manifest %q is missing harness metadata", path, providerID)
	}
	if _, ok := harness["acpSupport"].(map[string]any); !ok {
		return fmt.Errorf("%s: ACP provider manifest %q is missing typed acpSupport", path, providerID)
	}
	posture, _ := manifest["modelCatalogPosture"].(string)
	if !validModelCatalogPosture(posture) {
		return fmt.Errorf("%s: provider %q has invalid model catalog posture %q", path, providerID, posture)
	}
	if !selectable && posture != "unknown" {
		return fmt.Errorf("%s: catalog-only provider %q must use unknown model catalog posture", path, providerID)
	}
	for _, field := range []string{"models", "tools", "knownLimits", "harnessRoutes", "evidence"} {
		if _, ok := manifest[field].([]any); !ok {
			return fmt.Errorf("%s: provider %q requires %s as an array", path, providerID, field)
		}
	}
	if err := validatePrerequisites(manifest, selectable, path); err != nil {
		return err
	}
	evidence, err := validateEvidence(manifest, selectable, path)
	if err != nil {
		return err
	}
	facts, err := collectFacts(manifest, path)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(facts)+1)
	known["model_catalog"] = struct{}{}
	for _, fact := range facts {
		if _, duplicate := known[fact.ref]; duplicate {
			return fmt.Errorf("%s: provider %q has duplicate capability fact %q", path, providerID, fact.ref)
		}
		known[fact.ref] = struct{}{}
		if !selectable && fact.support != "unknown" {
			return fmt.Errorf("%s: catalog-only provider %q fact %q must remain unknown, got %q", path, providerID, fact.ref, fact.support)
		}
		if err := validateFactEvidence(fact, evidence, path, selectable); err != nil {
			return err
		}
	}
	for evidenceID, refs := range evidence.factRefs {
		for _, ref := range refs {
			if _, exists := known[ref]; !exists {
				return fmt.Errorf("%s: evidence %q references out-of-bounds fact %q", path, evidenceID, ref)
			}
		}
	}
	return nil
}

type capabilityFact struct {
	ref          string
	support      string
	evidenceRefs []string
}

type evidenceIndex struct {
	ids      map[string]struct{}
	factRefs map[string][]string
}

func validatePrerequisites(manifest map[string]any, selectable bool, path string) error {
	discovery, ok := manifest["discovery"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s: discovery metadata is required", path)
	}
	items, ok := discovery["prerequisites"].([]any)
	if !ok {
		return fmt.Errorf("%s: discovery prerequisites must be an array", path)
	}
	if !selectable && len(items) == 0 {
		return fmt.Errorf("%s: catalog-only provider requires actionable prerequisites", path)
	}
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: prerequisite %d is not an object", path, index)
		}
		for _, field := range []string{"kind", "name", "description"} {
			value, _ := item[field].(string)
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s: prerequisite %d is missing %s", path, index, field)
			}
		}
	}
	return nil
}

func validateEvidence(manifest map[string]any, selectable bool, path string) (evidenceIndex, error) {
	result := evidenceIndex{ids: make(map[string]struct{}), factRefs: make(map[string][]string)}
	items := manifest["evidence"].([]any)
	if selectable && len(items) == 0 {
		return result, fmt.Errorf("%s: selectable ACP provider requires capability evidence", path)
	}
	for index, raw := range items {
		record, ok := raw.(map[string]any)
		if !ok {
			return result, fmt.Errorf("%s: evidence %d is not an object", path, index)
		}
		id, _ := record["id"].(string)
		if strings.TrimSpace(id) == "" {
			return result, fmt.Errorf("%s: evidence %d has an empty id", path, index)
		}
		if _, duplicate := result.ids[id]; duplicate {
			return result, fmt.Errorf("%s: duplicate evidence id %q", path, id)
		}
		kind, _ := record["kind"].(string)
		if !validEvidenceKind(kind) {
			return result, fmt.Errorf("%s: evidence %q has unknown kind %q", path, id, kind)
		}
		verifiedOn, _ := record["verifiedOn"].(string)
		if _, err := time.Parse("2006-01-02", verifiedOn); err != nil {
			return result, fmt.Errorf("%s: evidence %q has invalid verifiedOn date", path, id)
		}
		refs, err := stringArray(record, "factRefs", path+"#evidence."+id)
		if err != nil {
			return result, err
		}
		if selectable && len(refs) == 0 {
			return result, fmt.Errorf("%s: evidence %q must reference at least one fact", path, id)
		}
		result.ids[id] = struct{}{}
		result.factRefs[id] = refs
	}
	return result, nil
}

func collectFacts(manifest map[string]any, path string) ([]capabilityFact, error) {
	facts := []capabilityFact{}
	harness := manifest["harness"].(map[string]any)
	acp := harness["acpSupport"].(map[string]any)
	fact, err := mapFact("harness/acp", acp, path)
	if err != nil {
		return nil, err
	}
	facts = append(facts, fact)
	routes, err := mapArray(manifest["harnessRoutes"], path+"#harnessRoutes")
	if err != nil {
		return nil, err
	}
	for index, route := range routes {
		direction, _ := route["direction"].(string)
		modality, _ := route["modality"].(string)
		fact, err := mapFact("harness/"+direction+"/"+modality, route, path)
		if err != nil {
			return nil, fmt.Errorf("%s: harness route %d: %w", path, index, err)
		}
		facts = append(facts, fact)
	}
	models, err := mapArray(manifest["models"], path+"#models")
	if err != nil {
		return nil, err
	}
	for modelIndex, model := range models {
		modelID, _ := model["id"].(string)
		modalities, err := mapArray(model["modalities"], path+"#models["+fmt.Sprint(modelIndex)+"].modalities")
		if err != nil {
			return nil, err
		}
		for modalityIndex, modality := range modalities {
			direction, _ := modality["direction"].(string)
			kind, _ := modality["modality"].(string)
			fact, err := mapFact("model/"+modelID+"/"+direction+"/"+kind, modality, path)
			if err != nil {
				return nil, fmt.Errorf("%s: model %q modality %d: %w", path, modelID, modalityIndex, err)
			}
			facts = append(facts, fact)
		}
	}
	tools, err := mapArray(manifest["tools"], path+"#tools")
	if err != nil {
		return nil, err
	}
	for toolIndex, tool := range tools {
		name, _ := tool["name"].(string)
		fact, err := mapFact("tool/"+name, tool, path)
		if err != nil {
			return nil, fmt.Errorf("%s: tool %d: %w", path, toolIndex, err)
		}
		facts = append(facts, fact)
		outputs, err := mapArrayOptional(tool["outputModalities"], path+"#tools["+fmt.Sprint(toolIndex)+"].outputModalities")
		if err != nil {
			return nil, err
		}
		for outputIndex, output := range outputs {
			modality, _ := output["modality"].(string)
			fact, err := mapFact("tool/"+name+"/output/"+modality, output, path)
			if err != nil {
				return nil, fmt.Errorf("%s: tool %q output %d: %w", path, name, outputIndex, err)
			}
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

func mapFact(ref string, value map[string]any, path string) (capabilityFact, error) {
	support, _ := value["support"].(string)
	if !validSupport(support) {
		return capabilityFact{}, fmt.Errorf("%s: capability fact %q has unknown support %q", path, ref, support)
	}
	condition, _ := value["condition"].(string)
	if support == "conditional" && strings.TrimSpace(condition) == "" {
		return capabilityFact{}, fmt.Errorf("%s: capability fact %q requires a condition", path, ref)
	}
	if support != "conditional" && strings.TrimSpace(condition) != "" {
		return capabilityFact{}, fmt.Errorf("%s: capability fact %q has a condition without conditional support", path, ref)
	}
	refs, err := stringArray(value, "evidenceRefs", path+"#"+ref)
	if err != nil {
		return capabilityFact{}, err
	}
	return capabilityFact{ref: ref, support: support, evidenceRefs: refs}, nil
}

func validateFactEvidence(fact capabilityFact, evidence evidenceIndex, path string, requireEvidence bool) error {
	if fact.support == "unknown" {
		if len(fact.evidenceRefs) != 0 {
			return fmt.Errorf("%s: unknown capability fact %q must not cite evidence", path, fact.ref)
		}
		return nil
	}
	if requireEvidence && len(fact.evidenceRefs) == 0 {
		return fmt.Errorf("%s: non-unknown capability fact %q requires evidenceRefs", path, fact.ref)
	}
	for _, ref := range fact.evidenceRefs {
		if _, exists := evidence.ids[ref]; !exists {
			return fmt.Errorf("%s: capability fact %q references missing evidence %q", path, fact.ref, ref)
		}
	}
	return nil
}

func mapArray(value any, path string) ([]map[string]any, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: value must be an array", path)
	}
	return mapArrayValues(values, path)
}

func mapArrayOptional(value any, path string) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	return mapArray(value, path)
}

func mapArrayValues(values []any, path string) ([]map[string]any, error) {
	result := make([]map[string]any, len(values))
	for index, value := range values {
		mapped, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: value must be an object", path, index)
		}
		result[index] = mapped
	}
	return result, nil
}

func readYAMLMap(source fs.FS, path string) (map[string]any, error) {
	payload, err := fs.ReadFile(source, path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := yaml.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if result == nil {
		return nil, fmt.Errorf("parse %s: document must be an object", path)
	}
	return result, nil
}

func requiredString(value map[string]any, field, path string) (string, error) {
	text, ok := value[field].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s: %s must be a non-empty string", path, field)
	}
	return text, nil
}

func stringArray(value map[string]any, field, path string) ([]string, error) {
	raw, exists := value[field]
	if !exists || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be an array", path, field)
	}
	result := make([]string, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(string)
		if !ok || strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("%s: %s[%d] must be a non-empty string", path, field, index)
		}
		if _, duplicate := seen[item]; duplicate {
			return nil, fmt.Errorf("%s: %s contains duplicate %q", path, field, item)
		}
		seen[item] = struct{}{}
		result[index] = item
	}
	return result, nil
}

func fsPath(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.Trim(part, "/"))
	}
	return strings.Join(values, "/")
}

func validSupport(value string) bool {
	switch value {
	case "supported", "unsupported", "conditional", "unknown":
		return true
	default:
		return false
	}
}

func validModelCatalogPosture(value string) bool {
	switch value {
	case "exact", "runtime_discovered", "operator_selected", "unknown":
		return true
	default:
		return false
	}
}

func validEvidenceKind(value string) bool {
	switch value {
	case "primary_documentation", "protocol_probe", "conformance_fixture", "maintainer_assertion":
		return true
	default:
		return false
	}
}
