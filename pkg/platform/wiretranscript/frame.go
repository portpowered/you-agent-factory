// Package wiretranscript records newline-delimited JSON-RPC traffic verbatim.
//
// It exists so a customer can answer "what did we actually send and receive"
// from a file on their own machine, and so a captured third-party agent's
// traffic can be compared against ours frame for frame. Both uses depend on
// one property: a recorded frame is the exact bytes observed, never a
// re-encoding. Round-tripping through a decoder would silently drop unknown
// fields and reorder keys, which is precisely the signal a comparison needs.
//
// The package knows JSON-RPC-over-stdio framing and nothing about ACP
// semantics, so it stays usable for either protocol direction and for any
// peer.
package wiretranscript

import "encoding/json"

// FormatVersion is the transcript record schema version. Every consumer reads
// it before interpreting a record; bumping it is a breaking change.
const FormatVersion = 1

// Peer identifies which side authored a frame.
type Peer string

const (
	// PeerClient authored the frame as the ACP client.
	PeerClient Peer = "client"
	// PeerAgent authored the frame as the ACP agent.
	PeerAgent Peer = "agent"
)

// Direction is relative to the recording process, not to the protocol.
type Direction string

const (
	// DirectionIn means the recording process received the frame.
	DirectionIn Direction = "in"
	// DirectionOut means the recording process sent the frame.
	DirectionOut Direction = "out"
)

// Stream names the pipe a frame was observed on.
type Stream string

const (
	StreamStdin  Stream = "stdin"
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

// Record is one observed line. Exactly one of Frame or Text is populated:
// Frame for a line that parsed as JSON, Text for anything else (stderr output,
// or a malformed line on a JSON stream, which is recorded rather than dropped
// because a rejected frame is exactly what a customer needs to see).
type Record struct {
	Version   int             `json:"v"`
	Sequence  uint64          `json:"seq"`
	Timestamp string          `json:"t"`
	Conn      string          `json:"conn"`
	Peer      Peer            `json:"peer"`
	Direction Direction       `json:"dir"`
	Stream    Stream          `json:"stream"`
	Bytes     int             `json:"bytes"`
	Frame     json.RawMessage `json:"frame,omitempty"`
	Text      string          `json:"text,omitempty"`
	Err       string          `json:"err,omitempty"`
}
