package main

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	mutationExcerptMaxBytes = 256
	mutationExcerptContext  = 32
)

type commandTreeSnapshot map[string][]byte

type commandTreeChange struct {
	path   string
	kind   string
	before []byte
	after  []byte
}

func assertCommandTreeUnchanged(t *testing.T, message string, before, after commandTreeSnapshot) {
	t.Helper()
	if reflect.DeepEqual(before, after) {
		return
	}
	t.Fatalf("%s:\n%s", message, formatCommandTreeDiff(before, after))
}

func formatCommandTreeDiff(before, after commandTreeSnapshot) string {
	changes := commandTreeChanges(before, after)
	if len(changes) == 0 {
		return ""
	}

	var output strings.Builder
	output.WriteString("repository mutation details:\n")
	for _, change := range changes {
		fmt.Fprintf(&output, "  %s: %s\n", change.kind, change.path)
		if change.kind != "modified" {
			continue
		}

		prefix := commonPrefixLength(change.before, change.after)
		suffix := commonSuffixLength(change.before[prefix:], change.after[prefix:])
		beforeEnd := len(change.before) - suffix
		afterEnd := len(change.after) - suffix
		fmt.Fprintf(
			&output,
			"    byte diff (first differing byte at offset %d; before %d bytes, after %d bytes):\n",
			prefix,
			len(change.before),
			len(change.after),
		)
		fmt.Fprintf(
			&output,
			"      before: %s\n",
			formatMutationExcerpt(change.before, prefix, beforeEnd),
		)
		fmt.Fprintf(
			&output,
			"      after:  %s\n",
			formatMutationExcerpt(change.after, prefix, afterEnd),
		)
	}
	return output.String()
}

func commandTreeChanges(before, after commandTreeSnapshot) []commandTreeChange {
	paths := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		paths[path] = struct{}{}
	}
	for path := range after {
		paths[path] = struct{}{}
	}

	orderedPaths := make([]string, 0, len(paths))
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)

	changes := make([]commandTreeChange, 0, len(orderedPaths))
	for _, path := range orderedPaths {
		beforePayload, beforeExists := before[path]
		afterPayload, afterExists := after[path]
		switch {
		case !beforeExists:
			changes = append(changes, commandTreeChange{path: path, kind: "added", after: afterPayload})
		case !afterExists:
			changes = append(changes, commandTreeChange{path: path, kind: "removed", before: beforePayload})
		case !bytes.Equal(beforePayload, afterPayload):
			changes = append(changes, commandTreeChange{
				path:   path,
				kind:   "modified",
				before: beforePayload,
				after:  afterPayload,
			})
		}
	}
	return changes
}

func commonPrefixLength(before, after []byte) int {
	limit := len(before)
	if len(after) < limit {
		limit = len(after)
	}
	for index := 0; index < limit; index++ {
		if before[index] != after[index] {
			return index
		}
	}
	return limit
}

func commonSuffixLength(before, after []byte) int {
	limit := len(before)
	if len(after) < limit {
		limit = len(after)
	}
	for offset := 0; offset < limit; offset++ {
		if before[len(before)-1-offset] != after[len(after)-1-offset] {
			return offset
		}
	}
	return limit
}

func formatMutationExcerpt(payload []byte, changedStart, changedEnd int) string {
	if len(payload) == 0 {
		return `""`
	}

	start := changedStart - mutationExcerptContext
	if start < 0 {
		start = 0
	}
	end := changedEnd + mutationExcerptContext
	if end > len(payload) {
		end = len(payload)
	}
	if end < start {
		end = start
	}
	if end-start > mutationExcerptMaxBytes {
		start = changedStart - mutationExcerptMaxBytes/4
		if start < 0 {
			start = 0
		}
		end = start + mutationExcerptMaxBytes
		if end > len(payload) {
			end = len(payload)
			start = end - mutationExcerptMaxBytes
			if start < 0 {
				start = 0
			}
		}
	}

	excerpt := strconv.Quote(string(payload[start:end]))
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(payload) {
		excerpt += "…"
	}
	return excerpt
}

func TestCommandTreeDiffReportsAllChangesInStablePathOrder(t *testing.T) {
	before := commandTreeSnapshot{
		"same.txt":    []byte("same"),
		"removed.txt": []byte("gone"),
		"changed.txt": []byte("old"),
		"z-last.txt":  []byte("last"),
	}
	after := commandTreeSnapshot{
		"same.txt":    []byte("same"),
		"changed.txt": []byte("new"),
		"added.txt":   []byte("new file"),
		"z-last.txt":  []byte("last"),
	}

	got := formatCommandTreeDiff(before, after)
	want := strings.Join([]string{
		"repository mutation details:",
		"  added: added.txt",
		"  modified: changed.txt",
		"    byte diff (first differing byte at offset 0; before 3 bytes, after 3 bytes):",
		"      before: \"old\"",
		"      after:  \"new\"",
		"  removed: removed.txt",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("formatCommandTreeDiff() = %q, want %q", got, want)
	}
}

func TestCommandTreeDiffShowsFixtureByteMutationAndSilencesUnchanged(t *testing.T) {
	root := commandFixture(t)
	const path = "unrelated.txt"
	writeCommandFixture(t, root, path, "alpha\nbeta\n")
	before := commandTree(t, root)
	writeCommandFixture(t, root, path, "alpha\r\nbeta\n")
	after := commandTree(t, root)

	diff := formatCommandTreeDiff(before, after)
	for _, fragment := range []string{
		"  modified: " + path,
		`before: "alpha\nbeta\n"`,
		`after:  "alpha\r\nbeta\n"`,
	} {
		if !strings.Contains(diff, fragment) {
			t.Fatalf("formatCommandTreeDiff() = %q, want fragment %q", diff, fragment)
		}
	}
	if unchanged := formatCommandTreeDiff(after, after); unchanged != "" {
		t.Fatalf("formatCommandTreeDiff() for unchanged snapshots = %q, want empty", unchanged)
	}
}

func TestCommandTreeDiffBoundsLargeByteChanges(t *testing.T) {
	before := bytes.Repeat([]byte("a"), mutationExcerptMaxBytes*4)
	after := bytes.Repeat([]byte("b"), mutationExcerptMaxBytes*4)
	diff := formatCommandTreeDiff(
		commandTreeSnapshot{"large.txt": before},
		commandTreeSnapshot{"large.txt": after},
	)
	if !strings.Contains(diff, "…") {
		t.Fatalf("formatCommandTreeDiff() = %q, want bounded excerpt marker", diff)
	}
	if len(diff) > mutationExcerptMaxBytes*4 {
		t.Fatalf("formatCommandTreeDiff() length = %d, want <= %d", len(diff), mutationExcerptMaxBytes*4)
	}
}
