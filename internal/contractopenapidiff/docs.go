package contractopenapidiff

import (
	"github.com/getkin/kin-openapi/openapi3"
)

func collectDocumentationChanges(before, after *openapi3.T) []Change {
	var changes []Change
	changes = appendInfoDocChanges(changes, before.Info, after.Info)
	changes = appendRootExternalDocsChanges(changes, before.ExternalDocs, after.ExternalDocs)
	changes = appendTagDocChanges(changes, before.Tags, after.Tags)
	changes = appendPathDocChanges(changes, before.Paths, after.Paths)
	changes = appendComponentDocChanges(changes, before.Components, after.Components)
	return changes
}

func appendInfoDocChanges(changes []Change, before, after *openapi3.Info) []Change {
	if before == nil && after == nil {
		return changes
	}
	if before == nil || after == nil {
		return changes
	}
	if before.Description != after.Description {
		changes = appendDocChange(changes, CodeInfoDescriptionChanged, "info")
	}
	return changes
}

func appendRootExternalDocsChanges(changes []Change, before, after *openapi3.ExternalDocs) []Change {
	return appendExternalDocsChange(changes, CodeInfoExternalDocsChanged, "externalDocs", before, after)
}

func appendTagDocChanges(changes []Change, before, after openapi3.Tags) []Change {
	beforeByName := tagsByName(before)
	afterByName := tagsByName(after)
	for name := range beforeByName {
		beforeTag := beforeByName[name]
		afterTag, ok := afterByName[name]
		if !ok {
			continue
		}
		if beforeTag.Description != afterTag.Description {
			changes = appendDocChange(changes, CodeTagDescriptionChanged, "tags."+name)
		}
		changes = appendExternalDocsChange(
			changes,
			CodeTagExternalDocsChanged,
			"tags."+name+".externalDocs",
			beforeTag.ExternalDocs,
			afterTag.ExternalDocs,
		)
	}
	return changes
}

func tagsByName(tags openapi3.Tags) map[string]*openapi3.Tag {
	out := make(map[string]*openapi3.Tag, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		out[tag.Name] = tag
	}
	return out
}

func appendPathDocChanges(changes []Change, before, after *openapi3.Paths) []Change {
	if before == nil && after == nil {
		return changes
	}
	if before == nil || after == nil {
		return changes
	}

	beforePaths := before.Map()
	afterPaths := after.Map()
	for path := range beforePaths {
		beforeItem := beforePaths[path]
		afterItem := afterPaths[path]
		if beforeItem == nil || afterItem == nil {
			continue
		}
		for method, beforeOperation := range beforeItem.Operations() {
			afterOperation := afterItem.GetOperation(method)
			if beforeOperation == nil || afterOperation == nil {
				continue
			}
			changes = appendOperationDocChanges(changes, method, path, beforeOperation, afterOperation)
		}
	}
	return changes
}

func appendOperationDocChanges(changes []Change, method, path string, before, after *openapi3.Operation) []Change {
	opPath := operationPath(method, path)
	if before.Summary != after.Summary {
		changes = appendDocChange(changes, CodeOperationSummaryChanged, opPath)
	}
	if before.Description != after.Description {
		changes = appendDocChange(changes, CodeOperationDescriptionChanged, opPath)
	}
	changes = appendExternalDocsChange(changes, CodeOperationExternalDocsChanged, opPath, before.ExternalDocs, after.ExternalDocs)

	changes = appendParameterDocChanges(changes, opPath, before.Parameters, after.Parameters)
	changes = appendRequestBodyDocChanges(changes, opPath, before.RequestBody, after.RequestBody)
	changes = appendResponseDocChanges(changes, opPath, before.Responses, after.Responses)
	return changes
}

func appendParameterDocChanges(changes []Change, opPath string, before, after openapi3.Parameters) []Change {
	beforeByKey := parametersByKey(before)
	afterByKey := parametersByKey(after)
	for key, beforeParameter := range beforeByKey {
		afterParameter, ok := afterByKey[key]
		if !ok || beforeParameter == nil || afterParameter == nil {
			continue
		}
		if beforeParameter.Description != afterParameter.Description {
			changes = appendDocChange(changes, CodeParameterDescriptionChanged, opPath+".parameters["+key+"]")
		}
	}
	return changes
}

