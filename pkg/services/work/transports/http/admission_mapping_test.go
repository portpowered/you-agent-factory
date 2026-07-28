package http

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestStageContentRequestFromAPI_DecodesBase64Payload(t *testing.T) {
	t.Parallel()

	request, err := StageContentRequestFromAPI(factoryapi.StageSubmitWorkFileRequest{
		ItemType:      factoryapi.SubmitWorkItemTypeImage,
		FileName:      "ui.png",
		MediaType:     "image/png",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("png-bytes")),
	})
	if err != nil {
		t.Fatalf("StageContentRequestFromAPI: %v", err)
	}
	if request.ItemType != "image" || string(request.Content) != "png-bytes" {
		t.Fatalf("request = %#v, want decoded stage content", request)
	}
}

func TestStageContentRequestFromAPI_RejectsInvalidBase64(t *testing.T) {
	t.Parallel()

	_, err := StageContentRequestFromAPI(factoryapi.StageSubmitWorkFileRequest{
		ItemType:      factoryapi.SubmitWorkItemTypeImage,
		FileName:      "ui.png",
		MediaType:     "image/png",
		ContentBase64: "not-base64!!!",
	})
	if err == nil || !strings.Contains(err.Error(), "contentBase64 must be valid base64") {
		t.Fatalf("error = %v, want invalid base64 validation", err)
	}
}

func TestWorkRequestFromUpsertAPI_MapsBatchRequest(t *testing.T) {
	t.Parallel()

	workTypeName := "prd"
	request, err := WorkRequestFromUpsertAPI(factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{
		RequestId: "request-1",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "draft",
			WorkTypeName: &workTypeName,
		}},
	})
	if err != nil {
		t.Fatalf("WorkRequestFromUpsertAPI: %v", err)
	}
	if request.RequestID != "request-1" || len(request.Works) != 1 || request.Works[0].WorkTypeID != "prd" {
		t.Fatalf("request = %#v, want mapped upsert request", request)
	}
}

func TestSubmitWorkResponseToAPI_EncodesDetachedResult(t *testing.T) {
	t.Parallel()

	response := SubmitWorkResponseToAPI(work.WorkRequestSubmitResult{
		RequestID:    "request-1",
		TraceID:      "trace-1",
		WorkID:       "work-1",
		Name:         "draft",
		WorkTypeName: "prd",
		Accepted:     true,
	}, "session-1")
	if response.RequestId != "request-1" || response.TraceId != "trace-1" || !response.Accepted {
		t.Fatalf("response = %#v, want encoded submit response", response)
	}
	if response.SessionId == nil || *response.SessionId != "session-1" {
		t.Fatalf("response = %#v, want session id", response)
	}
}

func TestUpsertWorkResponseToAPI_EncodesDetachedWorks(t *testing.T) {
	t.Parallel()

	response := UpsertWorkResponseToAPI(work.WorkRequestSubmitResult{
		RequestID: "request-1",
		TraceID:   "trace-1",
		Works: []work.WorkRequestSubmittedWork{{
			Name:         "draft",
			WorkTypeName: "prd",
			WorkID:       "work-1",
		}},
	})
	if response.RequestId != "request-1" || len(response.Works) != 1 || response.Works[0].WorkId != "work-1" {
		t.Fatalf("response = %#v, want encoded upsert response", response)
	}
}

func TestStageSubmitWorkFileRequestFromBody_RejectsUnsupportedFields(t *testing.T) {
	t.Parallel()

	_, err := StageSubmitWorkFileRequestFromBody(strings.NewReader(`{
		"itemType":"image",
		"fileName":"ui.png",
		"mediaType":"image/png",
		"contentBase64":"cG5n",
		"extra":"field"
	}`))
	if err == nil || !strings.Contains(err.Error(), "extra is not supported") {
		t.Fatalf("error = %v, want unsupported field validation", err)
	}
}
