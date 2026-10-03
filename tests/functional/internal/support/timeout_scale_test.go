package support

import "testing"

func TestResolveTimeoutScale(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		env  map[string]string
		want int
	}{
		"local default":     {nil, 1},
		"github actions":    {map[string]string{"GITHUB_ACTIONS": "true"}, ciTimeoutScale},
		"generic ci":        {map[string]string{"CI": "true"}, ciTimeoutScale},
		"explicit override": {map[string]string{TimeoutScaleEnv: "7", "CI": "true"}, 7},
		"explicit one":      {map[string]string{TimeoutScaleEnv: "1", "CI": "true"}, 1},
		"invalid ignored":   {map[string]string{TimeoutScaleEnv: "0"}, 1},
	} {
		env := tc.env
		if got := resolveTimeoutScale(func(key string) string { return env[key] }); got != tc.want {
			t.Errorf("%s: scale = %d, want %d", name, got, tc.want)
		}
	}
}
