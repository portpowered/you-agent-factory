// Package token owns the Factory token contracts that flow through runtime
// dispatch and worker-result boundaries.
package token

import workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

// DataType distinguishes resource tokens from work tokens.
type DataType = workerexecution.DataType

const (
	DataTypeResource = workerexecution.DataTypeResource
	DataTypeWork     = workerexecution.DataTypeWork
)

// Color carries the domain data attached to a token.
type Color = workerexecution.Color

// Token is a colored work item or resource flowing through the Factory net.
type Token = workerexecution.Token

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
