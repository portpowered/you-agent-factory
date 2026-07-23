package factorysessions

import (
	"errors"
	"fmt"
	"testing"
)

func TestSessionErrorsMatchStableBoundarySentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain error
		legacy error
	}{
		{name: "not found", domain: ErrNotFound, legacy: errors.New("factory session not found")},
		{name: "result unavailable", domain: ErrResultUnavailable, legacy: errors.New("factory session result unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(fmt.Errorf("read session: %w", test.domain), test.legacy) {
				t.Fatalf("wrapped %v did not match stable boundary sentinel %v", test.domain, test.legacy)
			}
		})
	}
}
