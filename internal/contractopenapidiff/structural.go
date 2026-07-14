package contractopenapidiff

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
)

func collectStructuralChanges(before, after *openapi3.T) ([]Change, error) {
	if before.OpenAPI != after.OpenAPI {
		return nil, unsupportedStructuralDiff("openapi")
	}
	if err := compareInfoStructural(before.Info, after.Info); err != nil {
		return nil, err
	}
	if err := compareServersStructural(before.Servers, after.Servers); err != nil {
		return nil, err
	}
	if err := compareSecurityStructural(before.Security, after.Security); err != nil {
		return nil, err
	}
	if err := compareTagsStructural(before.Tags, after.Tags); err != nil {
		return nil, err
	}
	pathChanges, err := collectPathChanges(before.Paths, after.Paths)
	if err != nil {
		return nil, err
	}
	componentChanges, err := collectComponentChanges(before.Components, after.Components)
	if err != nil {
		return nil, err
	}
	return append(pathChanges, componentChanges...), nil
}

func compareInfoStructural(before, after *openapi3.Info) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff("info")
	}
	if before.Title != after.Title || before.Version != after.Version {
		return unsupportedStructuralDiff("info")
	}
	if before.TermsOfService != after.TermsOfService {
		return unsupportedStructuralDiff("info.termsOfService")
	}
	if !contactStructuralEqual(before.Contact, after.Contact) {
		return unsupportedStructuralDiff("info.contact")
	}
	if !licenseStructuralEqual(before.License, after.License) {
		return unsupportedStructuralDiff("info.license")
	}
	return nil
}

func contactStructuralEqual(before, after *openapi3.Contact) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return before.Name == after.Name && before.URL == after.URL && before.Email == after.Email
}

func licenseStructuralEqual(before, after *openapi3.License) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return before.Name == after.Name && before.URL == after.URL
}

func externalDocsStructuralEqual(before, after *openapi3.ExternalDocs) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return before.URL == after.URL
}

func compareServersStructural(before, after openapi3.Servers) error {
	if len(before) != len(after) {
		return unsupportedStructuralDiff("servers")
	}
	for i := range before {
		if before[i] == nil || after[i] == nil {
			return unsupportedStructuralDiff("servers")
		}
		if before[i].URL != after[i].URL {
			return unsupportedStructuralDiff("servers[" + fmt.Sprint(i) + "].url")
		}
	}
	return nil
}

func compareSecurityStructural(before, after openapi3.SecurityRequirements) error {
	if !reflect.DeepEqual(before, after) {
		return unsupportedStructuralDiff("security")
	}
	return nil
}

func compareTagsStructural(before, after openapi3.Tags) error {
	beforeByName := tagsByName(before)
	afterByName := tagsByName(after)
	if len(beforeByName) != len(afterByName) {
		return unsupportedStructuralDiff("tags")
	}
	for name, beforeTag := range beforeByName {
		afterTag, ok := afterByName[name]
		if !ok {
			return unsupportedStructuralDiff("tags." + name)
		}
		if beforeTag.Name != afterTag.Name {
			return unsupportedStructuralDiff("tags." + name)
		}
		if !externalDocsStructuralEqual(beforeTag.ExternalDocs, afterTag.ExternalDocs) {
			return unsupportedStructuralDiff("tags." + name + ".externalDocs")
		}
	}
	return nil
}

func collectPathChanges(before, after *openapi3.Paths) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff("paths")
	}
	beforePaths := before.Map()
	afterPaths := after.Map()

	var changes []Change
	for path := range beforePaths {
		if afterItem, ok := afterPaths[path]; !ok {
			for method := range beforePaths[path].Operations() {
				changes = appendMajorChange(changes, CodeOperationRemoved, operationPath(method, path))
			}
			continue
		} else if beforePaths[path] == nil || afterItem == nil {
			return nil, unsupportedStructuralDiff("paths." + path)
		}
	}

	for path, afterItem := range afterPaths {
		beforeItem, ok := beforePaths[path]
		if !ok {
			for method := range afterItem.Operations() {
				changes = appendMinorChange(changes, CodeOperationAdded, operationPath(method, path))
			}
			continue
		}
		pathChanges, err := collectPathItemChanges(path, beforeItem, afterItem)
		if err != nil {
			return nil, err
		}
		changes = append(changes, pathChanges...)
	}
	return changes, nil
}

