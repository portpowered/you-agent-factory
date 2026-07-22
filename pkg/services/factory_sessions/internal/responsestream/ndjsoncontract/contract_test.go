package ndjsoncontract_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/ndjsoncontract"
)

func TestRejectRetiredPrivateRecordType_RejectsRetiredValues(t *testing.T) {
	t.Parallel()

	for _, recordType := range ndjsoncontract.RetiredPrivateRecordTypes {
		recordType := recordType
		t.Run(recordType, func(t *testing.T) {
			t.Parallel()
			if err := ndjsoncontract.RejectRetiredPrivateRecordType(recordType); err == nil {
				t.Fatalf("RejectRetiredPrivateRecordType(%q) = nil, want error", recordType)
			}
		})
	}
}

func TestRejectRetiredPrivateRecordType_AllowsSupportedValues(t *testing.T) {
	t.Parallel()

	for _, recordType := range ndjsoncontract.SupportedRecordTypes {
		recordType := recordType
		t.Run(recordType, func(t *testing.T) {
			t.Parallel()
			if err := ndjsoncontract.RejectRetiredPrivateRecordType(recordType); err != nil {
				t.Fatalf("RejectRetiredPrivateRecordType(%q) = %v, want nil", recordType, err)
			}
		})
	}
}

func TestIsSupportedRecordType(t *testing.T) {
	t.Parallel()

	if !ndjsoncontract.IsSupportedRecordType("response_event") {
		t.Fatal("response_event must be supported")
	}
	if !ndjsoncontract.IsSupportedRecordType("invocation_result") {
		t.Fatal("invocation_result must be supported")
	}
	if ndjsoncontract.IsSupportedRecordType("progress") {
		t.Fatal("progress must not be supported")
	}
}
