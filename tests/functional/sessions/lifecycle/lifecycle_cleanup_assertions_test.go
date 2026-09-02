package lifecycle_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const lifecycleDurableSessionPrefix = "dur-sess-"

func isLifecycleDurableSession(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), lifecycleDurableSessionPrefix)
}

func removeLifecycleSessionPath(t testing.TB, folderPath string) bool {
	t.Helper()
	if strings.TrimSpace(folderPath) == "" {
		return true
	}
	if err := os.RemoveAll(folderPath); err != nil {
		t.Errorf("remove temporary Factory path %q: %v", folderPath, err)
		return false
	}
	if _, err := os.Stat(folderPath); err == nil {
		t.Errorf("temporary Factory path %q remains after remove", folderPath)
		return false
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("probe temporary Factory path %q: %v", folderPath, err)
		return false
	}
	return true
}

func assertDurableSessionTerminal(t testing.TB, baseURL, sessionID string) bool {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := &http.Client{Timeout: lifecycleCleanupProbeTimeout}
	defer client.CloseIdleConnections()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Errorf("read durable Factory Session %q: %v", sessionID, err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Errorf("read durable Factory Session %q status=%d body=%q", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
		return false
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Errorf("decode durable Factory Session %q: %v", sessionID, err)
		return false
	}
	durable, err := envelope.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Errorf("decode durable Factory Session read model %q: %v", sessionID, err)
		return false
	}
	switch durable.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
		return true
	default:
		t.Errorf("durable Factory Session %q status=%q is not terminal", sessionID, durable.Status)
		return false
	}
}
