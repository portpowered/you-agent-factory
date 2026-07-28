package http

import (
	"strings"
	"testing"
)

func TestMoveWorkRequestFromBody_RejectsUnsupportedFields(t *testing.T) {
	t.Parallel()

	_, err := MoveWorkRequestFromBody(strings.NewReader(`{"stateName":"complete","extraField":true}`))
	if err == nil {
		t.Fatal("MoveWorkRequestFromBody() error = nil, want unsupported field rejection")
	}
	if _, ok := requestFieldValidationMessage(err); !ok {
		t.Fatalf("error = %v, want request field validation error", err)
	}
}

func TestMoveWorkRequestFromBody_DecodesStateNameAndRequestID(t *testing.T) {
	t.Parallel()

	req, err := MoveWorkRequestFromBody(strings.NewReader(`{"stateName":"complete","requestId":"move-1"}`))
	if err != nil {
		t.Fatalf("MoveWorkRequestFromBody() error = %v", err)
	}
	if req.StateName != "complete" || req.RequestId == nil || *req.RequestId != "move-1" {
		t.Fatalf("request = %#v, want stateName complete and requestId move-1", req)
	}
}

func TestMoveWorkRequestFromBody_RejectsEmptyBody(t *testing.T) {
	t.Parallel()

	_, err := MoveWorkRequestFromBody(strings.NewReader(""))
	if err == nil {
		t.Fatal("MoveWorkRequestFromBody() error = nil, want empty body rejection")
	}
}
