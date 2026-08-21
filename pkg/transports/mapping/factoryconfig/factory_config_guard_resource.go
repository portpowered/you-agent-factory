// Factory guard, resource, and shared optional-field mapping helpers.
package factoryconfig

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"
)

func resourcesAPIFromInternal(resources []interfaces.ResourceConfig) *[]factoryapi.Resource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]factoryapi.Resource, len(resources))
	for i, resource := range resources {
		result[i] = factoryapi.Resource{
			Id:         stringPtrIfNotEmpty(resource.ID),
			Name:       resource.Name,
			Type:       resourceTypePtrIfNotEmpty(resource.Type),
			Capacity:   resource.Capacity,
			Model:      stringPtrIfNotEmpty(resource.Model),
			Backend:    stringPtrIfNotEmpty(resource.Backend),
			LoadPolicy: stringPtrIfNotEmpty(resource.LoadPolicy),
			Provider:   stringPtrIfNotEmpty(resource.Provider),
		}
	}
	return &result
}

func resourceManifestAPIFromInternal(manifest *interfaces.PortableResourceManifestConfig) *factoryapi.ResourceManifest {
	if manifest == nil {
		return nil
	}
	return &factoryapi.ResourceManifest{
		RequiredTools: requiredToolsAPIFromInternal(manifest.RequiredTools),
		BundledFiles:  bundledFilesAPIFromInternal(manifest.BundledFiles),
	}
}

func requiredToolsAPIFromInternal(requiredTools []interfaces.RequiredToolConfig) *[]factoryapi.RequiredTool {
	if len(requiredTools) == 0 {
		return nil
	}
	values := make([]factoryapi.RequiredTool, len(requiredTools))
	for i, tool := range requiredTools {
		values[i] = factoryapi.RequiredTool{
			Name:        tool.Name,
			Command:     tool.Command,
			Purpose:     stringPtrIfNotEmpty(tool.Purpose),
			VersionArgs: stringSlicePtr(tool.VersionArgs),
		}
	}
	return &values
}

func bundledFilesAPIFromInternal(bundledFiles []interfaces.BundledFileConfig) *[]factoryapi.BundledFile {
	if len(bundledFiles) == 0 {
		return nil
	}
	sorted := append([]interfaces.BundledFileConfig(nil), bundledFiles...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TargetPath < sorted[j].TargetPath
	})
	values := make([]factoryapi.BundledFile, len(sorted))
	for i, file := range sorted {
		values[i] = factoryapi.BundledFile{
			Id:         stringPtrIfNotEmpty(interfaces.CanonicalBundledFileID(file.ID, file.TargetPath)),
			Type:       factoryapi.BundledFileType(file.Type),
			TargetPath: file.TargetPath,
			Content:    bundledFileContentAPIFromInternal(file),
		}
	}
	return &values
}

func bundledFileContentAPIFromInternal(file interfaces.BundledFileConfig) factoryapi.BundledFileContent {
	return factoryapi.BundledFileContent{
		Encoding: factoryapi.BundledFileContentEncoding(file.Content.Encoding),
		Inline:   file.Content.Inline,
	}
}

func dropSupportedPortableBundledInlineContent(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	supportingFiles, ok := root["supportingFiles"].(map[string]any)
	if !ok {
		return
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok {
		return
	}
	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		fileType, _ := bundledFile["type"].(string)
		targetPath, _ := bundledFile["targetPath"].(string)
		if !interfaces.ShouldOmitSupportedPortableBundledInline(interfaces.BundledFileConfig{
			Type:       fileType,
			TargetPath: targetPath,
		}) {
			continue
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			continue
		}
		inline, _ := content["inline"].(string)
		if strings.TrimSpace(inline) != "" {
			continue
		}
		delete(content, "inline")
	}
}

func inputGuardAPIFromInternal(guard interfaces.InputGuardConfig) factoryapi.InputGuard {
	apiGuard := factoryapi.InputGuard{
		Type: publicInputGuardTypeFromInternal(guard.Type),
	}
	if guard.MatchInput != "" {
		apiGuard.MatchInput = stringPtr(guard.MatchInput)
	}
	if guard.ParentInput != "" {
		apiGuard.ParentInput = stringPtr(guard.ParentInput)
	}
	if guard.SpawnedBy != "" {
		apiGuard.SpawnedBy = stringPtr(guard.SpawnedBy)
	}
	return apiGuard
}

func resourceTypePtrIfNotEmpty(value string) *factoryapi.ResourceType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	canonical := factoryapi.ResourceType(publicFactoryResourceTypeFromInternal(value))
	return &canonical
}

func workstationGuardsAPIFromInternal(guards []interfaces.GuardConfig) *[]factoryapi.WorkstationGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkstationGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.WorkstationGuard{
			Type:              publicWorkstationGuardTypeFromInternal(guard.Type),
			Workstation:       stringPtrIfNotEmpty(guard.Workstation),
			MaxVisits:         intPtrIfNonZero(guard.MaxVisits),
			MaxVisitsArgument: stringPtrIfNotEmpty(guard.MaxVisitsArgument),
			MatchConfig:       guardMatchConfigAPIFromInternal(guard.MatchConfig),
		}
	}
	return &values
}

func factoryGuardsAPIFromInternal(guards []interfaces.FactoryGuardConfig) *[]factoryapi.FactoryGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.FactoryGuard{
			Type:          publicFactoryRootGuardTypeFromInternal(guard.Type),
			ModelProvider: publicFactoryWorkerModelProviderFromInternal(guard.ModelProvider),
			Model:         stringPtrIfNotEmpty(guard.Model),
			RefreshWindow: guard.RefreshWindow,
		}
	}
	return &values
}

func guardMatchConfigAPIFromInternal(matchConfig *interfaces.GuardMatchConfig) *factoryapi.GuardMatchConfig {
	if matchConfig == nil {
		return nil
	}
	return &factoryapi.GuardMatchConfig{
		InputKey: matchConfig.InputKey,
	}
}

func valueOrEmpty[T ~string](value *T) T {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func stringSlicePtr(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	copied := factoryapi.StringMap(cloneStringMap(values))
	return &copied
}

func intPtrIfNonZero(value int) *int {
	return optional.NonZeroIntPtr(value)
}

func boolPtrIfTrue(value bool) *bool {
	return optional.TrueBoolPtr(value)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func boolValue(value *bool) bool {
	return optional.BoolValue(value)
}
