package requests

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

func newTraceID() string {
	return newRandomID("trace")
}

func newRequestID() string {
	return newRandomID("request")
}

// NewRequestID returns a unique request identifier for programmatic work submission.
func NewRequestID() string {
	return newRequestID()
}

func newRandomID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), traceFallbackSeq.Add(1))
	}
	return prefix + "-" + hex.EncodeToString(raw[:])
}

var traceFallbackSeq atomic.Uint64
