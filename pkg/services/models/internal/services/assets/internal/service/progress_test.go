package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

type progressWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (writer *progressWriteCloser) Close() error {
	writer.closed = true
	return nil
}

func TestCopyStagedAssetReportsTransferredBytesThroughRequestObserver(t *testing.T) {
	var observations []models.PullProgressObservation
	ctx := models.WithPullProgressObserver(context.Background(), func(observation models.PullProgressObservation) {
		observations = append(observations, observation)
	})
	progress := newAssetProgress(ctx, "voice", "model.bin", int64(len("payload")))
	var output progressWriteCloser

	written, _, err := copyStagedAssetWithProgress(
		ctx, &output, strings.NewReader("payload"), "model.bin", progress,
	)
	if err != nil {
		t.Fatalf("copyStagedAssetWithProgress() error = %v", err)
	}
	if written != int64(len("payload")) || output.String() != "payload" || !output.closed {
		t.Fatalf("copy result = written:%d output:%q closed:%t", written, output.String(), output.closed)
	}
	if len(observations) < 2 {
		t.Fatalf("progress observations = %#v, want initial and transferred observations", observations)
	}
	last := observations[len(observations)-1]
	if last.ModelName != "voice" || last.Artifact != "model.bin" ||
		last.TransferredBytes != int64(len("payload")) || last.TotalBytes != int64(len("payload")) {
		t.Fatalf("last progress observation = %#v", last)
	}
}
