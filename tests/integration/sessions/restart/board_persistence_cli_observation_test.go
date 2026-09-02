package restart_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const boardPersistenceMaxEventLineBytes = 4 * 1024 * 1024

func readBoardEvents(ctx context.Context, baseURL string) ([]factoryapi.FactoryEvent, error) {
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/events"
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET Factory Events status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	retainedCount, err := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)))
	if err != nil {
		return nil, fmt.Errorf("parse retained Factory Event count: %w", err)
	}
	events := make([]factoryapi.FactoryEvent, 0, retainedCount)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), boardPersistenceMaxEventLineBytes)
	for len(events) < retainedCount && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, fmt.Errorf("decode Factory Event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("read Factory Events: %w", err)
	}
	if len(events) != retainedCount {
		return nil, fmt.Errorf("Factory Event stream ended after %d of %d retained events", len(events), retainedCount)
	}
	return events, nil
}

func waitForBoardWorkerObservation(
	t *testing.T,
	baseURL, sessionID, workID string,
	predicate func(factoryapi.WorkerSessionObservation) bool,
	timeout time.Duration,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last []factoryapi.WorkerSessionObservation
	for {
		listed, err := readBoardWorkerSessions(t.Context(), baseURL, sessionID, workID)
		if err == nil {
			last = listed.Sessions
		}
		for _, observation := range listed.Sessions {
			if predicate(observation) {
				return observation
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			states, _ := readBoardDispatchStates(t.Context(), baseURL)
			t.Fatalf("timed out waiting for Worker Session observation for Work %q; last=%#v, dispatches=%#v", workID, last, states)
		}
	}
}

func waitForBoardSessionID(t *testing.T, baseURL string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListFactorySessionsResponse
	var lastErr error
	for {
		listed, err := readBoardSessions(t.Context(), baseURL)
		if err == nil {
			last = listed
			if len(listed.Sessions) > 0 {
				for _, session := range listed.Sessions {
					if session.IsDefault {
						return session.Id
					}
				}
				return listed.Sessions[0].Id
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for live Factory Session; last=%#v, error=%v", last, lastErr)
		}
	}
}

func readBoardSessions(ctx context.Context, baseURL string) (factoryapi.ListFactorySessionsResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/factory-sessions?scope=live", nil)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("GET factory sessions status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return listed, nil
}

func readBoardWorkerSessions(ctx context.Context, baseURL, sessionID, workID string) (factoryapi.ListWorkerSessionsResponse, error) {
	_ = sessionID
	endpoint := strings.TrimSuffix(baseURL, "/") + "/worker-sessions?scope=all&maxResults=100"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("GET Worker Sessions status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListWorkerSessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	filtered := factoryapi.ListWorkerSessionsResponse{Sessions: make([]factoryapi.WorkerSessionObservation, 0, len(listed.Sessions))}
	for _, observation := range listed.Sessions {
		for _, candidateWorkID := range observation.WorkIds {
			if candidateWorkID == workID {
				filtered.Sessions = append(filtered.Sessions, observation)
				break
			}
		}
	}
	return filtered, nil
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}
