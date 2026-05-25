package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

const (
	defaultHostedLinearEndpoint     = "https://api.linear.app/graphql"
	defaultHostedLinearPollInterval = 1 * time.Minute
	hostedLinearPageSize            = 25
	hostedLinearMaxPagesPerCycle    = 10
	hostedLinearRequestTimeout      = 30 * time.Second
)

type linearCheckpoint struct {
	IssueID   string `json:"issueId"`
	UpdatedAt string `json:"updatedAt"`
}

type linearIssue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	UpdatedAt   string
	URL         string
	Team        linearIssueTeam
	State       linearIssueState
	Assignee    *linearIssueAssignee
}

type linearIssueTeam struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type linearIssueState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type linearIssueAssignee struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type linearPollCycleResult struct {
	submissions []interfaces.SubmitRequest
	checkpoint  linearCheckpoint
	foundNewer  bool
}

type linearIssuePage struct {
	Issues []linearIssue
	Next   string
	More   bool
}

type linearPollerClient struct {
	endpoint string
	client   *http.Client
	logger   *zap.Logger
}

type linearIssueFilter struct {
	TeamIDs  []string
	StateIDs []string
}

func (fs *FactoryService) hostedPollerHTTPClient() *http.Client {
	if fs != nil && fs.cfg != nil && fs.cfg.HostedPollerHTTPClient != nil {
		return fs.cfg.HostedPollerHTTPClient
	}
	return &http.Client{Timeout: hostedLinearRequestTimeout}
}

func (fs *FactoryService) hostedPollerSecretResolver() secretResolver {
	if fs != nil && fs.cfg != nil && fs.cfg.HostedPollerSecretResolver != nil {
		return fs.cfg.HostedPollerSecretResolver
	}
	return resolveHostedSecretRef
}

func (fs *FactoryService) hostedLinearEndpoint() string {
	if fs != nil && fs.cfg != nil && strings.TrimSpace(fs.cfg.HostedLinearEndpoint) != "" {
		return strings.TrimSpace(fs.cfg.HostedLinearEndpoint)
	}
	return defaultHostedLinearEndpoint
}

func (fs *FactoryService) superviseHostedLinearPoller(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter workRequestSubmitter,
) {
	logger := fs.pollerLogger(workstation, workerDef).With(zap.String("provider", interfaces.HostedWorkerProviderLinear))
	backoffClock := fs.pollerSupervisorClock()
	attempt := 0
	logger.Info("hosted linear poller started")
	defer func() {
		logger.Info("hosted linear poller stopped", zap.String("reason", pollerStopReason(ctx.Err())))
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		attempt++
		runErr := fs.runHostedLinearPoller(ctx, runtimeCfg, workstation, workerDef, submitter, logger, backoffClock)
		if ctx.Err() != nil {
			return
		}

		backoff := pollerRestartBackoff(attempt)
		logger.Warn("hosted linear poller restarting",
			zap.Int("attempt", attempt),
			zap.Duration("backoff", backoff),
			zap.Error(runErr),
		)

		select {
		case <-ctx.Done():
			return
		case <-backoffClock.After(backoff):
		}
	}
}

