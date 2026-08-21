package requestadmission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWorkPayloadAdmissionAcceptsAbsentAndExactMultibytePayload(t *testing.T) {
	t.Parallel()

	if err := validateWorkPayloadSize("absent", "work-absent", nil); err != nil {
		t.Fatalf("absent payload error = %v, want nil", err)
	}

	payload := json.RawMessage(`"` + strings.Repeat("é", (MaxWorkPayloadBytes-2)/2) + `"`)
	if len(payload) != MaxWorkPayloadBytes {
		t.Fatalf("multibyte payload bytes = %d, want %d", len(payload), MaxWorkPayloadBytes)
	}
	if err := validateWorkPayloadSize("multibyte", "work-multibyte", payload); err != nil {
		t.Fatalf("exact multibyte payload error = %v, want nil", err)
	}
}

func TestWorkPayloadAdmissionRejectsOneByteOverWithoutPayloadContent(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`"` + strings.Repeat("x", MaxWorkPayloadBytes-1) + `"`)
	if len(payload) != MaxWorkPayloadBytes+1 {
		t.Fatalf("oversized payload bytes = %d, want %d", len(payload), MaxWorkPayloadBytes+1)
	}
	err := validateWorkPayloadSize("oversized", "work-oversized", payload)
	if err == nil {
		t.Fatal("oversized payload validation error = nil")
	}
	var sizeErr *PayloadSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("error = %T %v, want PayloadSizeError", err, err)
	}
	if sizeErr.PayloadBytes != MaxWorkPayloadBytes+1 || sizeErr.PayloadLimitBytes != MaxWorkPayloadBytes {
		t.Fatalf("size error = %#v", sizeErr)
	}
	if got := err.Error(); !strings.HasPrefix(got, "work_request:") ||
		!strings.Contains(got, `"oversized"`) ||
		!strings.Contains(got, "payloadBytes=65537") ||
		!strings.Contains(got, "payloadLimitBytes=65536") ||
		strings.Contains(got, "xxxxxxxx") {
		t.Fatalf("unsafe or incomplete diagnostic = %q", got)
	}
}

func TestPrepareWorkRequestAppliesPayloadLimitAtCanonicalAdmission(t *testing.T) {
	t.Parallel()

	service, err := NewRequestPreparationService(NewContentPreparation())
	if err != nil {
		t.Fatalf("NewRequestPreparationService: %v", err)
	}
	accepted := json.RawMessage(`"` + strings.Repeat("a", MaxWorkPayloadBytes-2) + `"`)
	prepared, err := service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: Request{
			RequestID: "request-boundary",
			Type:      RequestTypeFactoryRequestBatch,
			Works:     []Work{{Name: "at-limit", WorkTypeID: "task", Payload: accepted}},
		},
	})
	if err != nil {
		t.Fatalf("exact-limit preparation error = %v", err)
	}
	if len(prepared.Works) != 1 {
		t.Fatalf("prepared work count = %d, want 1", len(prepared.Works))
	}

	over := json.RawMessage(`"` + strings.Repeat("b", MaxWorkPayloadBytes-1) + `"`)
	_, err = service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: Request{
			RequestID: "request-over",
			Type:      RequestTypeFactoryRequestBatch,
			Works:     []Work{{Name: "over-limit", WorkTypeID: "task", Payload: over}},
		},
	})
	var sizeErr *PayloadSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("over-limit preparation error = %v, want PayloadSizeError", err)
	}
	if sizeErr.PayloadBytes != MaxWorkPayloadBytes+1 {
		t.Fatalf("over-limit payload bytes = %d, want %d", sizeErr.PayloadBytes, MaxWorkPayloadBytes+1)
	}
}

func TestFactoryRequestBatchPreparationAppliesPayloadLimitBeforeTransport(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`"` + strings.Repeat("q", MaxWorkPayloadBytes-1) + `"`)
	data, err := json.Marshal(Request{
		RequestID: "request-batch-over",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name: "batch-over-limit", WorkTypeID: "task", Payload: payload,
		}},
	})
	if err != nil {
		t.Fatalf("marshal batch = %v", err)
	}
	_, err = NewFactoryRequestBatchPreparation().PrepareFactoryRequestBatch(context.Background(), data)
	var sizeErr *PayloadSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("batch preparation error = %v, want PayloadSizeError", err)
	}
	if sizeErr.PayloadBytes != MaxWorkPayloadBytes+1 {
		t.Fatalf("batch payload bytes = %d, want %d", sizeErr.PayloadBytes, MaxWorkPayloadBytes+1)
	}
}

func TestNormalizeWorkRequestRejectsOversizedBatchBeforeReturningAnyWork(t *testing.T) {
	t.Parallel()

	over := json.RawMessage(`"` + strings.Repeat("z", MaxWorkPayloadBytes-1) + `"`)
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-atomic-size",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "valid-sibling", WorkTypeID: "task", Payload: json.RawMessage(`{"ok":true}`)},
			{Name: "oversized", WorkTypeID: "task", Payload: over},
		},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil {
		t.Fatal("oversized batch normalization error = nil")
	}
	if normalized != nil {
		t.Fatalf("normalized result = %#v, want nil on atomic rejection", normalized)
	}
	var sizeErr *PayloadSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("normalization error = %T %v, want PayloadSizeError", err, err)
	}
	if !strings.Contains(err.Error(), `"oversized"`) ||
		!strings.Contains(err.Error(), "payloadBytes=65537") ||
		!strings.Contains(err.Error(), "payloadLimitBytes=65536") {
		t.Fatalf("normalization diagnostic = %q", err.Error())
	}
}
