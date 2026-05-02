package petri_test

import (
	"fmt"
	"regexp"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestWorkIDGenerator_Next(t *testing.T) {
	gen := petri.NewWorkIDGenerator()
	pattern := regexp.MustCompile(`^work-.+-\d+$`)

	const n = 10
	seen := make(map[string]bool, n)

	for i := 0; i < n; i++ {
		id := gen.Next("mytype")
		if !pattern.MatchString(id) {
			t.Errorf("ID %q does not match work-{type}-{N} pattern", id)
		}
		if seen[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		seen[id] = true
	}

	if len(seen) != n {
		t.Errorf("expected %d unique IDs, got %d", n, len(seen))
	}
}

func TestWorkIDGenerator_MultipleTypes(t *testing.T) {
	gen := petri.NewWorkIDGenerator()

	id1 := gen.Next("alpha")
	id2 := gen.Next("beta")
	id3 := gen.Next("alpha")

	// Counter is shared across types, so numbers increase globally.
	expected := []string{"work-alpha-1", "work-beta-2", "work-alpha-3"}
	actual := []string{id1, id2, id3}

	for i, exp := range expected {
		if actual[i] != exp {
			t.Errorf("ID[%d]: got %q, want %q", i, actual[i], exp)
		}
	}
}

func TestWorkIDGenerator_ConcurrentSafety(t *testing.T) {
	gen := petri.NewWorkIDGenerator()
	const goroutines = 10
	const idsPerGoroutine = 100

	var mu sync.Mutex
	seen := make(map[string]bool, goroutines*idsPerGoroutine)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			typeID := fmt.Sprintf("type-%d", g)
			for i := 0; i < idsPerGoroutine; i++ {
				id := gen.Next(typeID)
				mu.Lock()
				if seen[id] {
					t.Errorf("duplicate ID under concurrency: %s", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}(g)
	}

	wg.Wait()
	if len(seen) != goroutines*idsPerGoroutine {
		t.Errorf("expected %d unique IDs, got %d", goroutines*idsPerGoroutine, len(seen))
	}
}
