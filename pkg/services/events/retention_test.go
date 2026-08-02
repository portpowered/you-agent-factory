package events

import (
	"errors"
	"testing"
)

func TestRetentionLimitsValidate(t *testing.T) {
	tests := []struct {
		name    string
		limits  RetentionLimits
		wantErr error
	}{
		{
			name:    "positive limits are valid",
			limits:  RetentionLimits{MaxRecords: 100, MaxBytes: 1024},
			wantErr: nil,
		},
		{
			name:    "zero max records",
			limits:  RetentionLimits{MaxRecords: 0, MaxBytes: 1024},
			wantErr: ErrInvalidMaxRecords,
		},
		{
			name:    "negative max records",
			limits:  RetentionLimits{MaxRecords: -1, MaxBytes: 1024},
			wantErr: ErrInvalidMaxRecords,
		},
		{
			name:    "zero max bytes",
			limits:  RetentionLimits{MaxRecords: 100, MaxBytes: 0},
			wantErr: ErrInvalidMaxBytes,
		},
		{
			name:    "negative max bytes",
			limits:  RetentionLimits{MaxRecords: 100, MaxBytes: -1},
			wantErr: ErrInvalidMaxBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no validation error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected errors.Is(err, %v), got %v", tt.wantErr, err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a *ValidationError, got %T", err)
			}
		})
	}
}

func TestRetainedRangeCarriesTopicScopedBounds(t *testing.T) {
	r := RetainedRange{Topic: "factory-session/s1/response-events", Earliest: 5, Head: 12}
	if r.Topic == "" {
		t.Fatalf("expected a non-empty Topic")
	}
	if r.Earliest > r.Head {
		t.Fatalf("expected Earliest <= Head, got Earliest=%d Head=%d", r.Earliest, r.Head)
	}
}

func validGap() Gap {
	return Gap{Topic: "factory-session/s1/response-events", From: 1, To: 3, ResumeAt: 4}
}

func TestGapValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Gap) Gap
		wantErr error
	}{
		{
			name:    "valid single-position gap",
			mutate:  func(g Gap) Gap { return g },
			wantErr: nil,
		},
		{
			name:    "empty topic",
			mutate:  func(g Gap) Gap { g.Topic = ""; return g },
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "zero from",
			mutate:  func(g Gap) Gap { g.From = 0; return g },
			wantErr: ErrInvalidGapRange,
		},
		{
			name:    "negative from",
			mutate:  func(g Gap) Gap { g.From = -1; return g },
			wantErr: ErrInvalidGapRange,
		},
		{
			name:    "to before from",
			mutate:  func(g Gap) Gap { g.To = g.From - 1; return g },
			wantErr: ErrInvalidGapRange,
		},
		{
			name:    "resume at within the missing range",
			mutate:  func(g Gap) Gap { g.ResumeAt = g.To; return g },
			wantErr: ErrInvalidGapRange,
		},
		{
			name:    "resume at before the missing range",
			mutate:  func(g Gap) Gap { g.ResumeAt = g.From; return g },
			wantErr: ErrInvalidGapRange,
		},
		{
			name: "single missing position is valid",
			mutate: func(g Gap) Gap {
				g.From = 5
				g.To = 5
				g.ResumeAt = 6
				return g
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validGap()).Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no validation error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected errors.Is(err, %v), got %v", tt.wantErr, err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a *ValidationError, got %T", err)
			}
		})
	}
}
