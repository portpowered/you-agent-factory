package browser

import (
	"reflect"
	"testing"
)

func TestCommandSelectsNativeLauncher(t *testing.T) {
	t.Parallel()

	const url = "http://127.0.0.1:7437/dashboard"
	for _, tc := range []struct {
		goos string
		name string
		args []string
	}{
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", url}},
		{goos: "darwin", name: "open", args: []string{url}},
		{goos: "linux", name: "xdg-open", args: []string{url}},
		{goos: "freebsd", name: "xdg-open", args: []string{url}},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			name, args := Command(tc.goos, url)
			if name != tc.name || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("Command(%q) = %q %#v, want %q %#v", tc.goos, name, args, tc.name, tc.args)
			}
		})
	}
}
