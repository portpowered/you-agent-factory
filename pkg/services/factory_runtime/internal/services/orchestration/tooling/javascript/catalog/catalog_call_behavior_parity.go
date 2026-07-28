package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
)

// RepresentativeCallBehaviorPaths are the terminal and composition symbols whose
// catalog call metadata must match the reviewed B03 call-behavior baseline.
var RepresentativeCallBehaviorPaths = []string{
	"workflow.final",
	"workflow.checkpoint",
	"agent.run",
	"parallel",
	"pipeline",
}

// CallBehaviorParityIssue records one catalog call-metadata mismatch by path.
type CallBehaviorParityIssue struct {
	Code      string
	SymbolKey string
	Path      string
	Field     string
	Message   string
}

// CatalogCallBehaviorParityIssues compares representative catalog symbol call
// metadata to the installed call-behavior baseline.
func CatalogCallBehaviorParityIssues(
	document map[string]any,
	callInventory callbehavior.Inventory,
) ([]CallBehaviorParityIssue, error) {
	symbols, err := catalogSymbolsFromDocument(document)
	if err != nil {
		return nil, err
	}
	byPath := catalogSymbolsByInstalledPath(symbols)
	wantByPath := callBehaviorRecordsByPath(callInventory)

	var issues []CallBehaviorParityIssue
	for _, path := range RepresentativeCallBehaviorPaths {
		want, ok := wantByPath[path]
		if !ok {
			return nil, fmt.Errorf("call-behavior baseline missing representative path %q", path)
		}
		entry, ok := byPath[path]
		if !ok {
			issues = append(issues, CallBehaviorParityIssue{
				Code:    "javascript.call_behavior.missing",
				Path:    path,
				Message: fmt.Sprintf("catalog missing representative symbol path %s", strconv.Quote(path)),
			})
			continue
		}
		issues = append(issues, catalogSymbolCallBehaviorParityIssues(entry.symbolKey, path, entry.symbol, want)...)
	}

	sort.Slice(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		return left.Message < right.Message
	})
	return issues, nil
}

// VerifyCatalogCallBehaviorParity ensures representative catalog call metadata
// matches the installed call-behavior baseline.
func VerifyCatalogCallBehaviorParity(
	document map[string]any,
	callInventory callbehavior.Inventory,
) error {
	issues, err := CatalogCallBehaviorParityIssues(document, callInventory)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		return nil
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.SymbolKey != "" && issue.Field != "" {
			messages = append(messages, fmt.Sprintf(
				"%s at /symbols/%s/%s",
				issue.Message,
				issue.SymbolKey,
				issue.Field,
			))
			continue
		}
		messages = append(messages, issue.Message)
	}
	return fmt.Errorf("catalog call-behavior parity failed: %s", strings.Join(messages, "; "))
}

func catalogSymbolsFromDocument(document map[string]any) (map[string]map[string]any, error) {
	symbolsValue, ok := document["symbols"]
	if !ok {
		return nil, fmt.Errorf("catalog document missing symbols")
	}
	symbols, ok := symbolsValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog symbols is not an object")
	}
	out := make(map[string]map[string]any, len(symbols))
	for key, symbolValue := range symbols {
		symbol, ok := symbolValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog symbol %q is not an object", key)
		}
		out[key] = symbol
	}
	return out, nil
}

func catalogSymbolsByInstalledPath(symbols map[string]map[string]any) map[string]struct {
	symbol    map[string]any
	symbolKey string
} {
	byPath := make(map[string]struct {
		symbol    map[string]any
		symbolKey string
	}, len(symbols))
	for key, symbol := range symbols {
		path, _ := symbol["path"].(string)
		if path == "" {
			continue
		}
		byPath[path] = struct {
			symbol    map[string]any
			symbolKey string
		}{symbol: symbol, symbolKey: key}
	}
	return byPath
}

func callBehaviorRecordsByPath(inventory callbehavior.Inventory) map[string]callbehavior.CallBehaviorRecord {
	byPath := make(map[string]callbehavior.CallBehaviorRecord, len(inventory.Records))
	for _, record := range inventory.Records {
		byPath[record.Path] = record
	}
	return byPath
}

func catalogSymbolCallBehaviorParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want callbehavior.CallBehaviorRecord,
) []CallBehaviorParityIssue {
	var issues []CallBehaviorParityIssue
	appendIssue := func(field, message string) {
		issues = append(issues, CallBehaviorParityIssue{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   message,
		})
	}

	gotAsync, ok := symbol["async"].(bool)
	if !ok {
		appendIssue("async", fmt.Sprintf("catalog %s async is missing or not boolean", strconv.Quote(path)))
	} else if gotAsync != want.Async {
		appendIssue("async", fmt.Sprintf(
			"catalog %s async = %v, want %v",
			strconv.Quote(path),
			gotAsync,
			want.Async,
		))
	}

	issues = append(issues, catalogParameterParityIssues(symbolKey, path, symbol, want.Parameters)...)
	issues = append(issues, catalogReturnParityIssues(symbolKey, path, symbol, want.Return)...)
	issues = append(issues, catalogStringSliceParityIssues(symbolKey, path, "emittedRecords", symbol, want.EmittedRecords)...)
	issues = append(issues, catalogErrorParityIssues(symbolKey, path, symbol, want.Errors)...)
	issues = append(issues, catalogPolicyCheckParityIssues(symbolKey, path, symbol, want.PolicyChecks)...)
	issues = append(issues, catalogStringFieldParityIssues(symbolKey, path, "determinism", symbol, want.Determinism)...)
	issues = append(issues, catalogStringFieldParityIssues(symbolKey, path, "resumeNotes", symbol, want.ResumeNotes)...)
	issues = append(issues, catalogCallbackParityIssues(symbolKey, path, symbol, want.Callback)...)

	return issues
}

func catalogParameterParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want []callbehavior.Parameter,
) []CallBehaviorParityIssue {
	field := "parameters"
	if len(want) == 0 {
		if params, ok := symbol[field].([]any); ok && len(params) > 0 {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s parameters = %d entries, want none", strconv.Quote(path), len(params)),
			}}
		}
		return nil
	}

	paramsValue, ok := symbol[field].([]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s parameters missing or not an array", strconv.Quote(path)),
		}}
	}
	if len(paramsValue) != len(want) {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message: fmt.Sprintf(
				"catalog %s parameter count = %d, want %d",
				strconv.Quote(path),
				len(paramsValue),
				len(want),
			),
		}}
	}

	var issues []CallBehaviorParityIssue
	for i, wantParam := range want {
		param, ok := paramsValue[i].(map[string]any)
		if !ok {
			issues = append(issues, CallBehaviorParityIssue{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s parameters[%d] is not an object", strconv.Quote(path), i),
			})
			continue
		}
		issues = append(issues, catalogSingleParameterParityIssues(symbolKey, path, field, i, param, wantParam)...)
	}
	return issues
}

func catalogSingleParameterParityIssues(
	symbolKey string,
	path string,
	field string,
	index int,
	param map[string]any,
	wantParam callbehavior.Parameter,
) []CallBehaviorParityIssue {
	var issues []CallBehaviorParityIssue
	appendMismatch := func(message string) {
		issues = append(issues, CallBehaviorParityIssue{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   message,
		})
	}

	if got, _ := param["name"].(string); got != wantParam.Name {
		appendMismatch(fmt.Sprintf(
			"catalog %s parameters[%d].name = %q, want %q",
			strconv.Quote(path),
			index,
			got,
			wantParam.Name,
		))
	}
	if got, _ := param["required"].(bool); got != wantParam.Required {
		appendMismatch(fmt.Sprintf(
			"catalog %s parameters[%d].required = %v, want %v",
			strconv.Quote(path),
			index,
			got,
			wantParam.Required,
		))
	}
	if got, _ := param["type"].(string); got != wantParam.Type {
		appendMismatch(fmt.Sprintf(
			"catalog %s parameters[%d].type = %q, want %q",
			strconv.Quote(path),
			index,
			got,
			wantParam.Type,
		))
	}
	gotRest, _ := param["rest"].(bool)
	if gotRest != wantParam.Rest {
		appendMismatch(fmt.Sprintf(
			"catalog %s parameters[%d].rest = %v, want %v",
			strconv.Quote(path),
			index,
			gotRest,
			wantParam.Rest,
		))
	}
	if wantParam.Default != "" {
		got, _ := param["default"].(string)
		if got != wantParam.Default {
			appendMismatch(fmt.Sprintf(
				"catalog %s parameters[%d].default = %q, want %q",
				strconv.Quote(path),
				index,
				got,
				wantParam.Default,
			))
		}
	}
	return issues
}

func catalogReturnParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want *callbehavior.ReturnBehavior,
) []CallBehaviorParityIssue {
	field := "return"
	if want == nil {
		if returnValue, ok := symbol[field]; ok && returnValue != nil {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s return present, want none", strconv.Quote(path)),
			}}
		}
		return nil
	}

	returnValue, ok := symbol[field].(map[string]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s return missing or not an object", strconv.Quote(path)),
		}}
	}

	if want.Async {
		if got, _ := returnValue["kind"].(string); got != "promise" {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s return.kind = %q, want promise", strconv.Quote(path), got),
			}}
		}
		if got, _ := returnValue["type"].(string); got != want.PromiseType {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message: fmt.Sprintf(
					"catalog %s return.type = %q, want %q",
					strconv.Quote(path),
					got,
					want.PromiseType,
				),
			}}
		}
		return nil
	}

	if got, _ := returnValue["kind"].(string); got != "sync" {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s return.kind = %q, want sync", strconv.Quote(path), got),
		}}
	}
	if got, _ := returnValue["type"].(string); got != want.SyncType {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message: fmt.Sprintf(
				"catalog %s return.type = %q, want %q",
				strconv.Quote(path),
				got,
				want.SyncType,
			),
		}}
	}
	return nil
}

func catalogStringSliceParityIssues(
	symbolKey string,
	path string,
	field string,
	symbol map[string]any,
	want []string,
) []CallBehaviorParityIssue {
	if len(want) == 0 {
		return nil
	}
	gotValue, ok := symbol[field].([]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s %s missing or not an array", strconv.Quote(path), field),
		}}
	}
	got := make([]string, 0, len(gotValue))
	for _, item := range gotValue {
		record, ok := item.(string)
		if !ok {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s %s contains non-string entry", strconv.Quote(path), field),
			}}
		}
		got = append(got, record)
	}
	if len(got) != len(want) {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s %s = %v, want %v", strconv.Quote(path), field, got, want),
		}}
	}
	for i := range want {
		if got[i] != want[i] {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message: fmt.Sprintf(
					"catalog %s %s[%d] = %q, want %q",
					strconv.Quote(path),
					field,
					i,
					got[i],
					want[i],
				),
			}}
		}
	}
	return nil
}

func catalogErrorParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want []callbehavior.ErrorCase,
) []CallBehaviorParityIssue {
	field := "errors"
	if len(want) == 0 {
		return nil
	}
	gotValue, ok := symbol[field].([]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s errors missing or not an array", strconv.Quote(path)),
		}}
	}
	if len(gotValue) != len(want) {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message: fmt.Sprintf(
				"catalog %s error count = %d, want %d",
				strconv.Quote(path),
				len(gotValue),
				len(want),
			),
		}}
	}

	var issues []CallBehaviorParityIssue
	for i, wantErr := range want {
		errValue, ok := gotValue[i].(map[string]any)
		if !ok {
			issues = append(issues, CallBehaviorParityIssue{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s errors[%d] is not an object", strconv.Quote(path), i),
			})
			continue
		}
		for _, check := range []struct {
			key  string
			want string
		}{
			{"condition", wantErr.Condition},
			{"type", wantErr.Type},
			{"message", wantErr.Message},
		} {
			if got, _ := errValue[check.key].(string); got != check.want {
				issues = append(issues, CallBehaviorParityIssue{
					Code:      "javascript.call_behavior.mismatch",
					SymbolKey: symbolKey,
					Path:      path,
					Field:     field,
					Message: fmt.Sprintf(
						"catalog %s errors[%d].%s = %q, want %q",
						strconv.Quote(path),
						i,
						check.key,
						got,
						check.want,
					),
				})
			}
		}
	}
	return issues
}

func catalogPolicyCheckParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want []callbehavior.PolicyCheck,
) []CallBehaviorParityIssue {
	field := "policyChecks"
	if len(want) == 0 {
		return nil
	}
	gotValue, ok := symbol[field].([]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s policyChecks missing or not an array", strconv.Quote(path)),
		}}
	}
	if len(gotValue) != len(want) {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message: fmt.Sprintf(
				"catalog %s policyChecks count = %d, want %d",
				strconv.Quote(path),
				len(gotValue),
				len(want),
			),
		}}
	}

	var issues []CallBehaviorParityIssue
	for i, wantCheck := range want {
		checkValue, ok := gotValue[i].(map[string]any)
		if !ok {
			issues = append(issues, CallBehaviorParityIssue{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s policyChecks[%d] is not an object", strconv.Quote(path), i),
			})
			continue
		}
		if got, _ := checkValue["kind"].(string); got != wantCheck.Kind {
			issues = append(issues, CallBehaviorParityIssue{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message: fmt.Sprintf(
					"catalog %s policyChecks[%d].kind = %q, want %q",
					strconv.Quote(path),
					i,
					got,
					wantCheck.Kind,
				),
			})
		}
		if wantCheck.Field != "" {
			if got, _ := checkValue["field"].(string); got != wantCheck.Field {
				issues = append(issues, CallBehaviorParityIssue{
					Code:      "javascript.call_behavior.mismatch",
					SymbolKey: symbolKey,
					Path:      path,
					Field:     field,
					Message: fmt.Sprintf(
						"catalog %s policyChecks[%d].field = %q, want %q",
						strconv.Quote(path),
						i,
						got,
						wantCheck.Field,
					),
				})
			}
		}
		if wantCheck.Message != "" {
			if got, _ := checkValue["message"].(string); got != wantCheck.Message {
				issues = append(issues, CallBehaviorParityIssue{
					Code:      "javascript.call_behavior.mismatch",
					SymbolKey: symbolKey,
					Path:      path,
					Field:     field,
					Message: fmt.Sprintf(
						"catalog %s policyChecks[%d].message = %q, want %q",
						strconv.Quote(path),
						i,
						got,
						wantCheck.Message,
					),
				})
			}
		}
	}
	return issues
}

func catalogStringFieldParityIssues(
	symbolKey string,
	path string,
	field string,
	symbol map[string]any,
	want string,
) []CallBehaviorParityIssue {
	if want == "" {
		return nil
	}
	got, _ := symbol[field].(string)
	if got != want {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s %s = %q, want %q", strconv.Quote(path), field, got, want),
		}}
	}
	return nil
}

func catalogCallbackParityIssues(
	symbolKey string,
	path string,
	symbol map[string]any,
	want *callbehavior.CallbackShape,
) []CallBehaviorParityIssue {
	field := "callback"
	if want == nil {
		if callbackValue, ok := symbol[field]; ok && callbackValue != nil {
			return []CallBehaviorParityIssue{{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s callback present, want none", strconv.Quote(path)),
			}}
		}
		return nil
	}

	callbackValue, ok := symbol[field].(map[string]any)
	if !ok {
		return []CallBehaviorParityIssue{{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s callback missing or not an object", strconv.Quote(path)),
		}}
	}

	var issues []CallBehaviorParityIssue
	if got, _ := callbackValue["role"].(string); got != want.Role {
		issues = append(issues, CallBehaviorParityIssue{
			Code:      "javascript.call_behavior.mismatch",
			SymbolKey: symbolKey,
			Path:      path,
			Field:     field,
			Message:   fmt.Sprintf("catalog %s callback.role = %q, want %q", strconv.Quote(path), got, want.Role),
		})
	}
	if want.Notes != "" {
		if got, _ := callbackValue["notes"].(string); got != want.Notes {
			issues = append(issues, CallBehaviorParityIssue{
				Code:      "javascript.call_behavior.mismatch",
				SymbolKey: symbolKey,
				Path:      path,
				Field:     field,
				Message:   fmt.Sprintf("catalog %s callback.notes = %q, want %q", strconv.Quote(path), got, want.Notes),
			})
		}
	}
	issues = append(issues, catalogParameterParityIssues(symbolKey, path+".callback", callbackValue, want.Parameters)...)
	return issues
}
