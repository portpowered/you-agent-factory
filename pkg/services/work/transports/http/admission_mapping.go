package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"
)

type decodedSubmitWorkRequest struct {
	Request factoryapi.SubmitWorkBySessionIdJSONRequestBody
}

type decodedUpsertWorkRequest struct {
	Request factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody
}

// StageSubmitWorkFileRequestFromBody decodes one stage-submit-work-file request.
func StageSubmitWorkFileRequestFromBody(body io.Reader) (factoryapi.StageSubmitWorkFileRequest, error) {
	var rawFields map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&rawFields); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	if err := validateStageSubmitWorkFileRequestFields(rawFields); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}

	payload, err := json.Marshal(rawFields)
	if err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	var req factoryapi.StageSubmitWorkFileRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	return req, nil
}

func validateStageSubmitWorkFileRequestFields(fields map[string]json.RawMessage) error {
	if err := requireOnlyFields(fields, "", "itemType", "fileName", "mediaType", "contentBase64"); err != nil {
		return err
	}

	itemType, err := requiredStageSubmitWorkItemType(fields)
	if err != nil {
		return err
	}
	switch itemType {
	case factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
	default:
		return requestFieldValidationError{message: "itemType must be one of image, video, audio, or document"}
	}

	if _, err := requiredNonEmptyStringField(fields, "", "fileName", "submit-work staged files"); err != nil {
		return err
	}
	if _, err := requiredNonEmptyStringField(fields, "", "mediaType", "submit-work staged files"); err != nil {
		return err
	}
	if _, err := requiredNonEmptyStringField(fields, "", "contentBase64", "submit-work staged files"); err != nil {
		return err
	}
	return nil
}

func requiredStageSubmitWorkItemType(
	fields map[string]json.RawMessage,
) (factoryapi.SubmitWorkItemType, error) {
	itemTypeRaw, ok := fields["itemType"]
	if !ok {
		return "", requestFieldValidationError{message: "itemType is required"}
	}

	var itemType string
	if err := json.Unmarshal(itemTypeRaw, &itemType); err != nil || itemType == "" {
		return "", requestFieldValidationError{message: "itemType must be a non-empty string"}
	}

	switch factoryapi.SubmitWorkItemType(itemType) {
	case factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
		return factoryapi.SubmitWorkItemType(itemType), nil
	case factoryapi.SubmitWorkItemTypeText:
		return "", requestFieldValidationError{
			message: "itemType must be one of image, video, audio, or document",
		}
	default:
		return "", requestFieldValidationError{
			message: "itemType must be one of image, video, audio, or document",
		}
	}
}

// StageContentRequestFromAPI maps one decoded stage request into a Work root
// staging call.
func StageContentRequestFromAPI(req factoryapi.StageSubmitWorkFileRequest) (work.StageContentRequest, error) {
	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		return work.StageContentRequest{}, requestFieldValidationError{
			message: "contentBase64 must be valid base64",
		}
	}
	if len(content) == 0 {
		return work.StageContentRequest{}, requestFieldValidationError{
			message: "contentBase64 must decode to a non-empty file payload",
		}
	}
	return work.StageContentRequest{
		ItemType:  string(req.ItemType),
		FileName:  req.FileName,
		MediaType: req.MediaType,
		Content:   content,
	}, nil
}

// StageSubmitWorkFileResponseToAPI encodes detached Work staging results into
// the public HTTP success shape.
func StageSubmitWorkFileResponseToAPI(result work.StageContentResult) factoryapi.StageSubmitWorkFileResponse {
	return factoryapi.StageSubmitWorkFileResponse{
		FileName:      result.FileName,
		MediaType:     result.MediaType,
		StagedFileRef: result.StagedFileRef,
		Url:           factoryapi.SubmitWorkContentURLProperty(result.URL),
	}
}

// SubmitWorkRequestFromBody decodes one submit-work request body.
func SubmitWorkRequestFromBody(body io.Reader) (decodedSubmitWorkRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return decodedSubmitWorkRequest{}, err
	}

	var req factoryapi.SubmitWorkBySessionIdJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	if err := validateSubmitWorkStructuredInputFields(fields); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	return decodedSubmitWorkRequest{Request: req}, nil
}

