package workers

import "strings"

func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, pair := range env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}
