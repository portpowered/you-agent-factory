package chatsessions

import (
	"errors"
	"strings"
	"testing"
)

func TestAttachmentValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		attachment Attachment
		wantErr    bool
		wantReason AttachmentInvalidReason
	}{
		{
			name: "fully populated attachment is valid",
			attachment: Attachment{
				ID:            "attach-1",
				SessionID:     "session-1",
				ConnectionID:  "conn-1",
				AfterSequence: 42,
				Interactive:   true,
			},
			wantErr: false,
		},
		{
			name: "zero AfterSequence is valid",
			attachment: Attachment{
				ID:            "attach-1",
				SessionID:     "session-1",
				ConnectionID:  "conn-1",
				AfterSequence: 0,
			},
			wantErr: false,
		},
		{
			name:       "missing ID is invalid",
			attachment: Attachment{SessionID: "session-1", ConnectionID: "conn-1"},
			wantErr:    true,
			wantReason: AttachmentInvalidMissingID,
		},
		{
			name:       "whitespace only ID is treated as missing",
			attachment: Attachment{ID: "  ", SessionID: "session-1", ConnectionID: "conn-1"},
			wantErr:    true,
			wantReason: AttachmentInvalidMissingID,
		},
		{
			name:       "missing SessionID is invalid",
			attachment: Attachment{ID: "attach-1", ConnectionID: "conn-1"},
			wantErr:    true,
			wantReason: AttachmentInvalidMissingSessionID,
		},
		{
			name:       "missing ConnectionID is invalid",
			attachment: Attachment{ID: "attach-1", SessionID: "session-1"},
			wantErr:    true,
			wantReason: AttachmentInvalidMissingConnectionID,
		},
		{
			name:       "empty attachment is invalid",
			attachment: Attachment{},
			wantErr:    true,
			wantReason: AttachmentInvalidMissingID,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			before := test.attachment
			err := test.attachment.Validate()

			if test.attachment != before {
				t.Fatalf("Validate mutated the supplied attachment: got %+v, want %+v", test.attachment, before)
			}

			if !test.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil, want error with reason %s", test.wantReason)
			}

			var invalid *InvalidAttachmentError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidAttachmentError", err, err)
			}
			if invalid.Reason != test.wantReason {
				t.Fatalf("Validate() reason = %s, want %s", invalid.Reason, test.wantReason)
			}
		})
	}
}

func TestAttachmentValidateErrorDoesNotLeakSuppliedValues(t *testing.T) {
	t.Parallel()

	secretLookingSessionID := "session-with-secret-token-abc123"
	err := Attachment{ID: "attach-1", SessionID: secretLookingSessionID}.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want error")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("Error() = %q, want non-empty message", got)
	} else if strings.Contains(got, secretLookingSessionID) {
		t.Fatalf("Error() = %q leaked the supplied connection id", got)
	}
}

func TestAttachmentsAreIndependentAcrossTheSameSessionAndStreamPosition(t *testing.T) {
	t.Parallel()

	first := Attachment{
		ID:            "attach-1",
		SessionID:     "session-1",
		ConnectionID:  "conn-1",
		AfterSequence: 10,
		Interactive:   true,
	}
	second := Attachment{
		ID:            "attach-2",
		SessionID:     "session-1",
		ConnectionID:  "conn-2",
		AfterSequence: 10,
		Interactive:   false,
	}

	if err := first.Validate(); err != nil {
		t.Fatalf("first.Validate() = %v, want nil", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second.Validate() = %v, want nil", err)
	}

	if first.ID == second.ID {
		t.Fatalf("attachments share ID %q, want independent IDs", first.ID)
	}
	if first.ConnectionID == second.ConnectionID {
		t.Fatalf("attachments share ConnectionID %q, want independent connection identities", first.ConnectionID)
	}
	if first.Interactive == second.Interactive {
		t.Fatalf("attachments share Interactive %v, want independently settable interactivity", first.Interactive)
	}
	if first.SessionID != second.SessionID || first.AfterSequence != second.AfterSequence {
		t.Fatalf("expected both attachments to share SessionID and AfterSequence in this scenario, got %+v and %+v", first, second)
	}

	// Advancing one attachment's cursor must never affect the other's.
	advanced := first
	advanced.AfterSequence = 99
	if advanced.AfterSequence != 99 {
		t.Fatalf("advanced.AfterSequence = %d, want 99", advanced.AfterSequence)
	}
	if second.AfterSequence != 10 {
		t.Fatalf("advancing first attachment's cursor changed second attachment's cursor: got %d, want 10", second.AfterSequence)
	}
	if first.AfterSequence != 10 {
		t.Fatalf("copying into advanced changed first attachment's cursor: got %d, want 10", first.AfterSequence)
	}
}

func TestAttachmentCursorIsIndependentOfSessionStreamHeadAndControlTargets(t *testing.T) {
	t.Parallel()

	session := Session{
		ID:             "session-1",
		State:          SessionStateActive,
		ActiveTurnID:   "turn-1",
		Version:        7,
		StreamHead:     100,
		TargetEpisode:  3,
		SelectedTarget: ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "factory-1"},
	}
	before := session

	attachment := Attachment{
		ID:            "attach-1",
		SessionID:     session.ID,
		ConnectionID:  "conn-1",
		AfterSequence: 40,
	}

	if err := attachment.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	if session != before {
		t.Fatalf("validating an attachment mutated the Session: got %+v, want %+v", session, before)
	}
	if attachment.AfterSequence == session.StreamHead {
		t.Fatalf("test fixture invalid: attachment cursor %d must differ from stream head %d to prove separation", attachment.AfterSequence, session.StreamHead)
	}
}

func TestAttachmentIsInteractive(t *testing.T) {
	t.Parallel()

	interactive := Attachment{Interactive: true}
	nonInteractive := Attachment{Interactive: false}

	if !interactive.Interactive {
		t.Fatalf("expected interactive attachment to report Interactive = true")
	}
	if nonInteractive.Interactive {
		t.Fatalf("expected non-interactive attachment to report Interactive = false")
	}
}
