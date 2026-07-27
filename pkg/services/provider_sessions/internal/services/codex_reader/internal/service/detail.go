package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

var safeProviderSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var codexTimestampPrefixedSessionPattern = regexp.MustCompile(`^rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-([A-Za-z0-9_-]+)\.jsonl$`)

// DefaultSessionsRoot returns the conventional Codex session storage root.
func DefaultSessionsRoot(resolveHome providersessions.ResolveHomeDirectory) (string, error) {
	home, err := resolveHome()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

// LoadDetails resolves and parses one Codex session from the configured root.
func LoadDetails(files providersessions.FileSystem, walkDirectory providersessions.CodexWalkDirectory, resolveSymlinks providersessions.CodexResolveSymlinks, root, id string) (providersessions.Detail, error) {
	return loadDetails(context.Background(), files, walkDirectory, resolveSymlinks, root, id)
}

func loadDetails(ctx context.Context, files providersessions.FileSystem, walkDirectory providersessions.CodexWalkDirectory, resolveSymlinks providersessions.CodexResolveSymlinks, root, id string) (providersessions.Detail, error) {
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}
	normalizedID := strings.TrimSpace(id)
	if !safeProviderSessionIDPattern.MatchString(normalizedID) {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}

	resolved, err := resolveCodexSessionFile(ctx, files, walkDirectory, resolveSymlinks, root, normalizedID)
	if err != nil {
		return providersessions.Detail{}, err
	}
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}

	file, err := files.Open(resolved.absolutePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return providersessions.Detail{}, providersessions.ErrSessionNotFound
		}
		return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
	}
	defer file.Close()
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}

	parsed, err := parseCodexSessionDetails(file)
	if err != nil {
		return providersessions.Detail{}, err
	}

	return detachDetail(providersessions.Detail{
		ProviderSession: providersessions.Ref{
			Provider: providersessions.ProviderCodex,
			Kind:     providers.SessionIDKind,
			ID:       normalizedID,
		},
		Source: providersessions.SourceMetadata{
			RelativePath: resolved.relativePath,
			SizeBytes:    resolved.sizeBytes,
			ModifiedAt:   resolved.modifiedAt,
		},
		Parse:      parsed.Summary,
		Transcript: parsed.Transcript,
	}), nil
}

type resolvedCodexSessionFile struct {
	absolutePath string
	relativePath string
	sizeBytes    int64
	modifiedAt   *time.Time
	layout       codexSessionFileLayout
}

type codexSessionFileLayout int

const (
	codexSessionFileLayoutExact codexSessionFileLayout = iota + 1
	codexSessionFileLayoutTimestampPrefixed
)

func resolveCodexSessionFile(ctx context.Context, files providersessions.FileSystem, walkDirectory providersessions.CodexWalkDirectory, resolveSymlinks providersessions.CodexResolveSymlinks, root, id string) (resolvedCodexSessionFile, error) {
	if walkDirectory == nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("codex session directory walker is required")
	}
	if resolveSymlinks == nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("codex session symlink resolver is required")
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	cleanRoot, resolvedRoot, err := resolveCodexSessionsRoot(ctx, files, resolveSymlinks, root)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}

	targetName := "rollout-" + id + ".jsonl"
	matches, err := collectCodexSessionMatches(ctx, walkDirectory, cleanRoot, id, targetName)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}
	if len(matches) == 0 {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
	}
	sort.Strings(matches)
	return buildResolvedCodexSessionCandidates(ctx, files, resolveSymlinks, cleanRoot, resolvedRoot, matches, targetName)
}

// Resolve locates one Codex rollout without opening or parsing it.
func Resolve(files providersessions.FileSystem, walkDirectory providersessions.CodexWalkDirectory, resolveSymlinks providersessions.CodexResolveSymlinks, root, id string) (providersessions.SourceMetadata, error) {
	resolved, err := resolveCodexSessionFile(context.Background(), files, walkDirectory, resolveSymlinks, root, id)
	if err != nil {
		return providersessions.SourceMetadata{}, err
	}
	return providersessions.SourceMetadata{
		ModifiedAt:   resolved.modifiedAt,
		RelativePath: resolved.relativePath,
		SizeBytes:    resolved.sizeBytes,
	}, nil
}

