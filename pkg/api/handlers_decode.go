package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factorypkg "github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	return map[string]string(*values)
}

func generatedSubmitRelations(values *[]factoryapi.SubmitRelation) []interfaces.Relation {
	if values == nil || len(*values) == 0 {
		return nil
	}
	relations := make([]interfaces.Relation, 0, len(*values))
	for _, relation := range *values {
		relations = append(relations, interfaces.Relation{
			Type:          interfaces.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkId,
			RequiredState: stringValue(relation.RequiredState),
		})
	}
	return relations
}

func generatedWorkRequestToDomain(req factoryapi.WorkRequest) (interfaces.WorkRequest, error) {
	workRequest := interfaces.WorkRequest{
		RequestID:              req.RequestId,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		Type:                   interfaces.WorkRequestType(req.Type),
	}
	if req.Works != nil {
		workRequest.Works = make([]interfaces.Work, 0, len(*req.Works))
		for i, work := range *req.Works {
			content, err := generatedWorkContentToDomainAtPath(work.Content, fmt.Sprintf("works[%d].content", i))
			if err != nil {
				return interfaces.WorkRequest{}, err
			}
			workRequest.Works = append(workRequest.Works, interfaces.Work{
				Name:                     work.Name,
				WorkID:                   stringValue(work.WorkId),
				RequestID:                stringValue(work.RequestId),
				WorkTypeID:               stringValue(work.WorkTypeName),
				State:                    generatedWorkStateName(work.State),
				ChainingTraceDepth:       intValue(work.ChainingTraceDepth),
				CurrentChainingTraceID:   stringValue(work.CurrentChainingTraceId),
				PreviousChainingTraceIDs: stringSliceValue(work.PreviousChainingTraceIds),
				TraceID:                  stringValue(work.TraceId),
				Content:                  content,
				Payload:                  work.Payload,
				Tags:                     generatedStringMap(work.Tags),
			})
		}
	}
	if req.Relations != nil {
		workRequest.Relations = make([]interfaces.WorkRelation, 0, len(*req.Relations))
		for _, relation := range *req.Relations {
			workRequest.Relations = append(workRequest.Relations, interfaces.WorkRelation{
				Type:           interfaces.WorkRelationType(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  stringValue(relation.RequiredState),
			})
		}
	}
	return workRequest, nil
}

func generatedWorkContentToDomain(content *factoryapi.WorkContent) []interfaces.WorkContentPart {
	parts, err := generatedWorkContentToDomainAtPath(content, "content")
	if err != nil {
		return nil
	}
	return parts
}

func domainWorkContentToGeneratedPtr(parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	if len(parts) == 0 {
		return nil
	}
	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				continue
			}
		case interfaces.WorkContentPartTypeImage:
			if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
				Type: factoryapi.WorkContentPartTypeImage,
				File: part.File,
			}); err != nil {
				continue
			}
		default:
			continue
		}
		content = append(content, generated)
	}
	if len(content) == 0 {
		return nil
	}
	return &content
}

func generatedWorkContentToDomainAtPath(content *factoryapi.WorkContent, fieldPath string) ([]interfaces.WorkContentPart, error) {
	if content == nil || len(*content) == 0 {
		return nil, nil
	}

	parts := make([]interfaces.WorkContentPart, 0, len(*content))
	for i, part := range *content {
		pathPrefix := fmt.Sprintf("%s[%d].", fieldPath, i)
		textPart, textErr := part.AsWorkTextContentPart()
		if textErr == nil && textPart.Type == factoryapi.WorkContentPartTypeText {
			parts = append(parts, interfaces.WorkContentPart{
				Type: interfaces.WorkContentPartTypeText,
				Text: textPart.Text,
			})
			continue
		}

		imagePart, imageErr := part.AsWorkImageContentPart()
		if imageErr == nil && imagePart.Type == factoryapi.WorkContentPartTypeImage {
			parts = append(parts, interfaces.WorkContentPart{
				Type: interfaces.WorkContentPartTypeImage,
				File: imagePart.File,
			})
			continue
		}

		return nil, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text or image", pathPrefix)}
	}
	return parts, nil
}

type requestFieldValidationError struct {
	message string
}

func (e requestFieldValidationError) Error() string {
	return e.message
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr requestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.message, true
	}
	return "", false
}

