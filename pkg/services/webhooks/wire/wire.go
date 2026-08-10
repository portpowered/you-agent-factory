// Package wire constructs the Webhooks root from exact application effects.
package wire

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	internalservice "github.com/portpowered/infinite-you/pkg/services/webhooks/internal/service"
)

// NewService constructs an inert Webhooks root. It starts no subscribers;
// Factory Sessions owns activation and shutdown for each runtime.
func NewService(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	},
	secretResolver webhooks.SecretResolver,
	clockSource interface{ Now() time.Time },
	deadLetterAppender webhooks.DeadLetterAppender,
	logger logging.Logger,
) webhooks.Service {
	if clockSource == nil {
		clockSource = clock.Real{}
	}
	if deadLetterAppender == nil {
		deadLetterAppender = appendDeadLetter
	}
	return internalservice.NewWithDeadLetterAppender(
		httpClient,
		secretResolver,
		clockSource,
		deadLetterAppender,
		logger,
	)
}

func appendDeadLetter(path string, line []byte) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("dead-letter path is required")
	}
	if err := os.MkdirAll(filepath.Dir(trimmedPath), 0o700); err != nil {
		return fmt.Errorf("create dead-letter directory: %w", err)
	}
	file, err := os.OpenFile(trimmedPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open dead-letter log: %w", err)
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return fmt.Errorf("append dead-letter log: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync dead-letter log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close dead-letter log: %w", err)
	}
	return nil
}
