package work

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
)

type listRequestPreparationFunc func(context.Context, workservice.ListOptions) (workservice.PreparedListRequest, error)

func (prepare listRequestPreparationFunc) PrepareListRequest(
	ctx context.Context,
	options workservice.ListOptions,
) (workservice.PreparedListRequest, error) {
	return prepare(ctx, options)
}

type testListRequestPreparation struct {
	FilterSummary string
}

func (prepare testListRequestPreparation) PrepareListRequest(
	_ context.Context,
	options workservice.ListOptions,
) (workservice.PreparedListRequest, error) {
	summary := "none"
	if prepare.FilterSummary != "" {
		summary = prepare.FilterSummary
	}
	return workservice.PreparedListRequest{Options: options, FilterSummary: summary}, nil
}

func TestBuildListRequest_RejectsInvalidQueryWithoutHTTP(t *testing.T) {
	tests := []struct {
		name    string
		config  ListConfig
		wantErr string
	}{
		{name: "state type", config: ListConfig{Context: context.Background(), StateType: "UNKNOWN"}, wantErr: "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED"},
		{name: "sort", config: ListConfig{Context: context.Background(), SortBy: "name"}, wantErr: "sortBy must be state.type"},
		{name: "page size", config: ListConfig{Context: context.Background(), MaxResults: -1}, wantErr: "maxResults must be zero or greater"},
		{name: "cursor", config: ListConfig{Context: context.Background(), NextToken: "not-base64"}, wantErr: "nextToken must be valid standard base64 for a non-empty cursor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Server = "://invalid-server"
			tt.config.Output = &bytes.Buffer{}
			prepare := listRequestPreparationFunc(func(context.Context, workservice.ListOptions) (workservice.PreparedListRequest, error) {
				return workservice.PreparedListRequest{}, &workservice.ValidationError{Message: tt.wantErr}
			})
			err := NewList(testHTTPProtocol(t), prepare)(tt.config)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("buildListRequest() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestList_InvalidQueryFailsBeforeRequest(t *testing.T) {
	tests := []struct {
		name    string
		config  ListConfig
		wantErr string
	}{
		{
			name:    "state type",
			config:  ListConfig{Context: context.Background(), StateType: "UNKNOWN"},
			wantErr: "--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
		},
		{
			name:    "sort",
			config:  ListConfig{Context: context.Background(), SortBy: "name"},
			wantErr: "--sort-by must be state.type",
		},
		{
			name:    "negative page size",
			config:  ListConfig{Context: context.Background(), MaxResults: -1},
			wantErr: "maxResults must be zero or greater",
		},
		{
			name:    "malformed cursor",
			config:  ListConfig{Context: context.Background(), NextToken: "not-base64"},
			wantErr: "nextToken must be valid standard base64 for a non-empty cursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Server = "://invalid-server"
			tt.config.Output = &bytes.Buffer{}
			field := ""
			switch tt.config.StateType {
			case "UNKNOWN":
				field = workservice.FilterStateType
			}
			if tt.config.SortBy != "" {
				field = "sortBy"
			}
			prepare := listRequestPreparationFunc(func(context.Context, workservice.ListOptions) (workservice.PreparedListRequest, error) {
				return workservice.PreparedListRequest{}, &workservice.ValidationError{Field: field, Message: strings.TrimPrefix(tt.wantErr, "--state-type ")}
			})
			if field == workservice.FilterStateType {
				prepare = func(context.Context, workservice.ListOptions) (workservice.PreparedListRequest, error) {
					return workservice.PreparedListRequest{}, &workservice.ValidationError{Field: field, Message: fmt.Sprintf("%s must be one of INITIAL, PROCESSING, TERMINAL, or FAILED", field)}
				}
			}
			err := NewList(testHTTPProtocol(t), prepare)(tt.config)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("NewList(testHTTPProtocol(t), testListRequestPreparation{})() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