func (fs *FactoryService) runHostedLinearPoller(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter workRequestSubmitter,
	logger *zap.Logger,
	clock clockwork.Clock,
) error {
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	if workerDef == nil {
		return fmt.Errorf("hosted linear poller worker is required")
	}
	if workerDef.Auth == nil || strings.TrimSpace(workerDef.Auth.SecretRef) == "" {
		return fmt.Errorf("hosted linear poller worker %q is missing auth.secretRef", workerDef.Name)
	}
	if workerDef.Linear == nil {
		return fmt.Errorf("hosted linear poller worker %q is missing linear config", workerDef.Name)
	}
	if submitter == nil {
		return fmt.Errorf("hosted linear poller submitter is required")
	}
	interval, err := hostedLinearPollInterval(workerDef.Linear)
	if err != nil {
		return err
	}

	resolver := fs.hostedPollerSecretResolver()
	apiKey, err := resolver(ctx, runtimeCfg, workerDef.Auth.SecretRef)
	if err != nil {
		return fmt.Errorf("resolve hosted linear auth %q: %w", workerDef.Auth.SecretRef, err)
	}

	pollerClient := linearPollerClient{
		endpoint: fs.hostedLinearEndpoint(),
		client:   fs.hostedPollerHTTPClient(),
		logger:   logger,
	}
	checkpointPath := hostedLinearCheckpointPath(runtimeCfg, workstation, workerDef)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		result, err := runHostedLinearPollCycle(ctx, pollerClient, runtimeCfg, workstation, workerDef, submitter, checkpointPath, apiKey, logger)
		if err != nil {
			return err
		}
		if result.foundNewer {
			logger.Info("hosted linear poller cycle completed",
				zap.Int("submissions", len(result.submissions)),
				zap.String("checkpoint_issue_id", result.checkpoint.IssueID),
				zap.String("checkpoint_updated_at", result.checkpoint.UpdatedAt),
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-clock.After(interval):
		}
	}
}

func runHostedLinearPollCycle(
	ctx context.Context,
	client linearPollerClient,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter workRequestSubmitter,
	checkpointPath string,
	apiKey string,
	logger *zap.Logger,
) (linearPollCycleResult, error) {
	checkpoint, err := loadLinearCheckpoint(checkpointPath)
	if err != nil {
		return linearPollCycleResult{}, err
	}

	issues, nextCheckpoint, err := collectHostedLinearIssues(ctx, client, workerDef, checkpoint, apiKey, logger)
	if err != nil {
		return linearPollCycleResult{}, err
	}
	foundNewer := nextCheckpoint != checkpoint && nextCheckpoint.IssueID != ""
	if !foundNewer {
		return linearPollCycleResult{}, nil
	}

	submissions, err := hostedLinearSubmissions(workstation, workerDef, issues)
	if err != nil {
		return linearPollCycleResult{}, err
	}
	if len(submissions) > 0 {
		request := requests.WorkRequestFromSubmitRequests(submissions)
		if err := submitter(ctx, request); err != nil {
			return linearPollCycleResult{}, err
		}
	}
	if err := saveLinearCheckpoint(checkpointPath, nextCheckpoint); err != nil {
		return linearPollCycleResult{}, err
	}
	return linearPollCycleResult{
		submissions: submissions,
		checkpoint:  nextCheckpoint,
		foundNewer:  true,
	}, nil
}

func collectHostedLinearIssues(
	ctx context.Context,
	client linearPollerClient,
	workerDef *interfaces.WorkerConfig,
	checkpoint linearCheckpoint,
	apiKey string,
	logger *zap.Logger,
) ([]linearIssue, linearCheckpoint, error) {
	var collected []linearIssue
	cursor := ""
	pages := 0
	foundCheckpoint := false
	nextCheckpoint := linearCheckpoint{}

	for pages < hostedLinearMaxPagesPerCycle {
		page, err := client.fetchIssuesPage(ctx, apiKey, cursor, hostedLinearIssueFilterFromConfig(workerDef.Linear))
		if err != nil {
			return nil, linearCheckpoint{}, err
		}
		pages++
		nextCheckpoint = advanceLinearCheckpoint(nextCheckpoint, page.Issues)
		pageIssues, pageFoundCheckpoint := collectNewHostedLinearPageIssues(page.Issues, checkpoint, workerDef.Linear)
		collected = append(collected, pageIssues...)
		foundCheckpoint = foundCheckpoint || pageFoundCheckpoint
		if shouldStopHostedLinearPaging(foundCheckpoint, page) {
			break
		}
		cursor = page.Next
	}

	if pages == hostedLinearMaxPagesPerCycle && !foundCheckpoint && checkpoint.IssueID != "" {
		logger.Warn("hosted linear poller hit bounded resume page limit",
			zap.Int("page_limit", hostedLinearMaxPagesPerCycle),
			zap.String("checkpoint_issue_id", checkpoint.IssueID),
			zap.String("checkpoint_updated_at", checkpoint.UpdatedAt),
		)
	}

	sort.SliceStable(collected, func(i, j int) bool {
		if collected[i].UpdatedAt == collected[j].UpdatedAt {
			return collected[i].ID < collected[j].ID
		}
		return collected[i].UpdatedAt < collected[j].UpdatedAt
	})

	return collected, nextCheckpoint, nil
}

