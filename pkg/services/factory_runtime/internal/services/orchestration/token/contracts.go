// Package token owns the Factory token contracts that flow through runtime
// dispatch and worker-result boundaries.
package token

import (
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DataType distinguishes resource tokens from work tokens.
type DataType = workerexecution.DataType

const (
	DataTypeResource = workerexecution.DataTypeResource
	DataTypeWork     = workerexecution.DataTypeWork
)

// Color carries the domain data attached to a token.
type Color = workerexecution.Color

// Token is the Factory Runtime's engine-owned token. Its place is deliberately
// retained here so the Petri implementation can schedule and replay tokens
// without putting that identifier on the Workers request shape.
type Token struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	Color     Color     `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	EnteredAt time.Time `json:"entered_at"`
	History   History   `json:"history"`
}

// FromWorker copies the engine-independent token facts into a runtime token.
// State is projected back into the runtime's qualified place identifier so a
// runtime snapshot can remain self-contained after crossing the Worker port.
func FromWorker(value workerexecution.Token) Token {
	return Token{
		ID: value.ID,
		PlaceID: placeFromState(value),
		Color: value.Color, CreatedAt: value.CreatedAt,
		EnteredAt: value.EnteredAt, History: value.History,
	}
}

// ToWorker projects a runtime token into the engine-neutral Worker view.
func ToWorker(value Token) workerexecution.Token {
	value = Clone(value)
	return workerexecution.Token{
		ID: value.ID,
		State: stateFromPlace(value.PlaceID),
		Color: value.Color,
		CreatedAt: value.CreatedAt,
		EnteredAt: value.EnteredAt,
		History: value.History,
	}
}

// FromWorkerSlice projects a Worker token slice into runtime-owned tokens.
func FromWorkerSlice(values []workerexecution.Token) []Token {
	if len(values) == 0 {
		return nil
	}
	projected := make([]Token, len(values))
	for index := range values {
		projected[index] = FromWorker(values[index])
	}
	return projected
}

// ToWorkerSlice projects runtime-owned tokens into Worker-facing token facts.
func ToWorkerSlice(values []Token) []workerexecution.Token {
	if len(values) == 0 {
		return nil
	}
	projected := make([]workerexecution.Token, len(values))
	for index := range values {
		projected[index] = ToWorker(values[index])
	}
	return projected
}

func stateFromPlace(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	index := strings.LastIndexByte(trimmed, ':')
	if index < 0 {
		return trimmed
	}
	return trimmed[index+1:]
}

func placeFromState(value workerexecution.Token) string {
	state := strings.TrimSpace(value.State)
	if state == "" {
		return ""
	}
	prefix := strings.TrimSpace(value.Color.WorkTypeID)
	if prefix == "" && value.Color.DataType == workerexecution.DataTypeResource {
		prefix = strings.TrimSpace(value.Color.Name)
	}
	if prefix == "" {
		return state
	}
	return prefix + ":" + state
}

// History tracks a token's journey through the net.
type History = workerexecution.History

// Failure captures a single failed transition attempt for a token.
type Failure = workerexecution.Failure

// ClearGuardBlockingFields resets guard counters while retaining failure history.
func ClearGuardBlockingFields(history *History) {
	if history == nil {
		return
	}
	history.TotalVisits = make(map[string]int)
	history.ConsecutiveFailures = make(map[string]int)
	history.PlaceVisits = make(map[string]int)
}
