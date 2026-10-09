package recordingcaller

import "m/pkg/services/recordings"

func Ordinary(r recordings.Reader) {
	_, _ = r.QueryHistoricalRecording(recordings.Request{}) // want "recording-read:.*Ordinary"
}