func advanceLinearCheckpoint(current linearCheckpoint, issues []linearIssue) linearCheckpoint {
	if current.IssueID != "" || len(issues) == 0 {
		return current
	}
	return linearCheckpoint{
		IssueID:   issues[0].ID,
		UpdatedAt: issues[0].UpdatedAt,
	}
}

func collectNewHostedLinearPageIssues(
	issues []linearIssue,
	checkpoint linearCheckpoint,
	cfg *interfaces.HostedLinearWorkerConfig,
) ([]linearIssue, bool) {
	collected := make([]linearIssue, 0, len(issues))
	for _, issue := range issues {
		if issueMatchesLinearCheckpoint(issue, checkpoint) {
			return collected, true
		}
		if hostedLinearIssueMatchesFilters(issue, cfg) {
			collected = append(collected, issue)
		}
	}
	return collected, false
}

func issueMatchesLinearCheckpoint(issue linearIssue, checkpoint linearCheckpoint) bool {
	return checkpoint.IssueID != "" && issue.ID == checkpoint.IssueID && issue.UpdatedAt == checkpoint.UpdatedAt
}

func shouldStopHostedLinearPaging(foundCheckpoint bool, page linearIssuePage) bool {
	return foundCheckpoint || !page.More || page.Next == ""
}

func hostedLinearIssueMatchesFilters(issue linearIssue, cfg *interfaces.HostedLinearWorkerConfig) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.TeamIDs) > 0 && !containsString(cfg.TeamIDs, issue.Team.ID) {
		return false
	}
	if len(cfg.StateIDs) > 0 && !containsString(cfg.StateIDs, issue.State.ID) {
		return false
	}
	return true
}

func hostedLinearSubmissions(
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	issues []linearIssue,
) ([]interfaces.SubmitRequest, error) {
	if len(issues) == 0 {
		return nil, nil
	}
	requestID := hostedLinearBatchRequestID(workstation.Name, issues)
	submissions := make([]interfaces.SubmitRequest, 0, len(issues))
	for _, issue := range issues {
		payload, err := hostedLinearIssuePayload(issue, workerDef.Linear)
		if err != nil {
			return nil, fmt.Errorf("marshal linear issue payload %q: %w", issue.ID, err)
		}
		submissions = append(submissions, interfaces.SubmitRequest{
			RequestID:   requestID,
			WorkID:      "linear:" + issue.ID,
			Name:        hostedLinearIssueName(issue),
			WorkTypeID:  workerDef.Linear.Mapping.WorkType,
			TargetState: workerDef.Linear.Mapping.State,
			TraceID:     hostedLinearIssueTraceID(issue),
			Payload:     payload,
			Tags: map[string]string{
				"external_source":         "linear",
				"linear_issue_id":         issue.ID,
				"linear_issue_identifier": issue.Identifier,
				"linear_team_id":          issue.Team.ID,
				"linear_state_id":         issue.State.ID,
				"poller_workstation":      workstation.Name,
				"poller_worker":           workerDef.Name,
			},
		})
	}
	return submissions, nil
}

func hostedLinearIssuePayload(issue linearIssue, cfg *interfaces.HostedLinearWorkerConfig) ([]byte, error) {
	payload := map[string]any{
		"source": "linear",
		"issue": map[string]any{
			"id":          issue.ID,
			"identifier":  issue.Identifier,
			"title":       issue.Title,
			"description": issue.Description,
			"updatedAt":   issue.UpdatedAt,
			"url":         issue.URL,
			"team": map[string]any{
				"id":   issue.Team.ID,
				"key":  issue.Team.Key,
				"name": issue.Team.Name,
			},
			"state": map[string]any{
				"id":   issue.State.ID,
				"name": issue.State.Name,
				"type": issue.State.Type,
			},
		},
	}
	if issue.Assignee != nil {
		payload["issue"].(map[string]any)["assignee"] = map[string]any{
			"id":    issue.Assignee.ID,
			"name":  issue.Assignee.Name,
			"email": issue.Assignee.Email,
		}
		if cfg != nil && cfg.Claim != nil && strings.TrimSpace(cfg.Claim.AssigneeField) != "" {
			payload["claims"] = map[string]any{
				cfg.Claim.AssigneeField: issue.Assignee.Email,
			}
		}
	}
	return json.Marshal(payload)
}