func parametersByKey(parameters openapi3.Parameters) map[string]*openapi3.Parameter {
	out := make(map[string]*openapi3.Parameter, len(parameters))
	for _, parameterRef := range parameters {
		if parameterRef == nil || parameterRef.Value == nil {
			continue
		}
		parameter := parameterRef.Value
		key := parameter.In + ":" + parameter.Name
		out[key] = parameter
	}
	return out
}

func appendRequestBodyDocChanges(changes []Change, opPath string, before, after *openapi3.RequestBodyRef) []Change {
	beforeBody := requestBodyValue(before)
	afterBody := requestBodyValue(after)
	if beforeBody == nil || afterBody == nil {
		return changes
	}
	if beforeBody.Description != afterBody.Description {
		changes = appendDocChange(changes, CodeRequestBodyDescriptionChanged, opPath+".requestBody")
	}
	return changes
}

func requestBodyValue(ref *openapi3.RequestBodyRef) *openapi3.RequestBody {
	if ref == nil || ref.Value == nil {
		return nil
	}
	return ref.Value
}

func appendResponseDocChanges(changes []Change, opPath string, before, after *openapi3.Responses) []Change {
	if before == nil || after == nil {
		return changes
	}
	for status, beforeResponseRef := range before.Map() {
		afterResponseRef := after.Value(status)
		beforeResponse := responseValue(beforeResponseRef)
		afterResponse := responseValue(afterResponseRef)
		if beforeResponse == nil || afterResponse == nil {
			continue
		}
		if !stringPtrEqual(beforeResponse.Description, afterResponse.Description) {
			changes = appendDocChange(changes, CodeResponseDescriptionChanged, opPath+".responses."+status)
		}
	}
	return changes
}

func responseValue(ref *openapi3.ResponseRef) *openapi3.Response {
	if ref == nil || ref.Value == nil {
		return nil
	}
	return ref.Value
}

func appendComponentDocChanges(changes []Change, before, after *openapi3.Components) []Change {
	if before == nil && after == nil {
		return changes
	}
	if before == nil || after == nil {
		return changes
	}
	if before.Schemas != nil {
		for name, beforeSchemaRef := range before.Schemas {
			afterSchemaRef := after.Schemas[name]
			changes = appendSchemaDocChanges(changes, "components.schemas."+name, beforeSchemaRef, afterSchemaRef)
		}
	}
	return changes
}

func appendSchemaDocChanges(changes []Change, schemaPath string, beforeRef, afterRef *openapi3.SchemaRef) []Change {
	beforeSchema := schemaValue(beforeRef)
	afterSchema := schemaValue(afterRef)
	if beforeSchema == nil || afterSchema == nil {
		return changes
	}
	if beforeSchema.Title != afterSchema.Title {
		changes = appendDocChange(changes, CodeSchemaTitleChanged, schemaPath)
	}
	if beforeSchema.Description != afterSchema.Description {
		changes = appendDocChange(changes, CodeSchemaDescriptionChanged, schemaPath)
	}
	changes = appendExternalDocsChange(changes, CodeSchemaExternalDocsChanged, schemaPath, beforeSchema.ExternalDocs, afterSchema.ExternalDocs)
	changes = appendPropertySchemaDocChanges(changes, schemaPath, beforeSchema, afterSchema)
	return changes
}

func appendPropertySchemaDocChanges(changes []Change, schemaPath string, before, after *openapi3.Schema) []Change {
	if before == nil || after == nil || before.Properties == nil {
		return changes
	}
	for propertyName, beforePropertyRef := range before.Properties {
		afterPropertyRef := after.Properties[propertyName]
		changes = appendSchemaDocChanges(changes, schemaPath+".properties."+propertyName, beforePropertyRef, afterPropertyRef)
	}
	if before.Items != nil && after.Items != nil {
		changes = appendSchemaDocChanges(changes, schemaPath+".items", before.Items, after.Items)
	}
	return changes
}

func schemaValue(ref *openapi3.SchemaRef) *openapi3.Schema {
	if ref == nil || ref.Value == nil {
		return nil
	}
	return ref.Value
}

func unsupportedStructuralDiff(path string) error {
	return &UnsupportedDiffError{Path: path}
}
