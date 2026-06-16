package modelhost

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPHealthChecker probes readiness through HTTP GET on a health endpoint.
type HTTPHealthChecker struct {
	Client *http.Client
	Path   string
}

func (h HTTPHealthChecker) Check(ctx context.Context, healthEndpoint string) error {
	url := healthEndpointURL(healthEndpoint, h.Path)
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func healthEndpointURL(endpoint string, path string) string {
	trimmed := strings.TrimSpace(endpoint)
	if strings.Contains(trimmed, "://") && (strings.HasSuffix(trimmed, "/health") || strings.Contains(trimmed, "/health?")) {
		return trimmed
	}
	base := strings.TrimRight(trimmed, "/")
	healthPath := strings.TrimSpace(path)
	if healthPath == "" {
		healthPath = defaultHealthCheckPath
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	return base + healthPath
}