func collectPathItemChanges(path string, before, after *openapi3.PathItem) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff("paths." + path)
	}
	beforeOps := before.Operations()
	afterOps := after.Operations()

	var changes []Change
	for method := range beforeOps {
		if after.GetOperation(method) == nil {
			changes = appendMajorChange(changes, CodeOperationRemoved, operationPath(method, path))
		}
	}

	for method, afterOperation := range afterOps {
		beforeOperation := before.GetOperation(method)
		if beforeOperation == nil {
			changes = appendMinorChange(changes, CodeOperationAdded, operationPath(method, path))
			continue
		}
		opChanges, err := collectOperationChanges(operationPath(method, path), beforeOperation, afterOperation)
		if err != nil {
			return nil, err
		}
		changes = append(changes, opChanges...)
	}
	return changes, nil
}

func collectOperationChanges(opPath string, before, after *openapi3.Operation) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff(opPath)
	}
	if before.OperationID != after.OperationID {
		return nil, unsupportedStructuralDiff(opPath + ".operationId")
	}
	if !reflect.DeepEqual(before.Tags, after.Tags) {
		return nil, unsupportedStructuralDiff(opPath + ".tags")
	}
	if before.Deprecated != after.Deprecated {
		return nil, unsupportedStructuralDiff(opPath + ".deprecated")
	}

	var changes []Change
	paramChanges, err := collectParameterChanges(opPath, before.Parameters, after.Parameters)
	if err != nil {
		return nil, err
	}
	changes = append(changes, paramChanges...)

	requestBodyChanges, err := collectRequestBodyChanges(opPath, before.RequestBody, after.RequestBody)
	if err != nil {
		return nil, err
	}
	changes = append(changes, requestBodyChanges...)

	responseChanges, err := collectResponsesChanges(opPath, before.Responses, after.Responses)
	if err != nil {
		return nil, err
	}
	changes = append(changes, responseChanges...)
	return changes, nil
}

func collectParameterChanges(opPath string, before, after openapi3.Parameters) ([]Change, error) {
	beforeByKey := parametersByKey(before)
	afterByKey := parametersByKey(after)

	var changes []Change
	for key := range beforeByKey {
		if _, ok := afterByKey[key]; !ok {
			changes = appendMajorChange(changes, CodeParameterRemoved, opPath+".parameters["+key+"]")
		}
	}

	for key, afterParameter := range afterByKey {
		beforeParameter, ok := beforeByKey[key]
		if !ok {
			if afterParameter.Required {
				changes = appendMajorChange(changes, CodeParameterRequiredNarrowed, opPath+".parameters["+key+"]")
			} else {
				changes = appendMinorChange(changes, CodeParameterAdded, opPath+".parameters["+key+"]")
			}
			continue
		}
		paramChanges, err := collectParameterPairChanges(opPath+".parameters["+key+"]", beforeParameter, afterParameter)
		if err != nil {
			return nil, err
		}
		changes = append(changes, paramChanges...)
	}
	return changes, nil
}

func collectParameterPairChanges(path string, before, after *openapi3.Parameter) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff(path)
	}
	if before.Name != after.Name || before.In != after.In {
		return nil, unsupportedStructuralDiff(path)
	}
	var changes []Change
	if before.Required != after.Required {
		switch {
		case after.Required && !before.Required:
			changes = appendMajorChange(changes, CodeParameterRequiredNarrowed, path)
		case before.Required && !after.Required:
			changes = appendMinorChange(changes, CodeParameterRequiredRelaxed, path)
		}
	}
	schemaChanges, err := collectSchemaRefChanges(path+".schema", before.Schema, after.Schema)
	if err != nil {
		return nil, err
	}
	return append(changes, schemaChanges...), nil
}

