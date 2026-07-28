package service

import (
	"net/url"
	"path/filepath"
	"testing"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

func TestLocalFilePathUsesInjectedHostPlatform(t *testing.T) {
	t.Parallel()

	parsed := &url.URL{Path: "/C:/factory/content.png"}
	tests := []struct {
		name         string
		hostPlatform work.ContentHostPlatform
		want         string
	}{
		{name: "windows drive URL", hostPlatform: "windows", want: filepath.FromSlash("C:/factory/content.png")},
		{name: "case insensitive windows", hostPlatform: "WINDOWS", want: filepath.FromSlash("C:/factory/content.png")},
		{name: "non-windows absolute path", hostPlatform: "linux", want: filepath.FromSlash("/C:/factory/content.png")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := localFilePath(parsed, test.hostPlatform)
			if err != nil {
				t.Fatalf("localFilePath: %v", err)
			}
			if got != test.want {
				t.Fatalf("localFilePath = %q, want %q", got, test.want)
			}
		})
	}
}
