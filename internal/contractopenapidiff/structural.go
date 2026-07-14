package contractopenapidiff

import (
	"fmt"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
)

func compareStructural(before, after *openapi3.T) error {
	if before.OpenAPI != after.OpenAPI {
		return unsupportedStructuralDiff("openapi")
	}
	if err := compareInfoStructural(before.Info, after.Info); err != nil {
		return err
	}
	if err := compareServersStructural(before.Servers, after.Servers); err != nil {
		return err
	}
	if err := compareSecurityStructural(before.Security, after.Security); err != nil {
		return err
	}
	if err := compareTagsStructural(before.Tags, after.Tags); err != nil {
		return err
	}
	if err := comparePathsStructural(before.Paths, after.Paths); err != nil {
		return err
	}
	if err := compareComponentsStructural(before.Components, after.Components); err != nil {
		return err
	}
	return nil
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

func comparePathsStructural(before, after *openapi3.Paths) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff("paths")
	}
	beforePaths := before.Map()
	afterPaths := after.Map()
	if len(beforePaths) != len(afterPaths) {
		return unsupportedStructuralDiff("paths")
	}
	for path := range beforePaths {
		afterItem, ok := afterPaths[path]
		if !ok {
			return unsupportedStructuralDiff("paths." + path)
		}
		if err := comparePathItemStructural(path, beforePaths[path], afterItem); err != nil {
			return err
		}
	}
	return nil
}

func comparePathItemStructural(path string, before, after *openapi3.PathItem) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff("paths." + path)
	}
	beforeOps := before.Operations()
	afterOps := after.Operations()
	if len(beforeOps) != len(afterOps) {
		return unsupportedStructuralDiff("paths." + path)
	}
	for method, beforeOperation := range beforeOps {
		afterOperation := after.GetOperation(method)
		if err := compareOperationStructural(operationPath(method, path), beforeOperation, afterOperation); err != nil {
			return err
		}
	}
	return nil
}

func compareOperationStructural(opPath string, before, after *openapi3.Operation) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff(opPath)
	}
	if before.OperationID != after.OperationID {
		return unsupportedStructuralDiff(opPath + ".operationId")
	}
	if !reflect.DeepEqual(before.Tags, after.Tags) {
		return unsupportedStructuralDiff(opPath + ".tags")
	}
	if before.Deprecated != after.Deprecated {
		return unsupportedStructuralDiff(opPath + ".deprecated")
	}
	if err := compareParametersStructural(opPath, before.Parameters, after.Parameters); err != nil {
		return err
	}
	if err := compareRequestBodyStructural(opPath, before.RequestBody, after.RequestBody); err != nil {
		return err
	}
	if err := compareResponsesStructural(opPath, before.Responses, after.Responses); err != nil {
		return err
	}
	return nil
}

func compareParametersStructural(opPath string, before, after openapi3.Parameters) error {
	beforeByKey := parametersByKey(before)
	afterByKey := parametersByKey(after)
	if len(beforeByKey) != len(afterByKey) {
		return unsupportedStructuralDiff(opPath + ".parameters")
	}
	for key, beforeParameter := range beforeByKey {
		afterParameter, ok := afterByKey[key]
		if !ok {
			return unsupportedStructuralDiff(opPath + ".parameters[" + key + "]")
		}
		if err := compareParameterStructural(opPath+".parameters["+key+"]", beforeParameter, afterParameter); err != nil {
			return err
		}
	}
	return nil
}

func compareParameterStructural(path string, before, after *openapi3.Parameter) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff(path)
	}
	if before.Name != after.Name || before.In != after.In || before.Required != after.Required {
		return unsupportedStructuralDiff(path)
	}
	if err := compareSchemaRefStructural(path+".schema", before.Schema, after.Schema); err != nil {
		return err
	}
	return nil
}

func compareRequestBodyStructural(opPath string, before, after *openapi3.RequestBodyRef) error {
	beforeBody := requestBodyValue(before)
	afterBody := requestBodyValue(after)
	if beforeBody == nil && afterBody == nil {
		return nil
	}
	if beforeBody == nil || afterBody == nil {
		return unsupportedStructuralDiff(opPath + ".requestBody")
	}
	if beforeBody.Required != afterBody.Required {
		return unsupportedStructuralDiff(opPath + ".requestBody.required")
	}
	return compareMediaTypesStructural(opPath+".requestBody.content", beforeBody.Content, afterBody.Content)
}