func collectRequestBodyChanges(opPath string, before, after *openapi3.RequestBodyRef) ([]Change, error) {
	beforeBody := requestBodyValue(before)
	afterBody := requestBodyValue(after)
	if beforeBody == nil && afterBody == nil {
		return nil, nil
	}
	if beforeBody == nil || afterBody == nil {
		return nil, unsupportedStructuralDiff(opPath + ".requestBody")
	}
	if beforeBody.Required != afterBody.Required {
		return nil, unsupportedStructuralDiff(opPath + ".requestBody.required")
	}
	return collectMediaTypeChanges(opPath+".requestBody.content", beforeBody.Content, afterBody.Content)
}

func collectResponsesChanges(opPath string, before, after *openapi3.Responses) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff(opPath + ".responses")
	}
	beforeResponses := before.Map()
	afterResponses := after.Map()
	if len(beforeResponses) != len(afterResponses) {
		return nil, unsupportedStructuralDiff(opPath + ".responses")
	}
	var changes []Change
	for status, beforeResponseRef := range beforeResponses {
		afterResponseRef := afterResponses[status]
		beforeResponse := responseValue(beforeResponseRef)
		afterResponse := responseValue(afterResponseRef)
		if beforeResponse == nil || afterResponse == nil {
			return nil, unsupportedStructuralDiff(opPath + ".responses." + status)
		}
		responseChanges, err := collectMediaTypeChanges(opPath+".responses."+status+".content", beforeResponse.Content, afterResponse.Content)
		if err != nil {
			return nil, err
		}
		changes = append(changes, responseChanges...)
	}
	return changes, nil
}

func collectMediaTypeChanges(path string, before, after openapi3.Content) ([]Change, error) {
	if len(before) != len(after) {
		return nil, unsupportedStructuralDiff(path)
	}
	var changes []Change
	for mediaType, beforeMedia := range before {
		afterMedia, ok := after[mediaType]
		if !ok {
			return nil, unsupportedStructuralDiff(path + "." + mediaType)
		}
		schemaChanges, err := collectSchemaRefChanges(path+"."+mediaType+".schema", beforeMedia.Schema, afterMedia.Schema)
		if err != nil {
			return nil, err
		}
		changes = append(changes, schemaChanges...)
	}
	return changes, nil
}

func collectComponentChanges(before, after *openapi3.Components) ([]Change, error) {
	if before == nil && after == nil {
		return nil, nil
	}
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff("components")
	}

	var changes []Change
	for name := range before.Schemas {
		if _, ok := after.Schemas[name]; !ok {
			changes = appendMajorChange(changes, CodeSchemaRemoved, "components.schemas."+name)
		}
	}

	for name, afterSchemaRef := range after.Schemas {
		beforeSchemaRef, ok := before.Schemas[name]
		if !ok {
			changes = appendMinorChange(changes, CodeSchemaAdded, "components.schemas."+name)
			continue
		}
		schemaChanges, err := collectSchemaRefChanges("components.schemas."+name, beforeSchemaRef, afterSchemaRef)
		if err != nil {
			return nil, err
		}
		changes = append(changes, schemaChanges...)
	}
	return changes, nil
}

func collectSchemaRefChanges(path string, before, after *openapi3.SchemaRef) ([]Change, error) {
	if before == nil && after == nil {
		return nil, nil
	}
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff(path)
	}
	if before.Ref != after.Ref {
		return nil, unsupportedStructuralDiff(path + ".ref")
	}
	if before.Ref != "" {
		return nil, nil
	}
	return collectSchemaChanges(path, before.Value, after.Value)
}

