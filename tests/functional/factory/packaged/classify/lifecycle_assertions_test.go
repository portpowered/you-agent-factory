package classify

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func classifyListenerClosed(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func classifyListenerError(done <-chan struct{}) error {
	if !classifyListenerClosed(done) {
		return errors.New("classify API listener did not report shutdown")
	}
	return nil
}

func classifyPathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func assertClassifySessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Errorf("GET deleted Factory Session %q: %v", sessionID, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return
	}
	body, _ := io.ReadAll(response.Body)
	t.Errorf(
		"GET deleted Factory Session %q status = %d, want 404: %s",
		sessionID,
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func cleanupClassifyScenarioRoot(t testing.TB, rootDir string) {
	t.Helper()
	if err := os.RemoveAll(rootDir); err != nil {
		t.Errorf("remove classify scenario root %q: %v", rootDir, err)
		return
	}
	if !classifyPathAbsent(rootDir) {
		t.Errorf("classify scenario root %q remains after cleanup", rootDir)
	}
}