func decodeSubmitWorkRequestBody(body io.Reader) (factoryapi.SubmitWorkJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}

	var req factoryapi.SubmitWorkJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return factoryapi.SubmitWorkJSONRequestBody{}, requestFieldValidationError{message: "name is required"}
	}
	return req, nil
}

func decodeWorkRequestBody(body io.Reader) (factoryapi.UpsertWorkRequestJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	var req factoryapi.UpsertWorkRequestJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}
	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	if req.Works == nil || len(*req.Works) == 0 {
		return req, nil
	}

	var rawRequest struct {
		Works []map[string]json.RawMessage `json:"works"`
	}
	if err := json.Unmarshal(data, &rawRequest); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	for i := range *req.Works {
		if i >= len(rawRequest.Works) {
			return req, nil
		}
		if err := validateWorkContentField(rawRequest.Works[i], fmt.Sprintf("works[%d].", i)); err != nil {
			return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
		}
	}
	return req, nil
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.CreateFactoryJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.CreateFactoryJSONRequestBody
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}
	return req, nil
}

func decodeSaveEditableFactoryDefinitionBody(body io.Reader) (factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}
	return req, nil
}

func decodePromptTemplateValidationRequestBody(body io.Reader) (factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var req factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody
	if err := dec.Decode(&req); err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}
	if err := ensureSingleJSONObject(dec); err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}

	return req, nil
}

func ensureSingleJSONObject(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return err
	}
	return nil
}

func validateCanonicalWorkRequestJSONForAPI(data []byte) error {
	if err := factorypkg.ValidateCanonicalWorkRequestJSON(data); err != nil {
		return translateCanonicalWorkRequestValidationError(err)
	}
	return nil
}

func currentFactoryWorkstation(factory factoryapi.Factory, workstationName string) (factoryapi.Workstation, bool) {
	if factory.Workstations == nil {
		return factoryapi.Workstation{}, false
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name == workstationName || stringValue(workstation.Id) == workstationName {
			return workstation, true
		}
	}
	return factoryapi.Workstation{}, false
}

func promptTemplateContractResponse(contract workers.PromptTemplateContract) factoryapi.PromptTemplateContract {
	availableVariables := make([]factoryapi.PromptTemplateVariableReference, 0, len(contract.AvailableVariables))
	for _, reference := range contract.AvailableVariables {
		availableVariables = append(availableVariables, factoryapi.PromptTemplateVariableReference{
			Category:    factoryapi.PromptTemplateVariableReferenceCategory(reference.Category),
			Description: reference.Description,
			Example:     reference.Example,
			Path:        reference.Path,
		})
	}
	unavailablePatterns := make([]factoryapi.PromptTemplateUnavailableAccessPattern, 0, len(contract.UnavailableAccessPatterns))
	for _, pattern := range contract.UnavailableAccessPatterns {
		unavailablePatterns = append(unavailablePatterns, factoryapi.PromptTemplateUnavailableAccessPattern{
			Example: pattern.Example,
			Path:    pattern.Path,
			Reason:  pattern.Reason,
		})
	}
	return factoryapi.PromptTemplateContract{
		AvailableVariables:        availableVariables,
		InputCount:                contract.InputCount,
		UnavailableAccessPatterns: unavailablePatterns,
	}
}

func promptTemplateValidationResultResponse(result workers.PromptTemplateValidationResult) factoryapi.PromptTemplateValidationResult {
	diagnostics := make([]factoryapi.PromptTemplateDiagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		diagnostics = append(diagnostics, factoryapi.PromptTemplateDiagnostic{
			EndOffset:   diagnostic.EndOffset,
			Kind:        factoryapi.PromptTemplateDiagnosticKind(diagnostic.Kind),
			Message:     diagnostic.Message,
			Path:        diagnostic.Path,
			SourceText:  diagnostic.SourceText,
			StartOffset: diagnostic.StartOffset,
		})
	}
	return factoryapi.PromptTemplateValidationResult{
		Diagnostics: diagnostics,
		Valid:       result.Valid,
	}
}