func compareResponsesStructural(opPath string, before, after *openapi3.Responses) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff(opPath + ".responses")
	}
	beforeResponses := before.Map()
	afterResponses := after.Map()
	if len(beforeResponses) != len(afterResponses) {
		return unsupportedStructuralDiff(opPath + ".responses")
	}
	for status, beforeResponseRef := range beforeResponses {
		afterResponseRef := afterResponses[status]
		beforeResponse := responseValue(beforeResponseRef)
		afterResponse := responseValue(afterResponseRef)
		if beforeResponse == nil || afterResponse == nil {
			return unsupportedStructuralDiff(opPath + ".responses." + status)
		}
		if err := compareMediaTypesStructural(opPath+".responses."+status+".content", beforeResponse.Content, afterResponse.Content); err != nil {
			return err
		}
	}
	return nil
}

func compareMediaTypesStructural(path string, before, after openapi3.Content) error {
	if len(before) != len(after) {
		return unsupportedStructuralDiff(path)
	}
	for mediaType, beforeMedia := range before {
		afterMedia, ok := after[mediaType]
		if !ok {
			return unsupportedStructuralDiff(path + "." + mediaType)
		}
		if err := compareSchemaRefStructural(path+"."+mediaType+".schema", beforeMedia.Schema, afterMedia.Schema); err != nil {
			return err
		}
	}
	return nil
}

func compareComponentsStructural(before, after *openapi3.Components) error {
	if before == nil && after == nil {
		return nil
	}
	if before == nil || after == nil {
		return unsupportedStructuralDiff("components")
	}
	if len(before.Schemas) != len(after.Schemas) {
		return unsupportedStructuralDiff("components.schemas")
	}
	for name, beforeSchemaRef := range before.Schemas {
		afterSchemaRef, ok := after.Schemas[name]
		if !ok {
			return unsupportedStructuralDiff("components.schemas." + name)
		}
		if err := compareSchemaRefStructural("components.schemas."+name, beforeSchemaRef, afterSchemaRef); err != nil {
			return err
		}
	}
	return nil
}

func compareSchemaRefStructural(path string, before, after *openapi3.SchemaRef) error {
	if before == nil && after == nil {
		return nil
	}
	if before == nil || after == nil {
		return unsupportedStructuralDiff(path)
	}
	if before.Ref != after.Ref {
		return unsupportedStructuralDiff(path + ".ref")
	}
	if before.Ref != "" {
		return nil
	}
	return compareSchemaStructural(path, before.Value, after.Value)
}

func compareSchemaStructural(path string, before, after *openapi3.Schema) error {
	if before == nil || after == nil {
		return unsupportedStructuralDiff(path)
	}
	if !typesEqual(before.Type, after.Type) {
		return unsupportedStructuralDiff(path + ".type")
	}
	if before.Format != after.Format {
		return unsupportedStructuralDiff(path + ".format")
	}
	if !reflect.DeepEqual(before.Enum, after.Enum) {
		return unsupportedStructuralDiff(path + ".enum")
	}
	if !reflect.DeepEqual(before.Required, after.Required) {
		return unsupportedStructuralDiff(path + ".required")
	}
	if !externalDocsStructuralEqual(before.ExternalDocs, after.ExternalDocs) {
		return unsupportedStructuralDiff(path + ".externalDocs")
	}
	if len(before.Properties) != len(after.Properties) {
		return unsupportedStructuralDiff(path + ".properties")
	}
	for propertyName, beforePropertyRef := range before.Properties {
		afterPropertyRef, ok := after.Properties[propertyName]
		if !ok {
			return unsupportedStructuralDiff(path + ".properties." + propertyName)
		}
		if err := compareSchemaRefStructural(path+".properties."+propertyName, beforePropertyRef, afterPropertyRef); err != nil {
			return err
		}
	}
	if (before.Items == nil) != (after.Items == nil) {
		return unsupportedStructuralDiff(path + ".items")
	}
	if before.Items != nil {
		if err := compareSchemaRefStructural(path+".items", before.Items, after.Items); err != nil {
			return err
		}
	}
	return nil
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
