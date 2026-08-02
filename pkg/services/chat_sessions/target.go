package chatsessions

import "strings"

// ChatTargetKind identifies what kind of runtime a Chat Session episode is
// bound to. WORKER is representable and validated in this packet but exposes
// no Worker Session operation or behavior; direct Worker targets remain a
// later lane.
type ChatTargetKind string

const (
	// ChatTargetKindFactory selects a Factory as the Chat Session target.
	ChatTargetKindFactory ChatTargetKind = "FACTORY"
	// ChatTargetKindWorker selects a Worker as the Chat Session target. It is
	// declared for representability only; no Worker Session behavior exists
	// in this packet.
	ChatTargetKindWorker ChatTargetKind = "WORKER"
)

// Validate reports whether k is one of the exactly declared ChatTargetKind
// values. The zero value and any unknown value are rejected.
func (k ChatTargetKind) Validate() error {
	switch k {
	case ChatTargetKindFactory, ChatTargetKindWorker:
		return nil
	default:
		return &InvalidChatTargetKindError{Kind: k}
	}
}

// ChatTargetRef is the canonical, unversioned identity of a Chat Session
// target. It carries only Kind and Ref: no version, digest, mutable
// definition snapshot, transport type, or persistence locator.
type ChatTargetRef struct {
	Kind ChatTargetKind
	Ref  string
}

// Validate reports whether ref has a valid Kind and a non-empty canonical
// Ref.
func (ref ChatTargetRef) Validate() error {
	if err := ref.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(ref.Ref) == "" {
		return &InvalidChatTargetRefError{Kind: ref.Kind}
	}
	return nil
}