func collectSchemaChanges(path string, before, after *openapi3.Schema) ([]Change, error) {
	if before == nil || after == nil {
		return nil, unsupportedStructuralDiff(path)
	}
	var changes []Change
	if !typesEqual(before.Type, after.Type) {
		changes = appendMajorChange(changes, CodeSchemaTypeNarrowed, path+".type")
	}
	if before.Format != after.Format {
		changes = appendMajorChange(changes, CodeSchemaTypeNarrowed, path+".format")
	}
	changes = append(changes, collectRequiredChanges(path, before.Required, after.Required)...)
	if !externalDocsStructuralEqual(before.ExternalDocs, after.ExternalDocs) {
		return nil, unsupportedStructuralDiff(path + ".externalDocs")
	}
	enumChanges, err := collectEnumChanges(path, before.Enum, after.Enum)
	if err != nil {
		return nil, err
	}
	changes = append(changes, enumChanges...)

	for propertyName := range before.Properties {
		if _, ok := after.Properties[propertyName]; !ok {
			changes = appendMajorChange(changes, CodeSchemaPropertyRemoved, path+".properties."+propertyName)
		}
	}
	for propertyName, afterPropertyRef := range after.Properties {
		beforePropertyRef, ok := before.Properties[propertyName]
		if !ok {
			if slices.Contains(after.Required, propertyName) {
				changes = appendMajorChange(changes, CodeSchemaRequiredNarrowed, path+".properties."+propertyName)
			} else {
				changes = appendMinorChange(changes, CodeSchemaPropertyAdded, path+".properties."+propertyName)
			}
			continue
		}
		propertyChanges, err := collectSchemaRefChanges(path+".properties."+propertyName, beforePropertyRef, afterPropertyRef)
		if err != nil {
			return nil, err
		}
		changes = append(changes, propertyChanges...)
	}
	if (before.Items == nil) != (after.Items == nil) {
		return nil, unsupportedStructuralDiff(path + ".items")
	}
	if before.Items != nil {
		itemChanges, err := collectSchemaRefChanges(path+".items", before.Items, after.Items)
		if err != nil {
			return nil, err
		}
		changes = append(changes, itemChanges...)
	}
	return changes, nil
}

func collectEnumChanges(path string, before, after []any) ([]Change, error) {
	beforeValues := enumValueSet(before)
	afterValues := enumValueSet(after)
	var changes []Change
	for value := range beforeValues {
		if _, ok := afterValues[value]; !ok {
			changes = appendMajorChange(changes, CodeEnumValueRemoved, path+".enum."+enumValuePath(value))
		}
	}
	for value := range afterValues {
		if _, ok := beforeValues[value]; ok {
			continue
		}
		changes = appendMinorChange(changes, CodeEnumValueAdded, path+".enum."+enumValuePath(value))
	}
	return changes, nil
}

func collectRequiredChanges(path string, beforeRequired, afterRequired []string) []Change {
	beforeSet := stringSet(beforeRequired)
	afterSet := stringSet(afterRequired)
	var changes []Change
	for _, name := range afterRequired {
		if _, ok := beforeSet[name]; ok {
			continue
		}
		changes = appendMajorChange(changes, CodeSchemaRequiredNarrowed, path+".required."+name)
	}
	for _, name := range beforeRequired {
		if _, ok := afterSet[name]; ok {
			continue
		}
		changes = appendMinorChange(changes, CodeSchemaRequiredRelaxed, path+".required."+name)
	}
	return changes
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func enumValueSet(values []any) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[enumValueKey(value)] = struct{}{}
	}
	return out
}

func enumValueKey(value any) string {
	return fmt.Sprint(value)
}

func enumValuePath(value any) string {
	return enumValueKey(value)
}

func typesEqual(before, after *openapi3.Types) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return reflect.DeepEqual(before.Slice(), after.Slice())
}
