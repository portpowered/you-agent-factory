package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validationFailure(violations []string) error {
	return fmt.Errorf("provider registry validation failed: %s", strings.Join(sortedUnique(violations), "; "))
}

func validateManifestSchema(manifest Manifest) error {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("serialize manifest: %w", err)
	}
	return validateSchema(modelproviders.ProviderManifestSchemaJSON(), payload)
}

func validateSchema(schemaPayload, document []byte) error {
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaPayload))
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	schemaObject, ok := schemaDocument.(map[string]any)
	if !ok {
		return fmt.Errorf("parse schema: root must be an object")
	}
	schemaID, _ := schemaObject["$id"].(string)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(schemaID, schemaObject); err != nil {
		return fmt.Errorf("load schema: %w", err)
	}
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return fmt.Errorf("parse document: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return err
	}
	return nil
}

func validateIdentityClaims(candidates []manifestCandidate) []string {
	claims := make(map[string][]string)
	var violations []string
	for _, candidate := range candidates {
		canonical := normalize(candidate.manifest.ID)
		if err := inference.ValidateIdentity(inference.Identity(canonical)); err != nil {
			violations = append(violations, identityLabel(canonical)+": manifest identity is invalid: "+err.Error())
		}
		claims[canonical] = append(claims[canonical], "canonical "+identityLabel(canonical))
		for _, rawAlias := range candidate.manifest.Aliases {
			alias := normalize(rawAlias)
			if err := inference.ValidateIdentity(inference.Identity(alias)); err != nil {
				violations = append(violations, identityLabel(alias)+": manifest alias is invalid: "+err.Error())
			}
			claims[alias] = append(claims[alias], "alias of "+identityLabel(canonical))
		}
	}
	for identity, owners := range claims {
		if len(owners) < 2 {
			continue
		}
		sortedOwners := append([]string(nil), owners...)
		sort.Strings(sortedOwners)
		violations = append(violations, fmt.Sprintf("%s: identity collision between %s", identityLabel(identity), strings.Join(sortedOwners, ", ")))
	}
	return violations
}

func validateMaximumCapabilities(identity string, manifest Manifest, integration inference.Integration) []string {
	maximum := integration.MaximumCapabilities()
	if err := inference.ValidateMaximumCapabilities(maximum); err != nil {
		return []string{identityLabel(identity) + ": integration maximum capabilities are invalid: " + err.Error()}
	}
	manifestMaximum := manifestCapabilities(manifest)
	var exceeded []string
	var omitted []string
	for _, capability := range allManifestCapabilities() {
		integrationHas := maximum.Has(capability)
		manifestHas := manifestMaximum.Has(capability)
		if integrationHas && !manifestHas {
			exceeded = append(exceeded, string(capability))
		}
		if manifestHas && !integrationHas {
			omitted = append(omitted, string(capability))
		}
	}
	var violations []string
	if len(exceeded) > 0 {
		violations = append(violations, fmt.Sprintf("%s: integration maximum exceeds manifest maximum: %s", identityLabel(identity), strings.Join(sortedUnique(exceeded), ", ")))
	}
	if len(omitted) > 0 {
		violations = append(violations, fmt.Sprintf("%s: integration maximum contradicts manifest maximum by omitting: %s", identityLabel(identity), strings.Join(sortedUnique(omitted), ", ")))
	}
	return violations
}

func manifestCapabilities(manifest Manifest) inference.CapabilitySet {
	execution := manifest.MaximumExecutionCapabilities
	response := manifest.MaximumResponseFidelityCapabilities
	values := make([]inference.Capability, 0, len(allManifestCapabilities()))
	appendIf := func(enabled bool, capability inference.Capability) {
		if enabled {
			values = append(values, capability)
		}
	}
	appendIf(execution.PromptSubmission, inference.CapabilityPromptSubmission)
	appendIf(execution.ImageInput, inference.CapabilityImageInput)
	appendIf(execution.SessionResume, inference.CapabilitySessionResume)
	appendIf(execution.StructuredOutput, inference.CapabilityStructuredOutput)
	appendIf(response.NativeStreaming, inference.CapabilityNativeStreaming)
	appendIf(response.MessageDeltas, inference.CapabilityMessageDeltas)
	appendIf(response.MessageSnapshots, inference.CapabilityMessageSnapshots)
	appendIf(response.ReasoningSummaries, inference.CapabilityReasoningSummaries)
	appendIf(response.ToolLifecycle, inference.CapabilityToolLifecycle)
	appendIf(response.ToolOutputDeltas, inference.CapabilityToolOutputDeltas)
	appendIf(response.FileChanges, inference.CapabilityFileChanges)
	appendIf(response.Plans, inference.CapabilityPlans)
	appendIf(response.Usage, inference.CapabilityUsage)
	appendIf(response.StableItemIDs, inference.CapabilityStableItemIDs)
	appendIf(response.ProviderReconnect, inference.CapabilityProviderReconnect)
	return inference.NewCapabilitySet(values...)
}

func allManifestCapabilities() []inference.Capability {
	return []inference.Capability{
		inference.CapabilityPromptSubmission,
		inference.CapabilityImageInput,
		inference.CapabilitySessionResume,
		inference.CapabilityStructuredOutput,
		inference.CapabilityNativeStreaming,
		inference.CapabilityMessageDeltas,
		inference.CapabilityMessageSnapshots,
		inference.CapabilityReasoningSummaries,
		inference.CapabilityToolLifecycle,
		inference.CapabilityToolOutputDeltas,
		inference.CapabilityFileChanges,
		inference.CapabilityPlans,
		inference.CapabilityUsage,
		inference.CapabilityStableItemIDs,
		inference.CapabilityProviderReconnect,
	}
}
