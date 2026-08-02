package fixtures

import (
	"strings"
	"testing"
)

func TestParseValidCorpus(t *testing.T) {
	data := []byte(`{"cases":[
		{"name":"a","role":"initialize","direction":"inbound","classification":"accepted","input":{"protocolVersion":1},"expected":{"protocolVersion":1}},
		{"name":"b","role":"stop_reason","direction":"outbound","classification":"accepted","input":{"outcome":"completed"},"expected":{"stopReason":"end_turn"}}
	]}`)
	corpus, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() unexpected error = %v", err)
	}
	if len(corpus.Cases) != 2 {
		t.Fatalf("len(corpus.Cases) = %d, want 2", len(corpus.Cases))
	}
	if corpus.Cases[0].Name != "a" || corpus.Cases[1].Name != "b" {
		t.Fatalf("unexpected case names: %+v", corpus.Cases)
	}
}

func TestParseInvalidShape(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name:    "not json",
			data:    `not json`,
			wantErr: "decode corpus",
		},
		{
			name:    "empty corpus",
			data:    `{"cases":[]}`,
			wantErr: "no cases",
		},
		{
			name:    "missing name",
			data:    `{"cases":[{"role":"initialize","direction":"inbound","classification":"accepted","input":{},"expected":{}}]}`,
			wantErr: "name is required",
		},
		{
			name:    "duplicate name",
			data:    `{"cases":[{"name":"a","role":"initialize","direction":"inbound","classification":"accepted","input":{},"expected":{}},{"name":"a","role":"initialize","direction":"inbound","classification":"accepted","input":{},"expected":{}}]}`,
			wantErr: "duplicate case name",
		},
		{
			name:    "unknown role",
			data:    `{"cases":[{"name":"a","role":"session/frobnicate","direction":"inbound","classification":"accepted","input":{},"expected":{}}]}`,
			wantErr: "unknown role",
		},
		{
			name:    "unknown direction",
			data:    `{"cases":[{"name":"a","role":"initialize","direction":"sideways","classification":"accepted","input":{},"expected":{}}]}`,
			wantErr: "unknown direction",
		},
		{
			name:    "unknown classification",
			data:    `{"cases":[{"name":"a","role":"initialize","direction":"inbound","classification":"maybe","input":{},"expected":{}}]}`,
			wantErr: "unknown classification",
		},
		{
			name:    "missing input",
			data:    `{"cases":[{"name":"a","role":"initialize","direction":"inbound","classification":"accepted","expected":{}}]}`,
			wantErr: "input must be non-empty valid JSON",
		},
		{
			name:    "missing expected",
			data:    `{"cases":[{"name":"a","role":"initialize","direction":"inbound","classification":"accepted","input":{}}]}`,
			wantErr: "expected must be non-empty valid JSON",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data))
			if err == nil {
				t.Fatalf("Parse() error = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
