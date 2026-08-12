package service

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

type deadLetterRecord struct {
	SchemaVersion     string                                 `json:"schemaVersion"`
	EndpointName      string                                 `json:"endpointName"`
	DestinationOrigin string                                 `json:"destinationOrigin"`
	DestinationPath   string                                 `json:"destinationPath"`
	EventID           string                                 `json:"eventId"`
	EventType         string                                 `json:"eventType"`
	EventContext      factorydefinitions.FactoryEventContext `json:"eventContext"`
	AttemptCount      int                                    `json:"attemptCount"`
	FirstAttemptAt    time.Time                              `json:"firstAttemptAt"`
	LastAttemptAt     time.Time                              `json:"lastAttemptAt"`
	TerminalAt        time.Time                              `json:"terminalAt"`
	TerminalReason    string                                 `json:"terminalReason"`
	StatusCode        int                                    `json:"statusCode,omitempty"`
	CanonicalBody     json.RawMessage                        `json:"canonicalBody"`
}

func (service *Service) appendDeadLetter(
	request webhooks.StartRequest,
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	body []byte,
	attemptCount int,
	firstAttemptAt time.Time,
	lastAttemptAt time.Time,
	statusCode int,
	terminalReason string,
) {
	if service.deadLetters == nil || strings.TrimSpace(request.DeadLetterPath) == "" {
		service.logger.Error(
			"factory webhook dead-letter storage unavailable",
			"endpoint", definition.Name,
			"event_id", string(event.ID),
			"terminal_reason", terminalReason,
		)
		return
	}
	var envelope factorydefinitions.FactoryEvent
	if err := json.Unmarshal(body, &envelope); err != nil {
		service.logger.Error(
			"factory webhook dead-letter envelope unavailable",
			"endpoint", definition.Name,
			"event_id", string(event.ID),
			"terminal_reason", terminalReason,
		)
		return
	}
	origin, destinationPath := safeDestination(definition.URL)
	record := deadLetterRecord{
		SchemaVersion:     "factory.webhook-dead-letter.v1",
		EndpointName:      definition.Name,
		DestinationOrigin: origin,
		DestinationPath:   destinationPath,
		EventID:           string(event.ID),
		EventType:         string(event.Kind),
		EventContext:      envelope.Context,
		AttemptCount:      attemptCount,
		FirstAttemptAt:    firstAttemptAt.UTC(),
		LastAttemptAt:     lastAttemptAt.UTC(),
		TerminalAt:        service.clock.Now().UTC(),
		TerminalReason:    terminalReason,
		StatusCode:        statusCode,
		CanonicalBody:     append(json.RawMessage(nil), body...),
	}
	line, err := json.Marshal(record)
	if err != nil {
		service.logger.Error(
			"factory webhook dead-letter encoding failed",
			"endpoint", definition.Name,
			"event_id", string(event.ID),
			"terminal_reason", terminalReason,
		)
		return
	}
	line = append(line, '\n')
	service.deadLetterMu.Lock()
	err = service.deadLetters(request.DeadLetterPath, line)
	service.deadLetterMu.Unlock()
	if err != nil {
		service.logger.Error(
			"factory webhook dead-letter append failed",
			"endpoint", definition.Name,
			"event_id", string(event.ID),
			"terminal_reason", terminalReason,
			"path", request.DeadLetterPath,
		)
	}
}

func safeDestination(raw string) (string, string) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return "", ""
	}
	origin := ""
	if parsed.Scheme != "" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	destinationPath := parsed.EscapedPath()
	if destinationPath == "" {
		destinationPath = "/"
	}
	return origin, path.Clean("/" + strings.TrimPrefix(destinationPath, "/"))
}
