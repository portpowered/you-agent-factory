package cli

import (
	"bufio"
	"encoding/json"
	"mime"
	"net/http"
	"strings"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const maxStreamFrameBytes = 128 * 1024

func (a *Adapter) follow(config Config, response *http.Response, request *http.Request) error {
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "text/event-stream" {
		return agentmessages.ErrStreamUnavailable
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), maxStreamFrameBytes)
	var kind string
	var data []string
	size := 0
	for scanner.Scan() {
		if err := config.Context.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		if line == "" {
			if len(data) != 0 {
				if err := a.streamFrame(config, request, kind, strings.Join(data, "\n")); err != nil {
					return err
				}
			}
			kind, data, size = "", nil, 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			kind = value
		case "data":
			size += len(value) + 1
			if size > maxStreamFrameBytes {
				return agentmessages.ErrStreamUnavailable
			}
			data = append(data, value)
		}
	}
	if config.Context.Err() != nil {
		return config.Context.Err()
	}
	// A live tail has no successful terminal frame. EOF requires a durable
	// inbox pull before reconnection; never pretend Events retained the gap.
	return agentmessages.ErrStreamUnavailable
}

func (a *Adapter) streamFrame(config Config, request *http.Request, kind, data string) error {
	if kind == "error" {
		var response factoryapi.ErrorResponse
		if json.Unmarshal([]byte(data), &response) != nil {
			return agentmessages.ErrStreamUnavailable
		}
		return safeAPIError(http.StatusOK, response, request)
	}
	if kind != "message" {
		return nil
	}
	var observation factoryapi.AgentMessageObservation
	if json.Unmarshal([]byte(data), &observation) != nil || observation.RecordId == "" ||
		observation.Sequence < 1 || observation.Message.MessageId == "" {
		return agentmessages.ErrStreamUnavailable
	}
	if config.OutputFormat == "json" {
		return json.NewEncoder(config.Output).Encode(observation)
	}
	return a.writeMessage(config.Output, observation.Message)
}
