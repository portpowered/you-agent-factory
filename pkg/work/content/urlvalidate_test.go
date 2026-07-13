package content

import (
	"strings"
	"testing"
)

func TestValidateContentURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		wantErr string
	}{
		{name: "file", rawURL: "file:///tmp/example.png"},
		{name: "https", rawURL: "https://example.com/a.png"},
		{name: "http", rawURL: "http://example.com/a.png"},
		{name: "data", rawURL: "data:image/png;base64,AAAA"},
		{name: "empty", rawURL: "   ", wantErr: "non-empty"},
		{name: "unsupported scheme", rawURL: "ftp://example.com/a.png", wantErr: "scheme"},
		{name: "invalid url", rawURL: "://bad", wantErr: "valid URL"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateContentURL(tc.rawURL)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateContentURL(%q) = %v, want nil", tc.rawURL, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateContentURL(%q) = %v, want error containing %q", tc.rawURL, err, tc.wantErr)
			}
		})
	}
}
