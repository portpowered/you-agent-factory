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