func hostedLinearIssueName(issue linearIssue) string {
	if strings.TrimSpace(issue.Identifier) != "" {
		return "linear-" + strings.ToLower(issue.Identifier)
	}
	return "linear-" + issue.ID
}

func hostedLinearIssueTraceID(issue linearIssue) string {
	return "linear:" + issue.ID + ":" + issue.UpdatedAt
}

func hostedLinearBatchRequestID(workstationName string, issues []linearIssue) string {
	h := sha256.New()
	_, _ = io.WriteString(h, workstationName)
	for _, issue := range issues {
		_, _ = io.WriteString(h, "\n")
		_, _ = io.WriteString(h, issue.ID)
		_, _ = io.WriteString(h, "|")
		_, _ = io.WriteString(h, issue.UpdatedAt)
	}
	return "linear-batch-" + hex.EncodeToString(h.Sum(nil)[:12])
}

func hostedLinearPollInterval(cfg *interfaces.HostedLinearWorkerConfig) (time.Duration, error) {
	if cfg == nil || strings.TrimSpace(cfg.PollInterval) == "" {
		return defaultHostedLinearPollInterval, nil
	}
	interval, err := time.ParseDuration(strings.TrimSpace(cfg.PollInterval))
	if err != nil {
		return 0, fmt.Errorf("invalid hosted linear pollInterval %q: %w", cfg.PollInterval, err)
	}
	if interval <= 0 {
		return 0, fmt.Errorf("invalid hosted linear pollInterval %q: must be > 0", cfg.PollInterval)
	}
	return interval, nil
}

func hostedLinearCheckpointPath(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
) string {
	baseDir := ""
	if runtimeCfg != nil {
		baseDir = runtimeCfg.RuntimeBaseDir()
		if baseDir == "" {
			baseDir = runtimeCfg.FactoryDir()
		}
	}
	return filepath.Join(baseDir, ".infinite-you", "poller-checkpoints", sanitizePathSegment(workstation.Name+"--"+workerDef.Name)+"--linear.json")
}

func loadLinearCheckpoint(path string) (linearCheckpoint, error) {
	if strings.TrimSpace(path) == "" {
		return linearCheckpoint{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return linearCheckpoint{}, nil
		}
		return linearCheckpoint{}, fmt.Errorf("read hosted linear checkpoint: %w", err)
	}
	var checkpoint linearCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return linearCheckpoint{}, fmt.Errorf("decode hosted linear checkpoint: %w", err)
	}
	return checkpoint, nil
}

func saveLinearCheckpoint(path string, checkpoint linearCheckpoint) error {
	if strings.TrimSpace(path) == "" || checkpoint.IssueID == "" || checkpoint.UpdatedAt == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hosted linear checkpoint dir: %w", err)
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode hosted linear checkpoint: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write hosted linear checkpoint: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit hosted linear checkpoint: %w", err)
	}
	return nil
}

func resolveHostedSecretRef(_ context.Context, runtimeCfg interfaces.RuntimeConfigLookup, secretRef string) (string, error) {
	ref := strings.TrimSpace(secretRef)
	if ref == "" {
		return "", fmt.Errorf("secret ref is required")
	}

	if value := strings.TrimSpace(os.Getenv(hostedSecretEnvName(ref))); value != "" {
		return value, nil
	}

	if runtimeCfg == nil {
		return "", fmt.Errorf("runtime config is required")
	}
	baseDir := runtimeCfg.RuntimeBaseDir()
	if baseDir == "" {
		baseDir = runtimeCfg.FactoryDir()
	}
	if baseDir == "" {
		return "", fmt.Errorf("runtime base dir is required")
	}

	secretPath, err := hostedSecretPath(baseDir, ref)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("read secret file %q: %w", secretPath, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("secret file %q is empty", secretPath)
	}
	return value, nil
}

