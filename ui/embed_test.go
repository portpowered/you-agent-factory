package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPrimaryIndexHTML_UsesYouAgentFactoryMetadata(t *testing.T) {
	body, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read primary index.html: %v", err)
	}

	shell := string(body)
	for _, want := range []string{
		"<title>You Agent Factory Dashboard</title>",
		"Standalone live dashboard shell for You Agent Factory.",
		"You%20Agent%20Factory%20dashboard%20icon",
		`<div id="root"></div>`,
	} {
		if !strings.Contains(shell, want) {
			t.Fatalf("expected primary shell to contain %q, got body: %s", want, shell)
		}
	}
	for _, retired := range []string{
		"Infinite You Dashboard",
		"Standalone live dashboard shell for Infinite You.",
		"Infinite%20You%20dashboard%20icon",
	} {
		if strings.Contains(shell, retired) {
			t.Fatalf("expected primary shell to retire %q, got body: %s", retired, shell)
		}
	}
}

func TestFallbackDistFS_ServesRenamedDashboardShell(t *testing.T) {
	distFS, err := fallbackDistFS()
	if err != nil {
		t.Fatalf("fallbackDistFS() error = %v", err)
	}

	server := httptest.NewServer(http.FileServer(http.FS(distFS)))
	defer server.Close()

	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET fallback shell: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("fallback shell status = %d, want 200", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fallback shell body: %v", err)
	}

	shell := string(body)
	for _, want := range []string{
		"<title>You Agent Factory Dashboard</title>",
		"Standalone live dashboard shell for You Agent Factory.",
		"You%20Agent%20Factory%20dashboard%20icon",
		`<div id="root"></div>`,
		"/dashboard/ui/assets/index.js",
		"/dashboard/ui/assets/index.css",
	} {
		if !strings.Contains(shell, want) {
			t.Fatalf("expected fallback shell to contain %q, got body: %s", want, shell)
		}
	}
	if strings.Contains(shell, "Infinite You Dashboard") {
		t.Fatalf("fallback shell should not contain retired dashboard title: %s", shell)
	}
}

func TestFallbackDistFS_ServesRenamedPlaceholderRuntime(t *testing.T) {
	distFS, err := fallbackDistFS()
	if err != nil {
		t.Fatalf("fallbackDistFS() error = %v", err)
	}

	server := httptest.NewServer(http.FileServer(http.FS(distFS)))
	defer server.Close()

	response, err := http.Get(server.URL + "/assets/index.js")
	if err != nil {
		t.Fatalf("GET fallback runtime asset: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("fallback runtime asset status = %d, want 200", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fallback runtime asset body: %v", err)
	}

	runtime := string(body)
	if !strings.Contains(runtime, "You Agent Factory Dashboard") {
		t.Fatalf("expected fallback runtime to contain renamed heading, got: %s", runtime)
	}
	if strings.Contains(runtime, "Infinite You Dashboard") {
		t.Fatalf("fallback runtime should not contain retired dashboard heading: %s", runtime)
	}
}
