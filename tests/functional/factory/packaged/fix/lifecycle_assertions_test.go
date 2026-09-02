package fix

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func assertPackagedFixSessionDeleted(t testing.TB, baseURL, sessionID string) bool {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Errorf("GET deleted Fix Factory Session %q: %v", sessionID, err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	body, _ := io.ReadAll(response.Body)
	t.Errorf("GET deleted Fix Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	return false
}

func assertPackagedFixPortClosed(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse Fix fixture URL: %w", err)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return fmt.Errorf("Fix fixture port %q still accepts connections", parsed.Host)
	}
	return nil
}
