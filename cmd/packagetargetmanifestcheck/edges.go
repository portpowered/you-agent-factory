package main

import (
	"fmt"
	"strings"
)

const edgesFutureDebtID = "FND-06"

// FutureDebt records deferred migration work that this packet intentionally
// does not perform (for example Edges narrowing under FND-06).
type FutureDebt struct {
	ID          string `json:"id"`
	PackagePath string `json:"packagePath,omitempty"`
	Description string `json:"description"`
}

func validateFutureDebt(debt []FutureDebt) error {
	found := false
	for i, entry := range debt {
		prefix := fmt.Sprintf("futureDebt[%d]", i)
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("%s.id is required", prefix)
		}
		if strings.TrimSpace(entry.Description) == "" {
			return fmt.Errorf("%s.description is required", prefix)
		}
		if entry.ID == edgesFutureDebtID && strings.Contains(strings.ToLower(entry.Description), "edges") {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("futureDebt must record %s Edges narrowing as deferred debt (not performed in this packet)", edgesFutureDebtID)
	}
	return nil
}