// UpsertWorkRequestFromBody decodes one upsert-work-request body.
func UpsertWorkRequestFromBody(body io.Reader) (decodedUpsertWorkRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	if !json.Valid(data) {
		var invalid any
		return decodedUpsertWorkRequest{}, json.Unmarshal(data, &invalid)
	}
	decodedData, err := normalizeWorkRequestStateJSON(data)
	if err != nil {
		return decodedUpsertWorkRequest{}, err
	}

	var req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody
	if err := json.Unmarshal(decodedData, &req); err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	var rawRequest struct {
		Works []map[string]json.RawMessage `json:"works"`
	}
	if err := json.Unmarshal(data, &rawRequest); err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	for index := range rawRequest.Works {
		if err := validateWorkContentField(
			rawRequest.Works[index],
			fmt.Sprintf("works[%d].", index),
		); err != nil {
			return decodedUpsertWorkRequest{}, err
		}
	}
	return decodedUpsertWorkRequest{Request: req}, nil
}

func normalizeWorkRequestStateJSON(data []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}

	var works []map[string]json.RawMessage
	if rawWorks, ok := request["works"]; ok {
		if err := json.Unmarshal(rawWorks, &works); err != nil {
			return nil, err
		}
	}
	for _, workEntry := range works {
		rawState, ok := workEntry["state"]
		if !ok {
			continue
		}
		var stateName string
		if err := json.Unmarshal(rawState, &stateName); err != nil {
			continue
		}
		canonicalState, err := json.Marshal(factoryapi.WorkState{
			Name: stateName,
			Type: factoryapi.WorkStateTypePROCESSING,
		})
		if err != nil {
			return nil, err
		}
		workEntry["state"] = canonicalState
	}
	if len(works) > 0 {
		normalizedWorks, err := json.Marshal(works)
		if err != nil {
			return nil, err
		}
		request["works"] = normalizedWorks
	}
	return json.Marshal(request)
}

func validateSubmitWorkStructuredInputFields(fields map[string]json.RawMessage) error {
	if _, ok := fields["items"]; !ok {
		return nil
	}
	return validateSubmitWorkItemsField(fields, "")
}

func validateSubmitWorkItemsField(fields map[string]json.RawMessage, prefix string) error {
	itemsRaw, ok := fields["items"]
	if !ok {
		return nil
	}

	var itemPayloads []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &itemPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%sitems must be an array", prefix)}
	}
	if len(itemPayloads) == 0 {
		return nil
	}

	for i, payload := range itemPayloads {
		var itemFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &itemFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%sitems[%d] must be an object", prefix, i)}
		}
		if _, err := validateSubmitWorkItemField(itemFields, fmt.Sprintf("%sitems[%d].", prefix, i)); err != nil {
			return err
		}
	}

	return nil
}

