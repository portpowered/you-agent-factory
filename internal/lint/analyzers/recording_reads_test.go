package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRecordingReadsTypedReferences(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), RecordingReads, "m/pkg/recordingconsumer", "m/pkg/testdata/recordingcaller")
}

func TestRecordingReadsExactOwners(t *testing.T) {
	useFixtures(t)
	old := recordingReadOwners
	recordingReadOwners = setOf("pkg/recordingallowed#Explicit")
	t.Cleanup(func() { recordingReadOwners = old })
	analysistest.Run(t, analysistest.TestData(), RecordingReads, "m/pkg/recordingallowed")
}

func TestRecordingReadsExactDebt(t *testing.T) {
	useFixtures(t,
		"recording-read|pkg/recordingdebt|pkg/recordingdebt/reads.go#Listed#QueryHistoricalRecording::count=1",
		"recording-read|pkg/recordingdebt|pkg/recordingdebt/reads.go#Duplicate#QueryHistoricalRecording::count=1",
		"recording-read|pkg/recordingdebt|pkg/recordingdebt/reads.go#Removed#QueryHistoricalRecording::count=1",
	)
	// The old duplicate allowance must itself be deleted when its count changes.
	analysistest.Run(t, analysistest.TestData(), RecordingReads, "m/pkg/recordingdebt")
}

func TestRecordingReadsDebtCannotBeReplaced(t *testing.T) {
	t.Parallel()
	old := "recording-read|pkg/a|a.go#Old#LoadWorkerRecording::count=1"
	newSite := "recording-read|pkg/a|a.go#New#LoadWorkerRecording::count=1"
	if _, err := CompareBaselineGrowth(old, newSite); err == nil {
		t.Fatal("a removed recording read must not authorize a new site")
	}
	if _, err := CompareBaselineGrowth(old, old+"\n"+newSite); err == nil {
		t.Fatal("existing debt must not authorize an additional read site")
	}
	twice := "recording-read|pkg/a|a.go#Old#LoadWorkerRecording::count=2"
	if _, err := CompareBaselineGrowth(twice, old); err != nil {
		t.Fatalf("rejecting a count reduction: %v", err)
	}
	if _, err := CompareBaselineGrowth(old, twice); err == nil {
		t.Fatal("existing debt must not authorize another occurrence")
	}
	if _, err := CompareBaselineGrowth(recordingReadMigration, recordingReadMigration+"\n"+old); err == nil {
		t.Fatal("zero established debt must not authorize reseeding")
	}
	if _, err := CompareBaselineGrowth(recordingReadMigration, ""); err == nil {
		t.Fatal("the migration marker must survive removal of the last allowance")
	}
}
