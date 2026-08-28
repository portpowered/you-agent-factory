package restart_test

import (
	"encoding/json"
	"strconv"
	"testing"
)

const (
	boardPersistenceRequestID        = "board-persistence-round-trip-request"
	boardPersistenceNewRequestID     = "board-persistence-after-recovery-request"
	boardPersistenceHelperEnv        = "YOU_BOARD_PERSISTENCE_HELPER"
	boardPersistenceReleaseEnv       = "YOU_BOARD_PERSISTENCE_RELEASE"
	boardPersistenceHelperEnvValue   = "1"
	boardPersistenceWorkerSentinel   = "board-persistence-worker-result:PASS"
	boardPersistenceInitialWorkID    = "board-persistence-init"
	boardPersistenceProcessingWorkID = "board-persistence-processing"
	boardPersistenceAwaitingWorkID   = "board-persistence-awaiting-ci"
	boardPersistenceNewWorkID        = "board-persistence-new-work"
)

type boardPersistenceExpectedWork struct {
	Name           string
	WorkID         string
	RequestID      string
	State          string
	StateType      string
	TraceID        string
	CurrentTraceID string
	Content        string
	RelationTarget string
	WorkerOutput   bool
}

type boardPersistenceBatchWork struct {
	Name    string
	WorkID  string
	State   string
	TraceID string
	Content string
}

func boardPersistenceExpectedWorks() map[string]boardPersistenceExpectedWork {
	return map[string]boardPersistenceExpectedWork{
		boardPersistenceInitialWorkID: {
			Name: "board-init", WorkID: boardPersistenceInitialWorkID, RequestID: boardPersistenceRequestID,
			State: "init", StateType: "INITIAL", TraceID: "trace-board-init", CurrentTraceID: "trace-board-init", Content: "durable init content",
		},
		boardPersistenceProcessingWorkID: {
			Name: "board-processing", WorkID: boardPersistenceProcessingWorkID, RequestID: boardPersistenceRequestID,
			State: "processing", StateType: "PROCESSING", TraceID: "trace-board-processing", CurrentTraceID: "trace-board-processing", Content: "durable processing content",
		},
		boardPersistenceAwaitingWorkID: {
			Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, RequestID: boardPersistenceRequestID,
			State: "awaiting-ci", StateType: "PROCESSING", TraceID: "trace-board-awaiting-ci", CurrentTraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content",
			RelationTarget: boardPersistenceProcessingWorkID,
		},
	}
}

func boardPersistenceFactoryConfig() map[string]any {
	return map[string]any{
		"name": "board-persistence-restart",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "awaiting-ci", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "restart-blocker"}},
		"workstations": []map[string]any{{
			"name":      "hold-processing",
			"worker":    "restart-blocker",
			"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func boardPersistenceWorkerConfig(testBinary string) string {
	return "---\n" +
		"type: SCRIPT_WORKER\n" +
		"command: " + strconv.Quote(testBinary) + "\n" +
		"args:\n" +
		"  - " + strconv.Quote("-test.run=^TestBoardPersistenceWorkerHelper$") + "\n" +
		"---\n" +
		"Hold the Work attempt until the restart test releases it.\n"
}

func boardPersistenceBatchJSON(t *testing.T, requestID string, works []boardPersistenceBatchWork) string {
	t.Helper()
	entries := make([]map[string]any, 0, len(works))
	for _, work := range works {
		entries = append(entries, map[string]any{
			"name":                     work.Name,
			"workId":                   work.WorkID,
			"workTypeName":             "task",
			"state":                    work.State,
			"traceId":                  work.TraceID,
			"currentChainingTraceId":   work.TraceID,
			"previousChainingTraceIds": []string{},
			"content":                  []map[string]any{{"type": "text", "text": work.Content}},
		})
	}
	request := map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works":     entries,
		"relations": []map[string]string{{
			"type":           "PARENT_CHILD",
			"sourceWorkName": "board-awaiting-ci",
			"targetWorkName": "board-processing",
		}},
	}
	if requestID == boardPersistenceNewRequestID {
		request["relations"] = []map[string]string{}
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal board persistence batch: %v", err)
	}
	return string(raw)
}
