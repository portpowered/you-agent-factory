package harness

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var errNoFileAccessEvents = errors.New("strace output contained no file-access events")

type traceRecord struct {
	line       string
	pid        int
	name       string
	args       string
	result     string
	unfinished bool
	resumed    bool
}

type traceState struct {
	cwd string
	fds map[int]string
}

type tracePath struct {
	raw  string
	base string
}

var tracePID = regexp.MustCompile(`^\s*(?:\[pid\s+)?(\d+)(?:\])?\s+`)
var traceFDPath = regexp.MustCompile(`(?:AT_FDCWD|\d+)<([^>]+)>`)
var traceQuoted = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
var resumedCall = regexp.MustCompile(`^<\.\.\.\s+([[:alnum:]_]+)\s+resumed>\s*`)
var traceReturn = regexp.MustCompile(`\)\s*=\s*`)

func auditTrace(repoRoot, initialCWD string, data []byte) (*SourceTreeReadError, error) {
	canonicalRoot, err := canonicalPath(repoRoot, "")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	canonicalCWD, err := canonicalPath(initialCWD, "")
	if err != nil {
		return nil, fmt.Errorf("resolve invocation directory: %w", err)
	}

	states := make(map[int]*traceState)
	pending := make(map[int]traceRecord)
	events := 0
	unknownExitArtifact := false
	processExited := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isStraceUnknownExitArtifact(line) {
			unknownExitArtifact = true
			continue
		}
		if isStraceProcessExit(line) {
			processExited = true
			continue
		}
		record, ok := parseTraceRecord(line)
		if !ok {
			continue
		}
		events++
		if record.resumed {
			unfinished, ok := pending[record.pid]
			if !ok {
				return nil, fmt.Errorf("strace resumed %s for pid %d without an unfinished record", record.name, record.pid)
			}
			delete(pending, record.pid)
			record.name = unfinished.name
			record.args = unfinished.args
			record.line = unfinished.line + " " + record.line
		}
		state := stateFor(states, record.pid, canonicalCWD)
		paths, err := tracePaths(record, state)
		if err != nil {
			return nil, err
		}
		resolved, err := resolveTracePaths(paths)
		if err != nil {
			return nil, fmt.Errorf("resolve trace path on %q: %w", record.line, err)
		}
		for _, path := range resolved {
			if pathWithin(canonicalRoot, path) {
				return &SourceTreeReadError{Path: path, TraceLine: record.line}, nil
			}
		}
		updateTraceState(states, record, state, resolved)
		if record.unfinished {
			pending[record.pid] = record
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read strace output: %w", err)
	}
	if unknownExitArtifact && !processExited {
		return nil, fmt.Errorf("strace output contained an unknown unfinished syscall without a process exit")
	}
	if len(pending) > 0 {
		return nil, fmt.Errorf("strace output ended with unfinished file-access syscalls")
	}
	if events == 0 {
		return nil, errNoFileAccessEvents
	}
	return nil, nil
}

// strace can emit this synthetic record when a traced thread dies immediately
// after syscall entry. With -ff the record is isolated to that thread's log;
// it has no syscall name or path to audit and is not a real file-access event.
// A genuinely unfinished named syscall remains fail-closed below.
func isStraceUnknownExitArtifact(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, "???(") && strings.Contains(trimmed, "<unfinished ...>")
}

func isStraceProcessExit(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, "+++ exited") || strings.Contains(trimmed, "+++ killed")
}

func parseTraceRecord(line string) (traceRecord, bool) {
	matches := tracePID.FindStringSubmatch(line)
	pid := 0
	lineOffset := 0
	if len(matches) == 2 {
		parsedPID, err := strconv.Atoi(matches[1])
		if err != nil {
			return traceRecord{}, false
		}
		pid = parsedPID
		lineOffset = len(matches[0])
	}
	rest := strings.TrimSpace(line[lineOffset:])
	if resumed := resumedCall.FindStringSubmatch(rest); len(resumed) == 2 {
		end, resultStart := traceReturnBounds(rest)
		if end < 0 {
			return traceRecord{}, false
		}
		return traceRecord{
			line:    line,
			pid:     pid,
			name:    resumed[1],
			result:  strings.TrimSpace(rest[resultStart:]),
			resumed: true,
		}, true
	}
	open := strings.IndexByte(rest, '(')
	if open <= 0 {
		return traceRecord{}, false
	}
	name := strings.TrimSpace(rest[:open])
	end, resultStart := traceReturnBounds(rest)
	unfinished := false
	if end < 0 {
		end = strings.LastIndex(rest, " <unfinished ...>")
		unfinished = end >= 0
	}
	if end <= open {
		return traceRecord{}, false
	}
	return traceRecord{
		line:       line,
		pid:        pid,
		name:       name,
		args:       rest[open+1 : end],
		result:     traceResult(rest, resultStart, unfinished),
		unfinished: unfinished,
	}, true
}

func traceReturnBounds(rest string) (int, int) {
	matches := traceReturn.FindAllStringIndex(rest, -1)
	if len(matches) == 0 {
		return -1, -1
	}
	match := matches[len(matches)-1]
	return match[0], match[1]
}

func traceResult(rest string, resultStart int, unfinished bool) string {
	if unfinished || resultStart < 0 {
		return ""
	}
	return strings.TrimSpace(rest[resultStart:])
}

func stateFor(states map[int]*traceState, pid int, initialCWD string) *traceState {
	if state, ok := states[pid]; ok {
		return state
	}
	state := &traceState{cwd: initialCWD, fds: make(map[int]string)}
	states[pid] = state
	return state
}

