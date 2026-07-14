// Package ndjsoncontract defines the supported CLI response-stream NDJSON
// recordType vocabulary and helpers for rejecting retired private records.
package ndjsoncontract

import "fmt"

// RetiredPrivateRecordTypes are unsupported CLI response-stream recordType values
// removed in Batch 09 Story 23.
var RetiredPrivateRecordTypes = []string{
	"progress",
	"compaction",
	"primary_result",
	"stream_gap",
}

// SupportedRecordTypes are the only supported CLI response-stream recordType values.
var SupportedRecordTypes = []string{
	"response_event",
	"invocation_result",
}

// RejectRetiredPrivateRecordType returns an error when recordType names a retired
// private CLI NDJSON record.
func RejectRetiredPrivateRecordType(recordType string) error {
	for _, retired := range RetiredPrivateRecordTypes {
		if recordType == retired {
			return fmt.Errorf("unsupported retired private CLI NDJSON recordType %q", recordType)
		}
	}
	return nil
}

// IsSupportedRecordType reports whether recordType is a supported public CLI NDJSON record.
func IsSupportedRecordType(recordType string) bool {
	for _, supported := range SupportedRecordTypes {
		if recordType == supported {
			return true
		}
	}
	return false
}
