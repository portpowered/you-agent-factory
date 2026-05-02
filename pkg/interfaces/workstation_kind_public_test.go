package interfaces

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestGeneratedPublicWorkstationKind(t *testing.T) {
	tests := []struct {
		name  string
		input WorkstationKind
		want  factoryapi.WorkstationKind
	}{
		{
			name:  "standard runtime kind",
			input: WorkstationKindStandard,
			want:  factoryapi.WorkstationKindStandard,
		},
		{
			name:  "repeater public kind",
			input: WorkstationKind(publicFactoryWorkstationKindRepeater),
			want:  factoryapi.WorkstationKindRepeater,
		},
		{
			name:  "cron trimmed public kind",
			input: WorkstationKind("  CRON  "),
			want:  factoryapi.WorkstationKindCron,
		},
		{
			name:  "trimmed unknown kind",
			input: WorkstationKind("  custom-kind  "),
			want:  factoryapi.WorkstationKind("custom-kind"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GeneratedPublicWorkstationKind(tt.input); got != tt.want {
				t.Fatalf("GeneratedPublicWorkstationKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratedPublicWorkstationKindPtr(t *testing.T) {
	if got := GeneratedPublicWorkstationKindPtr(WorkstationKind("   ")); got != nil {
		t.Fatalf("GeneratedPublicWorkstationKindPtr returned %#v, want nil for whitespace-only input", got)
	}

	tests := []struct {
		name  string
		input WorkstationKind
		want  factoryapi.WorkstationKind
	}{
		{
			name:  "supported kind",
			input: WorkstationKind("  REPEATER  "),
			want:  factoryapi.WorkstationKindRepeater,
		},
		{
			name:  "unknown trimmed kind",
			input: WorkstationKind("  custom-kind  "),
			want:  factoryapi.WorkstationKind("custom-kind"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GeneratedPublicWorkstationKindPtr(tt.input)
			if got == nil {
				t.Fatal("GeneratedPublicWorkstationKindPtr returned nil for non-empty input")
			}
			if *got != tt.want {
				t.Fatalf("GeneratedPublicWorkstationKindPtr(%q) returned %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}
