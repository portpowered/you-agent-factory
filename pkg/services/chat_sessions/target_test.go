package chatsessions

import (
	"errors"
	"testing"
)

func TestChatTargetKindValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    ChatTargetKind
		wantErr bool
	}{
		{name: "factory is valid", kind: ChatTargetKindFactory, wantErr: false},
		{name: "worker is valid", kind: ChatTargetKindWorker, wantErr: false},
		{name: "zero value is invalid", kind: ChatTargetKind(""), wantErr: true},
		{name: "unknown value is invalid", kind: ChatTargetKind("HUMAN"), wantErr: true},
		{name: "lowercase known value is invalid", kind: ChatTargetKind("factory"), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.kind.Validate()
			if !test.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			var invalid *InvalidChatTargetKindError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidChatTargetKindError", err, err)
			}
			if invalid.Kind != test.kind {
				t.Fatalf("error Kind = %q, want %q", invalid.Kind, test.kind)
			}
		})
	}
}

func TestChatTargetRefValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ref     ChatTargetRef
		wantErr bool
	}{
		{
			name: "valid factory ref",
			ref:  ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-builder"},
		},
		{
			name: "valid worker ref is representable",
			ref:  ChatTargetRef{Kind: ChatTargetKindWorker, Ref: "worker-123"},
		},
		{
			name:    "invalid kind",
			ref:     ChatTargetRef{Kind: "UNKNOWN", Ref: "@you/factory-builder"},
			wantErr: true,
		},
		{
			name:    "empty ref",
			ref:     ChatTargetRef{Kind: ChatTargetKindFactory, Ref: ""},
			wantErr: true,
		},
		{
			name:    "whitespace only ref",
			ref:     ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "   "},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.ref.Validate()
			if test.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
