package recordingallowed

import "m/pkg/services/recordings"

func Explicit(r recordings.Reader) { _, _ = r.QueryHistoricalRecording(recordings.Request{}) }
func Ordinary(r recordings.Reader) {
	_, _ = r.QueryHistoricalRecording(recordings.Request{}) // want "recording-read:.*Ordinary"
}
