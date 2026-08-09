package cli

import (
	"context"
	"encoding/base64"
	"net/url"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

func TestBuildListRequest_NormalizesQueryWithoutHTTP(t *testing.T) {
	nextToken := encodeCursor("work/42")
	request, err := buildListRequest(ListConfig{
		Context:      context.Background(),
		Server:       "https://factory.example",
		SessionID:    "session/alpha",
		StateName:    "in review",
		StateType:    "PROCESSING",
		Name:         "Plan & review",
		WorkTypeName: "story/type",
		TraceID:      "trace+1",
		SortBy:       "state.type",
		MaxResults:   25,
		NextToken:    nextToken,
	}, workdomain.PreparedListRequest{
		Options: workdomain.ListOptions{
			StateName: "in review", StateType: "PROCESSING", Name: "Plan & review",
			WorkTypeName: "story/type", TraceID: "trace+1", SortBy: "state.type",
			MaxResults: 25, NextToken: nextToken,
		},
		FilterSummary: "state.name,state.type,name,workTypeName,traceId,sortBy",
	})
	if err != nil {
		t.Fatalf("buildListRequest: %v", err)
	}

	wantQuery := url.Values{
		"state.name":   {"in review"},
		"state.type":   {"PROCESSING"},
		"name":         {"Plan & review"},
		"workTypeName": {"story/type"},
		"traceId":      {"trace+1"},
		"sortBy":       {"state.type"},
		"maxResults":   {"25"},
		"nextToken":    {nextToken},
	}
	if got := request.endpoint.Path; got != "/factory-sessions/session/alpha/work" {
		t.Fatalf("endpoint path = %q", got)
	}
	if got := request.endpoint.EscapedPath(); got != "/factory-sessions/session%2Falpha/work" {
		t.Fatalf("escaped endpoint path = %q", got)
	}
	if got := request.endpoint.RawQuery; got != wantQuery.Encode() {
		t.Fatalf("query = %q, want %q", got, wantQuery.Encode())
	}
	if got := request.filterSummary; got != "state.name,state.type,name,workTypeName,traceId,sortBy" {
		t.Fatalf("filter summary = %q", got)
	}
}

func encodeCursor(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