func validateSubmitWorkItemField(fields map[string]json.RawMessage, prefix string) (bool, error) {
	itemType, err := requiredSubmitWorkItemType(fields, prefix)
	if err != nil {
		return false, err
	}

	switch itemType {
	case factoryapi.SubmitWorkItemTypeText:
		if err := requireOnlyFields(fields, prefix, "type", "text"); err != nil {
			return false, err
		}
		if _, err := requiredStringField(fields, prefix, "text", "text items"); err != nil {
			return false, err
		}
		return true, nil
	case factoryapi.SubmitWorkItemTypeImage, factoryapi.SubmitWorkItemTypeVideo, factoryapi.SubmitWorkItemTypeAudio, factoryapi.SubmitWorkItemTypeDocument:
		if err := requireOnlyFields(fields, prefix, "type", "url", "stagedFileRef", "fileName", "mediaType"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "url", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "stagedFileRef", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "fileName", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "mediaType", string(itemType)+" items"); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

func requiredSubmitWorkItemType(fields map[string]json.RawMessage, prefix string) (factoryapi.SubmitWorkItemType, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var itemType string
	if err := json.Unmarshal(typeRaw, &itemType); err != nil || itemType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	switch factoryapi.SubmitWorkItemType(itemType) {
	case factoryapi.SubmitWorkItemTypeText,
		factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
		return factoryapi.SubmitWorkItemType(itemType), nil
	default:
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

// WorkRequestFromSubmitAPI maps one decoded submit-work request into a Work
// root admission call. Content items are resolved through the accepted root when
// present.
func WorkRequestFromSubmitAPI(
	ctx context.Context,
	root work.Service,
	req factoryapi.SubmitWorkBySessionIdJSONRequestBody,
) (work.WorkRequest, error) {
	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		return work.WorkRequest{}, err
	}
	content, err := submitWorkContentFromAPI(ctx, root, req)
	if err != nil {
		return work.WorkRequest{}, err
	}

	submitReq := work.SubmitRequest{
		Name:                   strings.TrimSpace(stringValue(req.Name)),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                stringValue(req.TraceId),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	return work.WorkRequestFromSubmitRequests([]work.SubmitRequest{submitReq}), nil
}

func submitWorkContentFromAPI(
	ctx context.Context,
	root work.Service,
	req factoryapi.SubmitWorkBySessionIdJSONRequestBody,
) ([]work.WorkContentPart, error) {
	if req.Items == nil {
		return contentcontract.PartsFromGenerated(req.Content), nil
	}
	if root == nil {
		return nil, fmt.Errorf("work service is required")
	}
	items, err := stagedSubmissionItemsFromGenerated(*req.Items)
	if err != nil {
		return nil, err
	}
	return root.PrepareContent(ctx, items)
}

func stagedSubmissionItemsFromGenerated(
	items []factoryapi.SubmitWorkItem,
) ([]work.StagedSubmissionItem, error) {
	result := make([]work.StagedSubmissionItem, 0, len(items))
	for i, item := range items {
		mapped, err := stagedSubmissionItemFromGenerated(item)
		if err != nil {
			return nil, requestFieldValidationError{message: fmt.Sprintf("items[%d]: %v", i, err)}
		}
		result = append(result, mapped)
	}
	return result, nil
}

func stagedSubmissionItemFromGenerated(
	item factoryapi.SubmitWorkItem,
) (work.StagedSubmissionItem, error) {
	textItem, textErr := item.AsSubmitWorkTextItem()
	if textErr == nil && textItem.Type == factoryapi.SubmitWorkItemTypeText {
		return work.StagedSubmissionItem{
			ItemType: string(textItem.Type),
			Text:     textItem.Text,
		}, nil
	}

	imageItem, imageErr := item.AsSubmitWorkImageItem()
	if imageErr == nil && imageItem.Type == factoryapi.SubmitWorkItemTypeImage {
		return stagedSubmissionFileItem(
			string(imageItem.Type), imageItem.StagedFileRef,
			imageItem.FileName, imageItem.MediaType,
		), nil
	}

	videoItem, videoErr := item.AsSubmitWorkVideoItem()
	if videoErr == nil && videoItem.Type == factoryapi.SubmitWorkItemTypeVideo {
		return stagedSubmissionFileItem(
			string(videoItem.Type), videoItem.StagedFileRef,
			videoItem.FileName, videoItem.MediaType,
		), nil
	}

	audioItem, audioErr := item.AsSubmitWorkAudioItem()
	if audioErr == nil && audioItem.Type == factoryapi.SubmitWorkItemTypeAudio {
		return stagedSubmissionFileItem(
			string(audioItem.Type), audioItem.StagedFileRef,
			audioItem.FileName, audioItem.MediaType,
		), nil
	}

	documentItem, documentErr := item.AsSubmitWorkDocumentItem()
	if documentErr == nil && documentItem.Type == factoryapi.SubmitWorkItemTypeDocument {
		return stagedSubmissionFileItem(
			string(documentItem.Type), documentItem.StagedFileRef,
			documentItem.FileName, documentItem.MediaType,
		), nil
	}

	return work.StagedSubmissionItem{}, fmt.Errorf("unsupported item type")
}

func stagedSubmissionFileItem(
	itemType string,
	stagedFileRef string,
	fileName string,
	mediaType string,
) work.StagedSubmissionItem {
	return work.StagedSubmissionItem{
		ItemType: itemType, StagedFileRef: stagedFileRef,
		FileName: fileName, MediaType: mediaType,
	}
}

// WorkRequestFromUpsertAPI maps one decoded upsert-work-request body into a
// Work root admission call.
func WorkRequestFromUpsertAPI(req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody) (work.WorkRequest, error) {
	workRequest := work.WorkRequest{
		RequestID:              req.RequestId,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		Type:                   work.WorkRequestType(req.Type),
	}
	if req.Works != nil {
		workRequest.Works = make([]work.Work, 0, len(*req.Works))
		for _, workItem := range *req.Works {
			workRequest.Works = append(workRequest.Works, work.Work{
				Name:                     workItem.Name,
				WorkID:                   stringValue(workItem.WorkId),
				RequestID:                stringValue(workItem.RequestId),
				WorkTypeID:               stringValue(workItem.WorkTypeName),
				State:                    generatedWorkStateName(workItem.State),
				ChainingTraceDepth:       intValue(workItem.ChainingTraceDepth),
				CurrentChainingTraceID:   stringValue(workItem.CurrentChainingTraceId),
				PreviousChainingTraceIDs: stringSliceValue(workItem.PreviousChainingTraceIds),
				TraceID:                  stringValue(workItem.TraceId),
				Content:                  contentcontract.PartsFromGenerated(workItem.Content),
				Payload:                  workItem.Payload,
				Tags:                     generatedStringMap(workItem.Tags),
			})
		}
	}
	if req.Relations != nil {
		workRequest.Relations = make([]work.WorkRelation, 0, len(*req.Relations))
		for _, relation := range *req.Relations {
			workRequest.Relations = append(workRequest.Relations, work.WorkRelation{
				Type:           work.WorkRelationType(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  stringValue(relation.RequiredState),
			})
		}
	}
	return workRequest, nil
}

// SubmitWorkResponseToAPI encodes detached Work admission results into the
// public submit-work HTTP success shape.
func SubmitWorkResponseToAPI(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	resp := factoryapi.SubmitWorkResponse{
		TraceId:   result.TraceID,
		RequestId: result.RequestID,
		Accepted:  result.Accepted,
	}
	if result.WorkID != "" {
		resp.WorkId = &result.WorkID
	}
	if result.Name != "" {
		resp.Name = &result.Name
	}
	if result.WorkTypeName != "" {
		resp.WorkTypeName = &result.WorkTypeName
	}
	if sessionID != "" {
		resp.SessionId = &sessionID
	}
	return resp
}

// UpsertWorkResponseToAPI encodes detached Work admission results into the
// public upsert-work-request HTTP success shape.
func UpsertWorkResponseToAPI(result work.WorkRequestSubmitResult) factoryapi.UpsertWorkRequestResponse {
	works := make([]factoryapi.UpsertWorkRequestSubmittedWork, 0, len(result.Works))
	for _, workItem := range result.Works {
		works = append(works, factoryapi.UpsertWorkRequestSubmittedWork{
			Name:         workItem.Name,
			WorkTypeName: workItem.WorkTypeName,
			WorkId:       workItem.WorkID,
		})
	}
	return factoryapi.UpsertWorkRequestResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Works:     works,
	}
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func generatedSubmitRelations(values *[]factoryapi.SubmitRelation) []work.Relation {
	if values == nil || len(*values) == 0 {
		return nil
	}
	relations := make([]work.Relation, 0, len(*values))
	for _, relation := range *values {
		relations = append(relations, work.Relation{
			Type:          work.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkId,
			RequiredState: stringValue(relation.RequiredState),
		})
	}
	return relations
}

func generatedPayloadToRawMessage(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}
