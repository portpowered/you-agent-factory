package runtime

import "github.com/portpowered/infinite-you/pkg/services/workers"

// runtimePromptTokens is the prompt-facing projection of a dispatch. Resource
// tokens reserve capacity in Runtime but are not detached Work inputs, so they
// must not change positional .Inputs references in the provider prompt.
func runtimePromptTokens(tokens []workers.Token) []workers.Token {
	if len(tokens) == 0 {
		return nil
	}
	filtered := make([]workers.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}

// runtimePromptContext gives the prompt renderer the same resolved execution
// fields that the detached Workers target will use. It clones the caller's
// context so prompt rendering cannot mutate request-owned state.
func runtimePromptContext(
	base *workers.Context,
	selection *runtimeExecutionSelection,
) *workers.Context {
	context := base.Clone()
	if context == nil {
		context = &workers.Context{}
	}
	if selection == nil {
		return context
	}
	if selection.workingDirectory != "" {
		context.WorkDirectory = selection.workingDirectory
	}
	if len(selection.environment) > 0 {
		if context.EnvVars == nil {
			context.EnvVars = make(map[string]string, len(selection.environment))
		}
		for key, value := range selection.environment {
			context.EnvVars[key] = value
		}
	}
	return context
}
