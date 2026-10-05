package stresstests

import (
	"encoding/json"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	"strings"
	"testing"
)

func TestASRCodecAcceptsLargeTranscriptAndSegmentOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("16 MiB transcript scale profile")
	}
	text := strings.Repeat("spoken words ", (16<<20)/len("spoken words ")+1)
	response := codecs.ASRResponse{Text: text, Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1, Text: text}}}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := codecs.NewASRCodec().DecodeResponse(payload)
	if err != nil || len(outputs) != 2 {
		t.Fatalf("large ASR response output count=%d error=%v", len(outputs), err)
	}
	if outputs[0].Name != "transcript" || outputs[0].Content != text {
		t.Fatal("large transcript was lost or truncated")
	}
	var segments []codecs.ASRSegment
	if err := json.Unmarshal([]byte(outputs[1].Content), &segments); err != nil || len(segments) != 1 || segments[0] != response.Segments[0] {
		t.Fatalf("large segment output was lost or truncated: %v", err)
	}
}
func TestASRCodecAcceptsMoreThanMillionSegments(t *testing.T) {
	if testing.Short() {
		t.Skip("million-segment scale profile")
	}
	const count = 1<<20 + 1
	response := codecs.ASRResponse{Text: "long recording", Segments: make([]codecs.ASRSegment, count)}
	for index := range response.Segments {
		response.Segments[index] = codecs.ASRSegment{ID: int32(index), Start: int64(index), End: int64(index + 1), Text: "word"}
	}
	outputs, err := codecs.NewASRCodec().DecodeResponseValue(response)
	if err != nil || len(outputs) != 2 {
		t.Fatalf("long ASR segmentation output count=%d error=%v", len(outputs), err)
	}
	if strings.Count(outputs[1].Content, `"id":`) != count {
		t.Fatal("long segment output lost timestamped entries")
	}
	last, err := json.Marshal(response.Segments[count-1])
	if err != nil || !strings.HasSuffix(outputs[1].Content, string(last)+"]") {
		t.Fatal("long segment output lost its final timestamped entry")
	}
}
