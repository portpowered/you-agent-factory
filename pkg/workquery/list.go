package workquery

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	FilterStateName    = "state.name"
	FilterStateType    = "state.type"
	FilterName         = "name"
	FilterWorkTypeName = "workTypeName"
	FilterTraceID      = "traceId"

	SortByStateType = "state.type"
)

// ListOptions contains the filters, ordering, and pagination controls for a
// work-list query. Filters are keyed by their public query parameter names.
type ListOptions struct {
	Filters    map[string]string
	SortBy     string
	MaxResults int
	NextToken  string
}

// ListQuery is a validated work-list query ready for use at a transport
// boundary. Its values are immutable through the exported API.
type ListQuery struct {
	values        url.Values
	filterSummary string
}

// NormalizeList validates options and returns their canonical public query
// values and active-filter summary.
func NormalizeList(options ListOptions) (ListQuery, error) {
	if err := validateFilterKeys(options.Filters); err != nil {
		return ListQuery{}, err
	}
	if stateType := options.Filters[FilterStateType]; stateType != "" &&
		!ValidWorkStateType(factoryapi.WorkStateType(stateType)) {
		return ListQuery{}, fmt.Errorf("%s must be one of INITIAL, PROCESSING, TERMINAL, or FAILED", FilterStateType)
	}
	if options.SortBy != "" && options.SortBy != SortByStateType {
		return ListQuery{}, fmt.Errorf("sortBy must be %s", SortByStateType)
	}
	if options.MaxResults < 0 {
		return ListQuery{}, fmt.Errorf("maxResults must be zero or greater")
	}
	if err := validateNextToken(options.NextToken); err != nil {
		return ListQuery{}, err
	}

	values := make(url.Values)
	active := make([]string, 0, len(options.Filters)+1)
	for _, key := range []string{
		FilterStateName,
		FilterStateType,
		FilterName,
		FilterWorkTypeName,
		FilterTraceID,
	} {
		if value := options.Filters[key]; value != "" {
			values.Set(key, value)
			active = append(active, key)
		}
	}
	if options.SortBy != "" {
		values.Set("sortBy", options.SortBy)
		active = append(active, "sortBy")
	}
	if options.MaxResults > 0 {
		values.Set("maxResults", strconv.Itoa(options.MaxResults))
	}
	if options.NextToken != "" {
		values.Set("nextToken", options.NextToken)
	}

	summary := "none"
	if len(active) > 0 {
		summary = strings.Join(active, ",")
	}
	return ListQuery{values: values, filterSummary: summary}, nil
}

// Values returns a copy of the normalized public query values.
func (q ListQuery) Values() url.Values {
	values := make(url.Values, len(q.values))
	for key, entries := range q.values {
		values[key] = append([]string(nil), entries...)
	}
	return values
}

// FilterSummary returns the active filter and sort keys in canonical order.
func (q ListQuery) FilterSummary() string {
	return q.filterSummary
}

func validateFilterKeys(filters map[string]string) error {
	unsupported := make([]string, 0)
	for key := range filters {
		if !supportedFilterKey(key) {
			unsupported = append(unsupported, key)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("unsupported work-list filter %q", unsupported[0])
}

func supportedFilterKey(key string) bool {
	switch key {
	case FilterStateName, FilterStateType, FilterName, FilterWorkTypeName, FilterTraceID:
		return true
	default:
		return false
	}
}

func validateNextToken(nextToken string) error {
	if nextToken == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil || len(decoded) == 0 {
		return fmt.Errorf("nextToken must be valid standard base64 for a non-empty cursor")
	}
	return nil
}