func tracePaths(record traceRecord, state *traceState) ([]tracePath, error) {
	args := splitTraceArgs(record.args)
	if len(args) == 0 {
		return nil, nil
	}

	var references []tracePath
	switch record.name {
	case "open", "creat", "stat", "lstat", "stat64", "lstat64", "access", "eaccess",
		"readlink", "truncate", "chroot", "chmod", "chown", "lchown", "listxattr",
		"llistxattr", "getxattr", "lgetxattr":
		references = appendStringReference(references, args, 0, state.cwd)
	case "openat", "openat2", "newfstatat", "fstatat", "faccessat", "faccessat2", "statx",
		"readlinkat", "unlinkat", "mkdirat", "mknodat", "fchmodat", "fchownat", "utimensat",
		"name_to_handle_at":
		base, err := descriptorBase(args, 0, state)
		if err != nil {
			return nil, err
		}
		references = appendStringReference(references, args, 1, base)
	case "execve":
		references = appendStringReference(references, args, 0, state.cwd)
	case "execveat":
		base, err := descriptorBase(args, 0, state)
		if err != nil {
			return nil, err
		}
		references = appendStringReference(references, args, 1, base)
	case "getdents", "getdents64", "fstat", "fstatfs", "fstatfs64", "fchdir":
		if base, ok := descriptorPath(args, 0, state); ok {
			references = append(references, tracePath{raw: base, base: state.cwd})
		}
	default:
		for _, raw := range quotedValues(record.args) {
			references = append(references, tracePath{raw: raw, base: state.cwd})
		}
	}
	return references, nil
}

func appendStringReference(references []tracePath, args []string, index int, base string) []tracePath {
	if index >= len(args) {
		return references
	}
	raw, ok := decodeTraceString(args[index])
	if !ok || raw == "" {
		return references
	}
	return append(references, tracePath{raw: raw, base: base})
}

func descriptorBase(args []string, index int, state *traceState) (string, error) {
	if index >= len(args) {
		return "", fmt.Errorf("missing directory descriptor")
	}
	if strings.HasPrefix(strings.TrimSpace(args[index]), "AT_FDCWD") {
		return state.cwd, nil
	}
	if path, ok := descriptorPath(args, index, state); ok {
		return path, nil
	}
	return "", fmt.Errorf("directory descriptor %q was not resolved", args[index])
}

func descriptorPath(args []string, index int, state *traceState) (string, bool) {
	if index >= len(args) {
		return "", false
	}
	raw := strings.TrimSpace(args[index])
	if match := traceFDPath.FindStringSubmatch(raw); len(match) == 2 {
		return match[1], true
	}
	fdText := raw
	if comma := strings.IndexByte(fdText, '<'); comma >= 0 {
		fdText = fdText[:comma]
	}
	fd, err := strconv.Atoi(strings.TrimSpace(fdText))
	if err != nil {
		return "", false
	}
	path, ok := state.fds[fd]
	return path, ok
}

func resolveTracePaths(paths []tracePath) ([]string, error) {
	resolved := make([]string, 0, len(paths))
	for _, reference := range paths {
		path, err := canonicalPath(reference.raw, reference.base)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

func updateTraceState(states map[int]*traceState, record traceRecord, state *traceState, resolved []string) {
	args := splitTraceArgs(record.args)
	if record.name == "chdir" && len(resolved) > 0 {
		state.cwd = resolved[0]
	}
	if record.name == "fchdir" && len(resolved) > 0 {
		state.cwd = resolved[0]
	}
	if isOpenCall(record.name) && len(resolved) > 0 {
		if fd, ok := resultNumber(record.result); ok {
			state.fds[fd] = resolved[0]
		}
	}
	if record.name == "close" && len(args) > 0 {
		if fd, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil {
			delete(state.fds, fd)
		}
	}
	if isDupCall(record.name) && len(args) > 0 {
		if source, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil {
			if target, ok := resultNumber(record.result); ok {
				if path, exists := state.fds[source]; exists {
					state.fds[target] = path
				}
			}
		}
	}
	if isForkCall(record.name) {
		if child, ok := resultNumber(record.result); ok {
			states[child] = cloneTraceState(state)
		}
	}
}

func isOpenCall(name string) bool {
	return name == "open" || name == "openat" || name == "openat2" || name == "creat"
}

func isDupCall(name string) bool {
	return name == "dup" || name == "dup2" || name == "dup3" || name == "fcntl"
}

func isForkCall(name string) bool {
	return name == "clone" || name == "clone3" || name == "fork" || name == "vfork"
}

func cloneTraceState(state *traceState) *traceState {
	fds := make(map[int]string, len(state.fds))
	for fd, path := range state.fds {
		fds[fd] = path
	}
	return &traceState{cwd: state.cwd, fds: fds}
}

func resultNumber(result string) (int, bool) {
	result = strings.TrimSpace(result)
	if result == "" {
		return 0, false
	}
	end := 0
	for end < len(result) && result[end] >= '0' && result[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	number, err := strconv.Atoi(result[:end])
	return number, err == nil
}

func quotedValues(args string) []string {
	matches := traceQuoted.FindAllString(args, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if value, ok := decodeTraceString(match); ok {
			values = append(values, value)
		}
	}
	return values
}

func decodeTraceString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false
	}
	decoded, err := strconv.Unquote(value)
	return decoded, err == nil
}

func splitTraceArgs(args string) []string {
	var result []string
	start := 0
	depth := 0
	inString := false
	escaped := false
	for index, character := range args {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(args[start:index]))
				start = index + 1
			}
		}
	}
	if tail := strings.TrimSpace(args[start:]); tail != "" {
		result = append(result, tail)
	}
	return result
}
