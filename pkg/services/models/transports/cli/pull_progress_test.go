package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestModelPullProgressPresenterRendersThrottledBytesAndStops(t *testing.T) {
	start := time.Unix(100, 0)
	current := start
	var output bytes.Buffer
	baseContext, cancel := context.WithCancel(context.Background())

	ctx, stop := startModelPullProgress(
		baseContext, "voice", modelPullProgressPhasePull, &output,
		time.Second, func() time.Time { return current }, true,
	)

	lines := progressLines(output.String())
	if len(lines) != 1 || !strings.Contains(lines[0], `modelName="voice" phase=pull elapsed=0s`) {
		t.Fatalf("initial progress = %#v, want one pull-phase heartbeat", lines)
	}

	models.ReportPullProgress(ctx, models.PullProgressObservation{
		Artifact: "model.bin", TransferredBytes: 25, TotalBytes: 100,
	})
	if got := len(progressLines(output.String())); got != 2 {
		t.Fatalf("first artifact progress = %d lines, want initial heartbeat plus artifact", got)
	}

	current = start.Add(500 * time.Millisecond)
	models.ReportPullProgress(ctx, models.PullProgressObservation{
		Artifact: "model.bin", TransferredBytes: 40, TotalBytes: 100,
	})
	if got := len(progressLines(output.String())); got != 2 {
		t.Fatalf("progress before interval = %d lines, want throttled to two", got)
	}

	current = start.Add(time.Second)
	models.ReportPullProgress(ctx, models.PullProgressObservation{
		Artifact: "model.bin", TransferredBytes: 50, TotalBytes: 100,
	})
	lines = progressLines(output.String())
	if len(lines) != 3 || !strings.Contains(lines[2], `artifact="model.bin" transferredBytes=50 totalBytes=100 percent=50.0%`) {
		t.Fatalf("throttled progress = %#v, want latest byte totals", lines)
	}

	cancel()
	stop()
	models.ReportPullProgress(ctx, models.PullProgressObservation{
		Artifact: "model.bin", TransferredBytes: 100, TotalBytes: 100,
	})
	if got := len(progressLines(output.String())); got != 3 {
		t.Fatalf("progress after stop = %d lines, want no further output", got)
	}
}

func TestModelPullProgressPresenterUsesPreparationPhaseAndJSONGating(t *testing.T) {
	start := time.Unix(200, 0)
	var output bytes.Buffer
	ctx, stop := startModelPullProgress(
		context.Background(), "asr", modelPullProgressPhasePreparation, &output,
		time.Hour, func() time.Time { return start }, true,
	)
	models.ReportPullProgress(ctx, models.PullProgressObservation{
		Artifact: "weights.safetensors", TransferredBytes: 1, TotalBytes: 2,
	})
	stop()
	if got := output.String(); !strings.Contains(got, `phase=preparation`) {
		t.Fatalf("preparation progress = %q, want preparation phase", got)
	}

	var jsonOutput bytes.Buffer
	jsonCtx, stopJSON := startModelPullProgress(
		context.Background(), "asr", modelPullProgressPhasePreparation, &jsonOutput,
		time.Second, func() time.Time { return start }, false,
	)
	models.ReportPullProgress(jsonCtx, models.PullProgressObservation{
		Artifact: "weights.safetensors", TransferredBytes: 1, TotalBytes: 2,
	})
	stopJSON()
	if got := jsonOutput.String(); got != "" {
		t.Fatalf("JSON-gated progress = %q, want no stderr progress", got)
	}
}

func progressLines(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
