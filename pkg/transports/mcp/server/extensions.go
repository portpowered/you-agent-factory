package server

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	skillsExtension = "io.modelcontextprotocol/skills"
	skillCacheTTLMs = 300000
	skillPageSize   = 100
)

// ResourceRegistration injects one protocol resource and its owner-provided
// read operation into the shared MCP server.
type ResourceRegistration struct {
	Resource *mcp.Resource
	Read     func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
}

// SkillEntry is the protocol-level manifest entry for one Agent Skill.
// Resources is either a complete []SkillResource manifest or the string
// "dynamic" when the skill's resources cannot be enumerated up front.
type SkillEntry struct {
	URI         string         `json:"uri"`
	Frontmatter map[string]any `json:"frontmatter"`
	Resources   any            `json:"resources"`
}

// SkillResource identifies one readable skill file and its content digest.
type SkillResource struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type listSkillsParams struct {
	mcp.ParamsBase
	Cursor string `json:"cursor,omitempty"`
}

type getSkillParams struct {
	mcp.ParamsBase
	URI string `json:"uri"`
}

type listSkillsResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
	mcp.Cacheable
	NextCursor string       `json:"nextCursor,omitempty"`
	Skills     []SkillEntry `json:"skills"`
}

type getSkillResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
	mcp.Cacheable
	Skill SkillEntry `json:"skill"`
}

func registerSkills(server *mcp.Server, entries []SkillEntry) error {
	byURI, err := indexSkills(entries)
	if err != nil {
		return err
	}
	if err := registerSkillsList(server, entries); err != nil {
		return err
	}
	return registerSkillGet(server, byURI)
}

func indexSkills(entries []SkillEntry) (map[string]SkillEntry, error) {
	byURI := make(map[string]SkillEntry, len(entries))
	for _, entry := range entries {
		if _, exists := byURI[entry.URI]; exists {
			return nil, fmt.Errorf("mcp server skill URI %q is duplicated", entry.URI)
		}
		if err := validateSkillEntry(entry); err != nil {
			return nil, err
		}
		byURI[entry.URI] = entry
	}
	return byURI, nil
}

func validateSkillEntry(entry SkillEntry) error {
	if entry.URI == "" || entry.Frontmatter == nil || entry.Resources == nil {
		return fmt.Errorf("mcp server skill requires URI, frontmatter, and resources")
	}
	if name, ok := entry.Frontmatter["name"].(string); !ok || name == "" {
		return fmt.Errorf("mcp server skill %q requires frontmatter name", entry.URI)
	}
	if description, ok := entry.Frontmatter["description"].(string); !ok || description == "" {
		return fmt.Errorf("mcp server skill %q requires frontmatter description", entry.URI)
	}
	if dynamic, ok := entry.Resources.(string); ok {
		if dynamic != "dynamic" {
			return fmt.Errorf("mcp server skill %q resources string must be dynamic", entry.URI)
		}
		return nil
	}
	resources := reflect.ValueOf(entry.Resources)
	if resources.Kind() != reflect.Slice || resources.Len() == 0 {
		return fmt.Errorf("mcp server skill %q resources must be a non-empty manifest or dynamic", entry.URI)
	}
	return nil
}

func registerSkillsList(server *mcp.Server, entries []SkillEntry) error {
	err := mcp.AddReceivingCustomMethod(server, "skills/list", func(_ context.Context, _ *mcp.ServerSession, params *listSkillsParams) (*listSkillsResult, error) {
		return makeListSkillsResult(entries, params)
	})
	if err != nil {
		return fmt.Errorf("register MCP skills/list: %w", err)
	}
	return nil
}

func makeListSkillsResult(entries []SkillEntry, params *listSkillsParams) (*listSkillsResult, error) {
	cursor := ""
	if params != nil {
		cursor = params.Cursor
	}
	start, err := parseSkillCursor(cursor, len(entries))
	if err != nil {
		return nil, invalidSkillParams(err.Error())
	}
	end := min(start+skillPageSize, len(entries))
	result := &listSkillsResult{
		ResultType: "complete",
		Cacheable:  mcp.Cacheable{TTLMs: skillCacheTTLMs, CacheScope: "public"},
		Skills:     append([]SkillEntry{}, entries[start:end]...),
	}
	if end < len(entries) {
		result.NextCursor = strconv.Itoa(end)
	}
	return result, nil
}

func registerSkillGet(server *mcp.Server, byURI map[string]SkillEntry) error {
	err := mcp.AddReceivingCustomMethod(server, "skills/get", func(_ context.Context, _ *mcp.ServerSession, params *getSkillParams) (*getSkillResult, error) {
		return makeGetSkillResult(byURI, params)
	})
	if err != nil {
		return fmt.Errorf("register MCP skills/get: %w", err)
	}
	return nil
}

func makeGetSkillResult(byURI map[string]SkillEntry, params *getSkillParams) (*getSkillResult, error) {
	if params == nil || params.URI == "" {
		return nil, invalidSkillParams("skill URI is required")
	}
	entry, ok := byURI[params.URI]
	if !ok {
		return nil, invalidSkillParams(fmt.Sprintf("unknown skill URI %q", params.URI))
	}
	return &getSkillResult{
		ResultType: "complete",
		Cacheable:  mcp.Cacheable{TTLMs: skillCacheTTLMs, CacheScope: "public"},
		Skill:      entry,
	}, nil
}

func parseSkillCursor(cursor string, total int) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	start, err := strconv.Atoi(cursor)
	if err != nil || start < 0 || start > total {
		return 0, fmt.Errorf("invalid skills/list cursor")
	}
	return start, nil
}

func invalidSkillParams(message string) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: message}
}
