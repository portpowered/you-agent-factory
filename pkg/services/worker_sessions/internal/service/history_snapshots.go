package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

const (
	historySnapshotIdle  = 5 * time.Minute
	historySnapshotCount = 64
	historySnapshotBytes = 16 << 20
	// Include row slice headers and conservative fixed cache/cursor bookkeeping
	// in addition to serialized identity and observation bytes.
	historySnapshotOverhead    = 1024
	historySnapshotRowOverhead = 24
	historyCursorMaxBytes      = 4096
)

// HistorySnapshotBudget bounds retained history pages across every query view
// in one constructed profile. Composition supplies the same budget to its
// registry and fleet views; separate profiles never share one.
type HistorySnapshotBudget struct {
	// Entropy supplies unpredictable cursor identities and signing keys.
	// Composition selects the implementation; reads are serialized by mu.
	Entropy io.Reader
	mu      sync.Mutex
	entries map[string]*observationSnapshot
	bytes   int
	access  uint64
}

// A view shares storage limits without accepting another view's cursors.
type observationSnapshots struct {
	*HistorySnapshotBudget
	owner *byte
}

func newObservationSnapshots(budget *HistorySnapshotBudget) observationSnapshots {
	return observationSnapshots{HistorySnapshotBudget: budget, owner: new(byte)}
}

type observationSnapshot struct {
	owner   *byte
	filter  string
	rows    [][]byte
	secret  [16]byte
	bytes   int
	touched time.Time
	access  uint64
}

type historyCursor struct {
	ID        string `json:"id"`
	Offset    int    `json:"offset"`
	Signature string `json:"signature"`
}

// Observation's internal ordering facts are excluded from public JSON. Retain
// them separately so a frozen page preserves the entire service observation.
type frozenObservation struct {
	Observation   workersessions.Observation
	Generation    string
	Sequence      int64
	SequenceKnown bool
}

func freezeHistoryRows(observations []workersessions.Observation, filter string) ([][]byte, int, error) {
	rows := make([][]byte, 0, len(observations))
	size := historySnapshotOverhead + len(filter)
	for _, observation := range observations {
		row, err := json.Marshal(frozenObservation{observation, observation.StreamGenerationID, observation.StateSequence, observation.StateSequenceKnown})
		if err != nil {
			return nil, 0, workersessions.ErrObservationProjectionUnavailable
		}
		size += len(row) + historySnapshotRowOverhead
		if size > historySnapshotBytes {
			return nil, 0, workersessions.ErrObservationProjectionUnavailable
		}
		rows = append(rows, row)
	}
	return rows, size, nil
}

func (s *observationSnapshots) first(observations []workersessions.Observation, filter string, limit int, now time.Time) (workersessions.ListWorkerSessionObservationsResult, error) {
	rows, size, err := freezeHistoryRows(observations, filter)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	entry := &observationSnapshot{owner: s.owner, filter: filter, rows: rows, bytes: size, touched: now}
	result, err := entry.page(0, limit)
	if err != nil || len(rows) <= limit {
		return result, err
	}
	var entropy [32]byte
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Entropy == nil {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	if _, err := io.ReadFull(s.Entropy, entropy[:]); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	id := hex.EncodeToString(entropy[:16])
	copy(entry.secret[:], entropy[16:])
	s.prune(now)
	for len(s.entries) >= historySnapshotCount || s.bytes+size > historySnapshotBytes {
		s.evictOldest()
	}
	if s.entries == nil {
		s.entries = make(map[string]*observationSnapshot)
	}
	s.access++
	entry.access = s.access
	s.entries[id] = entry
	s.bytes += size
	result.NextToken = entry.token(id, limit)
	return result, nil
}

func (s *observationSnapshots) next(token, filter string, limit int, now time.Time) (workersessions.ListWorkerSessionObservationsResult, error) {
	cursor, err := decodeHistoryCursor(token)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	entry := s.entries[cursor.ID]
	if entry == nil || entry.owner != s.owner || entry.filter != filter || cursor.Offset < 1 || cursor.Offset >= len(entry.rows) || !entry.validCursor(cursor) {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrInvalidObservationPagination
	}
	result, err := entry.page(cursor.Offset, limit)
	if err != nil {
		return result, err
	}
	s.access++
	entry.access, entry.touched = s.access, now
	end := cursor.Offset + len(result.Observations)
	if end < len(entry.rows) {
		result.NextToken = entry.token(cursor.ID, end)
	}
	return result, nil
}

func (s *observationSnapshots) prune(now time.Time) {
	for id, entry := range s.entries {
		if now.Sub(entry.touched) >= historySnapshotIdle {
			s.bytes -= entry.bytes
			delete(s.entries, id)
		}
	}
}

func (s *observationSnapshots) evictOldest() {
	var oldest string
	var access uint64
	for id, entry := range s.entries {
		if oldest == "" || entry.access < access {
			oldest, access = id, entry.access
		}
	}
	s.bytes -= s.entries[oldest].bytes
	delete(s.entries, oldest)
}

func (s *observationSnapshot) page(offset, limit int) (workersessions.ListWorkerSessionObservationsResult, error) {
	result := workersessions.ListWorkerSessionObservationsResult{MaxResults: limit, Observations: make([]workersessions.Observation, 0, min(limit, len(s.rows)-offset))}
	// Subtract before adding so an arbitrarily large valid limit cannot overflow.
	end := offset + min(limit, len(s.rows)-offset)
	for _, row := range s.rows[offset:end] {
		var frozen frozenObservation
		if err := json.Unmarshal(row, &frozen); err != nil {
			return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
		}
		observation := frozen.Observation
		observation.StreamGenerationID, observation.StateSequence, observation.StateSequenceKnown = frozen.Generation, frozen.Sequence, frozen.SequenceKnown
		result.Observations = append(result.Observations, observation)
	}
	return result, nil
}

func (s *observationSnapshot) signature(id string, offset int) []byte {
	mac := hmac.New(sha256.New, s.secret[:])
	_, _ = mac.Write([]byte(id + ":" + strconv.Itoa(offset)))
	return mac.Sum(nil)
}

func (s *observationSnapshot) validCursor(cursor historyCursor) bool {
	signature, err := hex.DecodeString(cursor.Signature)
	return err == nil && hmac.Equal(signature, s.signature(cursor.ID, cursor.Offset))
}

func (s *observationSnapshot) token(id string, offset int) string {
	data, _ := json.Marshal(historyCursor{ID: id, Offset: offset, Signature: hex.EncodeToString(s.signature(id, offset))})
	return base64.StdEncoding.EncodeToString(data)
}

func decodeHistoryCursor(token string) (historyCursor, error) {
	var cursor historyCursor
	if len(token) > historyCursorMaxBytes {
		return cursor, workersessions.ErrInvalidObservationPagination
	}
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return cursor, workersessions.ErrInvalidObservationPagination
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.ID == "" {
		return cursor, workersessions.ErrInvalidObservationPagination
	}
	return cursor, nil
}

// IsHistoryCursor distinguishes a signed snapshot envelope from the legacy
// last-identity cursor so every fleet entry point rejects mixed pagination.
func IsHistoryCursor(token string) bool {
	cursor, err := decodeHistoryCursor(token)
	return err == nil && cursor.Signature != ""
}