func hostedSecretEnvName(secretRef string) string {
	var builder strings.Builder
	builder.WriteString("INFINITE_YOU_SECRET_")
	for _, r := range strings.ToUpper(strings.TrimSpace(secretRef)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	return builder.String()
}

func hostedSecretPath(baseDir, secretRef string) (string, error) {
	if filepath.IsAbs(secretRef) {
		return filepath.Clean(secretRef), nil
	}
	cleanRef := filepath.Clean(filepath.FromSlash(secretRef))
	if cleanRef == "." || strings.HasPrefix(cleanRef, ".."+string(filepath.Separator)) || cleanRef == ".." {
		return "", fmt.Errorf("secret ref %q must stay within the runtime base dir", secretRef)
	}
	return filepath.Join(baseDir, cleanRef), nil
}

func sanitizePathSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unnamed"
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func hostedLinearIssueFilterFromConfig(cfg *interfaces.HostedLinearWorkerConfig) linearIssueFilter {
	if cfg == nil {
		return linearIssueFilter{}
	}
	return linearIssueFilter{
		TeamIDs:  append([]string(nil), cfg.TeamIDs...),
		StateIDs: append([]string(nil), cfg.StateIDs...),
	}
}

func hostedLinearIssueFilterClause(filter linearIssueFilter) string {
	clauses := make([]string, 0, 2)
	if len(filter.TeamIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("team: { id: { in: %s } }", hostedLinearGraphQLStringList(filter.TeamIDs)))
	}
	if len(filter.StateIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("state: { id: { in: %s } }", hostedLinearGraphQLStringList(filter.StateIDs)))
	}
	if len(clauses) == 0 {
		return ""
	}
	return ", filter: { " + strings.Join(clauses, " ") + " }"
}

func hostedLinearGraphQLStringList(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (c linearPollerClient) fetchIssuesPage(ctx context.Context, apiKey, cursor string, filter linearIssueFilter) (linearIssuePage, error) {
	filterClause := hostedLinearIssueFilterClause(filter)
	body := map[string]any{
		"query": fmt.Sprintf(`query HostedLinearPollerIssues($first: Int!, $after: String) {
  issues(first: $first, after: $after, orderBy: updatedAt%s) {
    nodes {
      id
      identifier
      title
      description
      updatedAt
      url
      team {
        id
        key
        name
      }
      state {
        id
        name
        type
      }
      assignee {
        id
        name
        email
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`, filterClause),
		"variables": map[string]any{
			"first": hostedLinearPageSize,
			"after": cursor,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return linearIssuePage{}, fmt.Errorf("encode hosted linear graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return linearIssuePage{}, fmt.Errorf("build hosted linear graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return linearIssuePage{}, fmt.Errorf("hosted linear graphql request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return linearIssuePage{}, fmt.Errorf("read hosted linear graphql response: %w", err)
	}

	var decoded struct {
		Data struct {
			Issues struct {
				Nodes    []linearIssue `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		} `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return linearIssuePage{}, fmt.Errorf("decode hosted linear graphql response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		code := decoded.Errors[0].Extensions.Code
		msg := decoded.Errors[0].Message
		if code == "RATELIMITED" {
			return linearIssuePage{}, fmt.Errorf("hosted linear graphql rate limited: %s", msg)
		}
		return linearIssuePage{}, fmt.Errorf("hosted linear graphql error: %s", msg)
	}
	if resp.StatusCode >= 400 {
		return linearIssuePage{}, fmt.Errorf("hosted linear graphql returned status %d", resp.StatusCode)
	}
	return linearIssuePage{
		Issues: decoded.Data.Issues.Nodes,
		Next:   decoded.Data.Issues.PageInfo.EndCursor,
		More:   decoded.Data.Issues.PageInfo.HasNextPage,
	}, nil
}
