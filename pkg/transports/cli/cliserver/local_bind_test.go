package cliserver

import (
	"strings"
	"testing"
)

func TestLocalBindTargetFromServer_DefaultLocalURI(t *testing.T) {
	t.Parallel()

	target, err := LocalBindTargetFromServer("")
	if err != nil {
		t.Fatalf("LocalBindTargetFromServer: %v", err)
	}
	if target.Host != "localhost" || target.Port != 7437 {
		t.Fatalf("target = %#v, want localhost:7437", target)
	}
}

func TestLocalBindTargetFromServer_CustomLoopbackPort(t *testing.T) {
	t.Parallel()

	target, err := LocalBindTargetFromServer("http://127.0.0.1:9090")
	if err != nil {
		t.Fatalf("LocalBindTargetFromServer: %v", err)
	}
	if target.Host != "127.0.0.1" || target.Port != 9090 {
		t.Fatalf("target = %#v, want 127.0.0.1:9090", target)
	}
}

func TestLocalBindTargetFromServer_RejectsRemoteHost(t *testing.T) {
	t.Parallel()

	_, err := LocalBindTargetFromServer("https://remote.example.com:7443")
	if err == nil {
		t.Fatal("expected error for remote host")
	}
	if !strings.Contains(err.Error(), "not a local bind target") {
		t.Fatalf("error = %v, want local bind guidance", err)
	}
}

func TestLocalBindTargetFromServer_DefaultHTTPPort(t *testing.T) {
	t.Parallel()

	target, err := LocalBindTargetFromServer("http://localhost")
	if err != nil {
		t.Fatalf("LocalBindTargetFromServer: %v", err)
	}
	if target.Port != 80 {
		t.Fatalf("port = %d, want default http port 80", target.Port)
	}
}

func TestLocalBindTargetFromListen_ExactLoopbackAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		host  string
		port  int
	}{
		{name: "localhost", input: "localhost:9090", host: "localhost", port: 9090},
		{name: "ipv4", input: "127.0.0.1:9091", host: "127.0.0.1", port: 9091},
		{name: "ipv6", input: "[::1]:9092", host: "::1", port: 9092},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			target, err := LocalBindTargetFromListen(test.input)
			if err != nil {
				t.Fatalf("LocalBindTargetFromListen(%q): %v", test.input, err)
			}
			if target.Host != test.host || target.Port != test.port {
				t.Fatalf("target = %#v, want %s:%d", target, test.host, test.port)
			}
		})
	}
}

func TestLocalBindTargetFromListen_RejectsInvalidOrRemoteAddress(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "localhost", "localhost:0", "localhost:70000", "remote.example:9090", "::1:9090"} {
		input := input
		t.Run(input, func(t *testing.T) {
			_, err := LocalBindTargetFromListen(input)
			if err == nil {
				t.Fatalf("LocalBindTargetFromListen(%q): expected error", input)
			}
			if !IsLocalBindError(err) {
				t.Fatalf("error = %T %v, want LocalBindError", err, err)
			}
		})
	}
}