func validateWorkContentField(fields map[string]json.RawMessage, prefix string) error {
	contentRaw, ok := fields["content"]
	if !ok {
		return nil
	}

	var partPayloads []json.RawMessage
	if err := json.Unmarshal(contentRaw, &partPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%scontent must be an array", prefix)}
	}
	for i, payload := range partPayloads {
		var partFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &partFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%scontent[%d] must be an object", prefix, i)}
		}
		if _, err := validatedRawWorkContentPart(partFields, fmt.Sprintf("%scontent[%d].", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func validatedRawWorkContentPart(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var partType string
	if err := json.Unmarshal(typeRaw, &partType); err != nil || partType == "" {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	switch interfaces.WorkContentPartType(partType) {
	case interfaces.WorkContentPartTypeText:
		if err := requireOnlyFields(fields, prefix, "type", "text"); err != nil {
			return interfaces.WorkContentPart{}, err
		}
		textRaw, ok := fields["text"]
		if !ok {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stext is required for text content parts", prefix)}
		}
		var text string
		if err := json.Unmarshal(textRaw, &text); err != nil {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stext must be a string", prefix)}
		}
		return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeText, Text: text}, nil
	case interfaces.WorkContentPartTypeImage:
		if err := requireOnlyFields(fields, prefix, "type", "file"); err != nil {
			return interfaces.WorkContentPart{}, err
		}
		fileRaw, ok := fields["file"]
		if !ok {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sfile is required for image content parts", prefix)}
		}
		var file string
		if err := json.Unmarshal(fileRaw, &file); err != nil || file == "" {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sfile must be a non-empty string", prefix)}
		}
		return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeImage, File: file}, nil
	default:
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text or image", prefix)}
	}
}

func requireOnlyFields(fields map[string]json.RawMessage, prefix string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		return requestFieldValidationError{message: fmt.Sprintf("%s%s is not supported", prefix, field)}
	}
	return nil
}

func applyStableTraceToWorkRequest(req *interfaces.WorkRequest) {
	if req == nil || len(req.Works) == 0 {
		return
	}
	traceID := ""
	if req.CurrentChainingTraceID != "" {
		traceID = req.CurrentChainingTraceID
	}
	if traceID == "" {
		for _, work := range req.Works {
			if work.CurrentChainingTraceID != "" {
				traceID = work.CurrentChainingTraceID
				break
			}
			if work.TraceID != "" {
				traceID = work.TraceID
				break
			}
		}
	}
	if traceID == "" {
		traceID = "trace-" + req.RequestID
	}
	if req.CurrentChainingTraceID == "" {
		req.CurrentChainingTraceID = traceID
	}
	for i := range req.Works {
		if req.Works[i].CurrentChainingTraceID == "" {
			if req.Works[i].TraceID != "" {
				req.Works[i].CurrentChainingTraceID = req.Works[i].TraceID
			} else {
				req.Works[i].CurrentChainingTraceID = traceID
			}
		}
		if req.Works[i].TraceID == "" {
			req.Works[i].TraceID = req.Works[i].CurrentChainingTraceID
		}
	}
}

func generatedPayloadToRawMessage(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func submitWorkBadRequestMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := err.Error()
	if strings.HasPrefix(message, "work_request:") {
		return submitWorkTypeNameMessage(message), true
	}
	if strings.Contains(message, "unknown work type") || strings.Contains(message, "work type") && strings.Contains(message, "not found") {
		return submitWorkTypeNameMessage(message), true
	}
	return "", false
}

func submitWorkTypeNameMessage(message string) string {
	message = strings.ReplaceAll(message, "work_type_name", "workTypeName")
	message = strings.ReplaceAll(message, "work_type_id", "workTypeName")
	if strings.Contains(message, "work type name") {
		return message
	}
	return strings.ReplaceAll(message, "work type", "work type name")
}

func translateCanonicalWorkRequestValidationError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	message = strings.TrimPrefix(message, "work request batch ")
	message = strings.ReplaceAll(message, " uses retired work_type_id field; use workTypeName", ".work_type_id is not supported; use workTypeName")
	message = strings.ReplaceAll(message, " uses retired target_state field; use state", ".target_state is not supported; use state")
	if strings.HasPrefix(message, "works[") && strings.Contains(message, "] ") {
		message = strings.Replace(message, "] ", "].", 1)
	}
	if strings.HasSuffix(message, ".work_type_id is not supported; use workTypeName") ||
		strings.HasSuffix(message, ".target_state is not supported; use state") {
		return requestFieldValidationError{message: message}
	}
	switch message {
	case "uses retired work_type_id field; use workTypeName":
		return requestFieldValidationError{message: "work_type_id is not supported; use workTypeName"}
	case "uses retired target_state field; use state":
		return requestFieldValidationError{message: "target_state is not supported; use state"}
	default:
		return requestFieldValidationError{message: message}
	}
}