func resolveCodexSessionsRoot(ctx context.Context, files providersessions.FileSystem, resolveSymlinks providersessions.CodexResolveSymlinks, root string) (string, string, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	rootInfo, err := files.Stat(cleanRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", providersessions.ErrSessionNotFound
		}
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if !rootInfo.IsDir() {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	resolvedRoot, err := resolveSymlinks(cleanRoot)
	if err != nil {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return cleanRoot, resolvedRoot, nil
}

func collectCodexSessionMatches(ctx context.Context, walkDirectory providersessions.CodexWalkDirectory, cleanRoot, id, targetName string) ([]string, error) {
	matches := make([]string, 0, 1)
	err := walkDirectory(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path != cleanRoot && matchesCodexSessionBaseName(filepath.Base(path), id, targetName) {
			matches = append(matches, path)
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeType != 0 && entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		return nil
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, providersessions.ErrSessionStorageUnavailable
	}
	return matches, nil
}

func buildResolvedCodexSessionCandidates(ctx context.Context, files providersessions.FileSystem, resolveSymlinks providersessions.CodexResolveSymlinks, cleanRoot, resolvedRoot string, matches []string, targetName string) (resolvedCodexSessionFile, error) {
	candidates := make([]resolvedCodexSessionFile, 0, len(matches))
	for _, match := range matches {
		if err := ctx.Err(); err != nil {
			return resolvedCodexSessionFile{}, err
		}
		candidate, err := resolvedCodexSessionCandidate(ctx, files, resolveSymlinks, cleanRoot, resolvedRoot, match, targetName)
		if err != nil {
			return resolvedCodexSessionFile{}, err
		}
		candidates = append(candidates, candidate)
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	return selectResolvedCodexSessionFile(candidates)
}

func resolvedCodexSessionCandidate(ctx context.Context, files providersessions.FileSystem, resolveSymlinks providersessions.CodexResolveSymlinks, cleanRoot, resolvedRoot, match, targetName string) (resolvedCodexSessionFile, error) {
	resolvedMatch, err := resolveSymlinks(match)
	if err != nil {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	if !pathInsideRoot(resolvedRoot, resolvedMatch) {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionOutsideRoot
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	info, err := files.Stat(resolvedMatch)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
		}
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	if !info.Mode().IsRegular() {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionSourceNotRegularFile
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	rel, err := filepath.Rel(cleanRoot, match)
	if err != nil {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	modifiedAt := info.ModTime().UTC()
	return resolvedCodexSessionFile{
		absolutePath: resolvedMatch,
		relativePath: filepath.ToSlash(rel),
		sizeBytes:    info.Size(),
		modifiedAt:   &modifiedAt,
		layout:       classifyCodexSessionFileLayout(filepath.Base(match), targetName),
	}, nil
}

func matchesCodexSessionBaseName(baseName, id, exactName string) bool {
	if baseName == exactName {
		return true
	}
	matches := codexTimestampPrefixedSessionPattern.FindStringSubmatch(baseName)
	if matches == nil {
		return false
	}
	return matches[2] == id
}

// MatchesSessionBaseName reports whether a file name is a supported Codex
// rollout layout for the requested session.
func MatchesSessionBaseName(baseName, id, exactName string) bool {
	return matchesCodexSessionBaseName(baseName, id, exactName)
}

func classifyCodexSessionFileLayout(baseName, exactName string) codexSessionFileLayout {
	if baseName == exactName {
		return codexSessionFileLayoutExact
	}
	return codexSessionFileLayoutTimestampPrefixed
}

func selectResolvedCodexSessionFile(candidates []resolvedCodexSessionFile) (resolvedCodexSessionFile, error) {
	exactMatches := make([]resolvedCodexSessionFile, 0, 1)
	timestampMatches := make([]resolvedCodexSessionFile, 0, 1)
	for _, candidate := range candidates {
		switch candidate.layout {
		case codexSessionFileLayoutExact:
			exactMatches = append(exactMatches, candidate)
		case codexSessionFileLayoutTimestampPrefixed:
			timestampMatches = append(timestampMatches, candidate)
		}
	}

	switch {
	case len(exactMatches) == 1:
		return exactMatches[0], nil
	case len(exactMatches) > 1:
		return resolvedCodexSessionFile{}, providersessions.ErrAmbiguousSessionFile
	case len(timestampMatches) == 1:
		return timestampMatches[0], nil
	case len(timestampMatches) > 1:
		return resolvedCodexSessionFile{}, providersessions.ErrAmbiguousSessionFile
	default:
		return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
	}
}

func pathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ParsedDetails contains the provider-independent summary and transcript
// extracted from a Codex JSONL stream.
type ParsedDetails struct {
	Summary    providersessions.ParseSummary
	Transcript []providersessions.TranscriptEntry
}

// ParseSummary parses a Codex JSONL stream into its inspection summary.
func ParseSummary(reader io.Reader) (providersessions.ParseSummary, error) {
	parsed, err := parseCodexSessionDetails(reader)
	if err != nil {
		return providersessions.ParseSummary{}, err
	}
	return parsed.Summary, nil
}

// ParseDetails parses a Codex JSONL stream into summary and transcript data.
func ParseDetails(reader io.Reader) (ParsedDetails, error) {
	parsed, err := parseCodexSessionDetails(reader)
	if err != nil {
		return ParsedDetails{}, err
	}
	return detachParsedDetails(parsed), nil
}

// Codex JSONL reconstruction preserves source line order for transcript,
// parse-summary, and turn facts. turn_context opens a new turn; later events
// without an explicit turn inherit the current turn until the next turn_context.
// When timestamps are absent, ordering follows JSONL line order only.
// Mirrored user/assistant messages emitted as both event_msg and response_item
// message records are deduplicated. Function outputs attach to the earliest
// matching call_id within the reconstructed stream.
func parseCodexSessionDetails(reader io.Reader) (ParsedDetails, error) {
	parser := codexSessionParser{
		summary: providersessions.ParseSummary{
			Turns:         []providersessions.TurnSummary{},
			FunctionCalls: []providersessions.FunctionCallSummary{},
			Reasoning:     []providersessions.ReasoningSummary{},
			ParseErrors:   []providersessions.LineError{},
			UnknownEvents: []providersessions.UnknownEvent{},
		},
		transcript: []providersessions.TranscriptEntry{},
	}
	bufferedReader := bufio.NewReader(reader)
	lineNumber := 0
	for {
		lineBytes, err := bufferedReader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(lineBytes) == 0 {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return ParsedDetails{}, fmt.Errorf("read provider session stream: %w", err)
		}

		lineNumber++
		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		parser.summary.LineCount++
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			parser.summary.MalformedLineCount++
			parser.summary.ParseErrors = append(parser.summary.ParseErrors, providersessions.LineError{
				LineNumber: lineNumber,
				Message:    "invalid JSON event record",
			})
			continue
		}
		parser.summary.EventCount++
		parser.recordEvent(lineNumber, event)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return ParsedDetails{
		Summary:    parser.summary,
		Transcript: parser.transcript,
	}, nil
}

type codexSessionParser struct {
	summary          providersessions.ParseSummary
	transcript       []providersessions.TranscriptEntry
	currentTurnIndex int
}

func (p *codexSessionParser) recordEvent(lineNumber int, event map[string]any) {
	eventType := stringField(event, "type")
	timestamp := timeField(event, "timestamp")
	switch eventType {
	case "session_meta":
		return
	case "turn_context":
		p.startTurn(timestamp).EventCount++
	case "event_msg":
		p.recordEventMessage(lineNumber, event, timestamp)
	case "response_item":
		p.recordResponseItem(lineNumber, event, timestamp)
	default:
		p.recordUnknownEvent(lineNumber, eventType, nestedPayloadType(event))
	}
}

func (p *codexSessionParser) recordEventMessage(lineNumber int, event map[string]any, timestamp *time.Time) {
	payload, _ := event["payload"].(map[string]any)
	payloadType := stringField(payload, "type")
	switch payloadType {
	case "token_count":
		p.recordTokenUsage(payload)
	case "agent_message", "user_message", "task_started", "task_complete", "patch_apply_end":
		turn := p.ensureTurn(timestamp)
		turn.EventCount++
		p.appendEventMessageTranscript(lineNumber, payloadType, payload, timestamp, turn)
	case "agent_reasoning":
		turn := p.ensureTurn(timestamp)
		turn.EventCount++
		p.appendReasoning("agent_reasoning", payload, timestamp, lineNumber, turn)
	default:
		p.recordUnknownEvent(lineNumber, "event_msg", payloadType)
	}
}

func (p *codexSessionParser) recordResponseItem(lineNumber int, event map[string]any, timestamp *time.Time) {
	payload, _ := event["payload"].(map[string]any)
	if payload == nil {
		if item, ok := event["item"].(map[string]any); ok {
			payload = item
		} else {
			payload = event
		}
	}
	itemType := stringField(payload, "type")
	if itemType == "" {
		itemType = stringField(payload, "item.type")
	}

	turn := p.ensureTurn(timestamp)
	turn.EventCount++
	turn.ResponseItemCount++

	switch itemType {
	case "message":
		p.appendResponseMessage(payload, timestamp, lineNumber, turn)
	case "reasoning":
		p.appendReasoning(itemType, payload, timestamp, lineNumber, turn)
	case "function_call", "custom_tool_call":
		p.appendFunctionCall(itemType, payload, timestamp, lineNumber, turn)
	case "function_call_output", "custom_tool_call_output":
		p.attachFunctionOutput(itemType, payload, timestamp, lineNumber, turn)
	default:
		p.recordUnknownEvent(lineNumber, "response_item", itemType)
	}
}

func (p *codexSessionParser) startTurn(startedAt *time.Time) *providersessions.TurnSummary {
	p.summary.Turns = append(p.summary.Turns, providersessions.TurnSummary{
		Index:             len(p.summary.Turns) + 1,
		StartedAt:         startedAt,
		EventCount:        0,
		ResponseItemCount: 0,
		FunctionCallCount: 0,
		ReasoningCount:    0,
	})
	p.currentTurnIndex = len(p.summary.Turns)
	return &p.summary.Turns[p.currentTurnIndex-1]
}

func (p *codexSessionParser) ensureTurn(startedAt *time.Time) *providersessions.TurnSummary {
	if p.currentTurnIndex == 0 {
		return p.startTurn(startedAt)
	}
	turn := &p.summary.Turns[p.currentTurnIndex-1]
	if turn.StartedAt == nil && startedAt != nil {
		turn.StartedAt = startedAt
	}
	return turn
}

func (p *codexSessionParser) appendFunctionCall(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	turn.FunctionCallCount++
	order := len(p.summary.FunctionCalls) + 1
	call := providersessions.FunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallID:    stringPtrIfNotEmpty(firstStringField(payload, "call_id", "callId", "id")),
		Type:      itemType,
		Name:      stringPtrIfNotEmpty(firstStringField(payload, "name", "tool_name", "toolName")),
		Arguments: stringPtrIfNotEmpty(firstCompactField(payload, "arguments", "arguments_json", "input")),
		Status:    stringPtrIfNotEmpty(firstStringField(payload, "status")),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		Arguments:  call.Arguments,
		CallID:     call.CallID,
		LineNumber: intPtr(lineNumber),
		Name:       call.Name,
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     call.Status,
		Timestamp:  timestamp,
		TurnIndex:  call.TurnIndex,
		Type:       providersessions.TranscriptEntryType("tool_call"),
	})
}

func (p *codexSessionParser) attachFunctionOutput(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	callID := firstStringField(payload, "call_id", "callId", "id")
	output := firstCompactField(payload, "output", "content", "result")
	status := firstStringField(payload, "status")
	if status == "" && output != "" {
		status = "completed"
	}
	for i := range p.summary.FunctionCalls {
		if stringValue(p.summary.FunctionCalls[i].CallID) == callID && callID != "" {
			p.summary.FunctionCalls[i].Output = stringPtrIfNotEmpty(output)
			p.summary.FunctionCalls[i].Status = stringPtrIfNotEmpty(status)
			p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, p.summary.FunctionCalls[i].Name, p.summary.FunctionCalls[i].TurnIndex)
			return
		}
	}

	order := len(p.summary.FunctionCalls) + 1
	call := providersessions.FunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallID:    stringPtrIfNotEmpty(callID),
		Type:      itemType,
		Output:    stringPtrIfNotEmpty(output),
		Status:    stringPtrIfNotEmpty(status),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, call.Name, call.TurnIndex)
}

func (p *codexSessionParser) appendReasoning(sourceType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	turn.ReasoningCount++
	order := len(p.summary.Reasoning) + 1
	encryptedContent := firstCompactField(payload, "encrypted_content", "encryptedContent")
	encrypted := encryptedContent != ""
	reasoning := providersessions.ReasoningSummary{
		Order:            order,
		TurnIndex:        intPtr(turn.Index),
		SourceType:       sourceType,
		Text:             stringPtrIfNotEmpty(firstReasoningText(payload)),
		Summary:          stringPtrIfNotEmpty(firstCompactField(payload, "summary")),
		Encrypted:        &encrypted,
		EncryptedContent: stringPtrIfNotEmpty(encryptedContent),
	}
	p.summary.Reasoning = append(p.summary.Reasoning, reasoning)
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		Encrypted:        reasoning.Encrypted,
		EncryptedContent: reasoning.EncryptedContent,
		LineNumber:       intPtr(lineNumber),
		SourceType:       stringPtrIfNotEmpty(sourceType),
		Summary:          reasoning.Summary,
		Text:             reasoning.Text,
		Timestamp:        timestamp,
		TurnIndex:        reasoning.TurnIndex,
		Type:             providersessions.TranscriptEntryType("reasoning"),
	})
}

func (p *codexSessionParser) appendEventMessageTranscript(
	lineNumber int,
	payloadType string,
	payload map[string]any,
	timestamp *time.Time,
	turn *providersessions.TurnSummary,
) {
	entryType := providersessions.TranscriptEntryType("system_event")
	switch payloadType {
	case "user_message":
		entryType = providersessions.TranscriptEntryType("user_message")
	case "agent_message":
		entryType = providersessions.TranscriptEntryType("assistant_message")
	}

	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		LineNumber: intPtr(lineNumber),
		SourceType: stringPtrIfNotEmpty(payloadType),
		Text:       stringPtrIfNotEmpty(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendResponseMessage(
	payload map[string]any,
	timestamp *time.Time,
	lineNumber int,
	turn *providersessions.TurnSummary,
) {
	role := firstStringField(payload, "role")
	entryType := providersessions.TranscriptEntryType("assistant_message")
	if role == "user" {
		entryType = providersessions.TranscriptEntryType("user_message")
	}

	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		LineNumber: intPtr(lineNumber),
		SourceType: stringPtrIfNotEmpty("message"),
		Text:       stringPtrIfNotEmpty(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendToolOutputTranscript(
	itemType string,
	callID string,
	output string,
	status string,
	timestamp *time.Time,
	lineNumber int,
	name *string,
	turnIndex *int,
) {
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		CallID:     stringPtrIfNotEmpty(callID),
		LineNumber: intPtr(lineNumber),
		Name:       name,
		Output:     stringPtrIfNotEmpty(output),
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     stringPtrIfNotEmpty(status),
		Timestamp:  timestamp,
		TurnIndex:  turnIndex,
		Type:       providersessions.TranscriptEntryType("tool_output"),
	})
}

func (p *codexSessionParser) appendTranscriptEntry(entry providersessions.TranscriptEntry) {
	if len(p.transcript) > 0 && isDuplicateTranscriptMessage(p.transcript[len(p.transcript)-1], entry) {
		return
	}
	entry.Order = len(p.transcript) + 1
	p.transcript = append(p.transcript, entry)
}

func isDuplicateTranscriptMessage(previous, next providersessions.TranscriptEntry) bool {
	if previous.Type != next.Type || !isTranscriptMessageType(next.Type) {
		return false
	}
	if stringValue(previous.Text) == "" || stringValue(previous.Text) != stringValue(next.Text) {
		return false
	}
	if intValue(previous.TurnIndex) != intValue(next.TurnIndex) {
		return false
	}
	return isCodexMirrorMessageSource(previous.SourceType, next.SourceType)
}

func isTranscriptMessageType(entryType providersessions.TranscriptEntryType) bool {
	return entryType == providersessions.TranscriptUserMessage ||
		entryType == providersessions.TranscriptAssistantMessage
}

func isCodexMirrorMessageSource(previous, next *string) bool {
	previousSource := stringValue(previous)
	nextSource := stringValue(next)
	return isCodexMessageMirrorSource(previousSource, nextSource) ||
		isCodexMessageMirrorSource(nextSource, previousSource)
}

func isCodexMessageMirrorSource(eventMessageSource, responseItemSource string) bool {
	return (eventMessageSource == "agent_message" || eventMessageSource == "user_message") && responseItemSource == "message"
}

func (p *codexSessionParser) recordTokenUsage(payload map[string]any) {
	usage, ok := nestedMap(payload, "info.total_token_usage")
	if !ok {
		return
	}
	p.summary.TokenUsage = &providersessions.TokenUsage{
		InputTokens:           intPtrIfPresent(intField(usage, "input_tokens")),
		CachedInputTokens:     intPtrIfPresent(intField(usage, "cached_input_tokens")),
		OutputTokens:          intPtrIfPresent(intField(usage, "output_tokens")),
		ReasoningOutputTokens: intPtrIfPresent(intField(usage, "reasoning_output_tokens")),
		TotalTokens:           intPtrIfPresent(intField(usage, "total_tokens")),
	}
}

func (p *codexSessionParser) recordUnknownEvent(lineNumber int, eventType string, payloadType string) {
	p.summary.UnknownEventCount++
	p.summary.UnknownEvents = append(p.summary.UnknownEvents, providersessions.UnknownEvent{
		LineNumber:  lineNumber,
		Type:        stringPtrIfNotEmpty(eventType),
		PayloadType: stringPtrIfNotEmpty(payloadType),
	})
}

func nestedPayloadType(event map[string]any) string {
	payload, _ := event["payload"].(map[string]any)
	return stringField(payload, "type")
}

func firstReasoningText(payload map[string]any) string {
	if value := firstCompactField(payload, "text", "content"); value != "" {
		return value
	}
	if summary := firstCompactField(payload, "summary"); summary != "" {
		return summary
	}
	return ""
}

func firstMessageText(payload map[string]any) string {
	if value := firstCompactField(payload, "text", "message", "content_text"); value != "" {
		return value
	}
	if items, ok := payload["content"].([]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := firstCompactField(mapped, "text", "content", "value")
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	if value := firstCompactField(payload, "content"); value != "" {
		return value
	}
	return ""
}

func firstStringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(values, key); value != "" {
			return value
		}
	}
	return ""
}

func firstCompactField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := nestedValue(values, key); ok {
			if compact := compactSessionValue(raw); compact != "" {
				return compact
			}
		}
	}
	return ""
}

func stringField(values map[string]any, key string) string {
	raw, ok := nestedValue(values, key)
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func intField(values map[string]any, key string) (int, bool) {
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return int(value), true
	case int:
		return value, true
	case json.Number:
		parsed, err := value.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func timeField(values map[string]any, key string) *time.Time {
	value := stringField(values, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func nestedMap(values map[string]any, key string) (map[string]any, bool) {
	raw, ok := nestedValue(values, key)
	if !ok {
		return nil, false
	}
	mapped, ok := raw.(map[string]any)
	return mapped, ok
}

func nestedValue(values map[string]any, key string) (any, bool) {
	current := any(values)
	for _, segment := range strings.Split(key, ".") {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := mapped[segment]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func compactSessionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func intPtrIfPresent(value int, ok bool) *int {
	if !ok {
		return nil
	}
	return &value
}

func intPtr(value int) *int {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
