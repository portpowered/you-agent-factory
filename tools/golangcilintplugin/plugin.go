// Package golangcilintplugin adapts shared analyzers to golangci's module API.
// Rule evaluation stays in internal/lint/analyzers; this package only binds
// invocation-local settings and requests the compiler's type information.
package golangcilintplugin

import (
	"flag"
	"fmt"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/portpowered/infinite-you/internal/lint/analyzers"
)

func init() {
	register.Plugin("repolint", New)
}

type settings struct {
	DeferStale []string `json:"defer-stale"`
}

type plugin struct {
	analyzers []*analysis.Analyzer
}

// New validates configuration without mutating the shared analyzer definitions.
func New(raw any) (register.LinterPlugin, error) {
	config, err := register.DecodeSettings[settings](raw)
	if err != nil {
		return nil, fmt.Errorf("repolint settings: %w", err)
	}
	deferred := map[string]bool{}
	for _, name := range config.DeferStale {
		if deferred[name] {
			return nil, fmt.Errorf("repolint: duplicate deferred analyzer %q", name)
		}
		deferred[name] = true
	}
	result := &plugin{}
	for _, original := range analyzers.All() {
		if original == analyzers.BaselineGrowth {
			original = analyzers.BaselineGrowthForDirectory(".", strings.Join(config.DeferStale, ","))
		}
		copy := *original
		copy.Flags = *flag.NewFlagSet(original.Name, flag.ContinueOnError)
		copy.Flags.Bool("check-stale", !deferred[original.Name], "reject stale exact debt entries")
		delete(deferred, original.Name)
		result.analyzers = append(result.analyzers, &copy)
	}
	for name := range deferred {
		return nil, fmt.Errorf("repolint: unknown deferred analyzer %q", name)
	}
	return result, nil
}

func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return p.analyzers, nil
}

func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
