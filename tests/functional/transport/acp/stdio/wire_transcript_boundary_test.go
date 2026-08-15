package stdio_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/rollingfile"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
)

// TestACPWireTranscriptBoundaryKeepsRotationAndFailureTruthful exercises the
// customer-visible wire/transcript boundary at the two lifecycle edges that
// ordinary ACP traffic does not reach: a healthy rolling rotation and a failed
// outbound write. The observing sink models the ACP peer reading stdout as
// soon as the transport publishes it.
func TestACPWireTranscriptBoundaryKeepsRotationAndFailureTruthful(t *testing.T) {
	t.Parallel()

	t.Run("rotation records before publication", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "transcript.jsonl")
		seed := bytes.Repeat([]byte("x"), 1024*1024-1)
		if err := os.WriteFile(path, seed, 0o600); err != nil {
			t.Fatalf("WriteFile(): %v", err)
		}
		transcript := wiretranscript.NewWriter(&rollingfile.Writer{Filename: path, MaxSize: 1}, boundaryClock{})
		out := &boundaryObservingWriter{path: path, marker: []byte(`"frame":{"id":1}`)}
		tee := wiretranscript.TeeWriter(out, transcript, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)
		frame := []byte(`{"id":1}` + "\n")

		n, err := tee.Write(frame)
		if err != nil || n != len(frame) {
			t.Fatalf("TeeWriter.Write() = (%d, %v), want complete publication", n, err)
		}
		if err := transcript.Close(); err != nil {
			t.Fatalf("transcript.Close() error = %v", err)
		}
		active, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(active): %v", err)
		}
		if !bytes.Contains(active, out.marker) {
			t.Fatalf("active transcript = %q, want outbound record before publication", active)
		}
	})

	t.Run("failed write rolls back", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "transcript.jsonl")
		transcript := wiretranscript.NewWriter(&rollingfile.Writer{Filename: path, MaxSize: 1}, boundaryClock{})
		wantErr := errors.New("peer write failed")
		tee := wiretranscript.TeeWriter(boundaryErrorWriter{err: wantErr}, transcript, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)

		n, err := tee.Write([]byte(`{"id":2}` + "\n"))
		if n != 0 || !errors.Is(err, wantErr) {
			t.Fatalf("TeeWriter.Write() = (%d, %v), want (0, %v)", n, err, wantErr)
		}
		if err := transcript.Close(); err != nil {
			t.Fatalf("transcript.Close() error = %v", err)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("failed outbound write left a transcript record, stat error = %v", statErr)
		}
	})
}

type boundaryClock struct{}

func (boundaryClock) Now() time.Time { return time.Unix(1700000000, 0).UTC() }

type boundaryObservingWriter struct {
	path   string
	marker []byte
	bytes.Buffer
}

func (w *boundaryObservingWriter) Write(p []byte) (int, error) {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return 0, err
	}
	if !bytes.Contains(data, w.marker) {
		return 0, errors.New("outbound bytes exposed before transcript record")
	}
	return w.Buffer.Write(p)
}

type boundaryErrorWriter struct{ err error }

func (w boundaryErrorWriter) Write([]byte) (int, error) { return 0, w.err }
