package main

import (
	"errors"
	"fmt"
	"strings"
)

func validateFunctionalQuarantineEntryMetadata(entry functionalQuarantineEntry) error {
	if entry.Bucket != functionalBucketEnvironment && entry.Bucket != functionalBucketFailure && entry.Bucket != functionalBucketFlaky {
		return fmt.Errorf("validate functional quarantine: selector %q has unsupported bucket %q; expected %s, %s, or %s", functionalSelectorDisplay(entry), entry.Bucket, functionalBucketEnvironment, functionalBucketFailure, functionalBucketFlaky)
	}
	if _, err := functionalQuarantineMeasurement(entry); err != nil {
		return fmt.Errorf("validate functional quarantine: selector %q has invalid measurement metadata: %w", functionalSelectorDisplay(entry), err)
	}
	if strings.TrimSpace(entry.Reason) == "" {
		return fmt.Errorf("validate functional quarantine: selector %q requires a non-empty reason", functionalSelectorDisplay(entry))
	}
	if entry.Bucket == functionalBucketFailure && strings.TrimSpace(entry.FollowUp) == "" {
		return fmt.Errorf("validate functional quarantine: genuinely failing selector %q requires a non-empty followUp", functionalSelectorDisplay(entry))
	}
	if entry.Bucket == functionalBucketFlaky && strings.TrimSpace(entry.FollowUpIssue) == "" && strings.TrimSpace(entry.FollowUpLane) == "" {
		return fmt.Errorf("validate functional quarantine: FLAKY selector %q requires a non-empty followUpIssue or followUpLane", functionalSelectorDisplay(entry))
	}
	return nil
}

func validateFunctionalQuarantineMetadata(manifest functionalQuarantine) error {
	if manifest.Version != functionalQuarantineVersion {
		return fmt.Errorf("validate functional quarantine: version %d is unsupported; expected %d", manifest.Version, functionalQuarantineVersion)
	}
	if manifest.Suite != functionalSuiteName {
		return fmt.Errorf("validate functional quarantine: suite %q is unsupported; expected %q", manifest.Suite, functionalSuiteName)
	}
	if manifest.Entries == nil {
		return errors.New("validate functional quarantine: entries must be an array")
	}
	for index, entry := range manifest.Entries {
		if err := validateFunctionalQuarantineEntryMetadata(entry); err != nil {
			return fmt.Errorf("entries[%d]: %w", index, err)
		}
	}
	return nil
}
