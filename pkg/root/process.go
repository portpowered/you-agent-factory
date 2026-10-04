package root

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// BuildProcess constructs the reusable application process. Production passes
// an empty edge set; functional tests replace only their external boundaries.
// The policy-free network transport is the production default for the pinned
// model protocol, while caller-provided edges remain authoritative.
func BuildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (*initializerapplication.Process, error) {
	if isNilConstructionEffect(ctx) {
		return nil, fmt.Errorf("build application process: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	if err := validateConstructionOverrides(edges); err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	clock, scheduler, err := normalizeProcessTime(edges.Clock, edges.ProcessScheduler)
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	edges.Clock, edges.ProcessScheduler = clock, scheduler
	applicationProcess, err := wire.InjectBundle(ctx, serviceedges.Merge(
		serviceedges.Edges{ModelInvocationGRPCDialer: platformgrpc.NetworkDialer{}},
		edges,
	), resolveACPWireLogSettings(os.Getenv))
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	return applicationProcess, nil
}

func resolveACPWireLogSettings(lookup func(string) string) wire.ACPWireLogSettings {
	return wire.ACPWireLogSettings{
		Disabled:  strings.EqualFold(strings.TrimSpace(lookup("YOU_ACP_WIRE_LOG")), "off"),
		Directory: strings.TrimSpace(lookup("YOU_ACP_WIRE_LOG_DIR")),
	}
}

// Validate raw overrides before selecting defaults or running any constructor.
// Nil top-level interfaces and functions mean omission. A populated interface
// must contain a usable effect, including when its dynamic value is a function.
// Data values (optional pointers, slices and maps) are not required effects.
func validateConstructionOverrides(edges serviceedges.Edges) error {
	values := reflect.ValueOf(edges)
	fields := values.Type()
	for index := 0; index < values.NumField(); index++ {
		value := values.Field(index)
		if value.Kind() != reflect.Interface || value.IsNil() {
			continue
		}
		if isNilConstructionEffect(value.Interface()) {
			return invalidConstructionOverride(fields.Field(index).Name)
		}
	}
	for _, registration := range edges.ProviderRegistrations {
		if isNilConstructionEffect(registration.Integration) {
			return fmt.Errorf("provider registry validation failed for %q: integration is required", registration.Manifest.ID)
		}
	}
	return nil
}

func invalidConstructionOverride(field string) error {
	// Keep diagnostics already exposed by the selected providers while moving
	// their admission ahead of graph construction. Other ports name the override.
	if field == "Clock" || field == "ProcessScheduler" {
		return fmt.Errorf("%s must not be typed-nil; omit it to select the default", field)
	}
	message := map[string]string{
		"PlatformProcessClock":          "platform process clock is required",
		"PlatformProcessCommandFactory": "platform process command factory is required",
		"ModelAssetHTTPClient":          "construct Models: asset HTTP client is required",
		"ModelHostProcessLauncher":      "construct Models: model host process launcher is required",
		"ModelHostHTTPClient":           "construct Models: model host HTTP client is required",
		"ModelHostClock":                "construct Models: model host clock is required",
		"ModelRuntimeCommandRunner":     "construct Models: model runtime command runner is required",
		"ModelRuntimeHTTPClient":        "construct Models: model runtime HTTP client is required",
	}[field]
	if message != "" {
		return fmt.Errorf("Edges.%s: %s", field, message)
	}
	return fmt.Errorf("Edges.%s is required when supplied", field)
}

// Classify presence without invoking caller methods or functions.
func isNilConstructionEffect(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// normalizeProcessTime selects process time once at the caller boundary.
// Legacy Now-only clocks control facts while a documented wall scheduler
// controls deadlines. Specialized owner overrides are left to their providers.
func normalizeProcessTime(
	clock platformclock.Source,
	scheduler platformclock.TimerSource,
) (platformclock.Source, platformclock.TimerSource, error) {
	if isTypedNilProcessTime(clock) {
		return nil, nil, invalidConstructionOverride("Clock")
	}
	if isTypedNilProcessTime(scheduler) {
		return nil, nil, invalidConstructionOverride("ProcessScheduler")
	}
	if clock == nil {
		clock = platformclock.Real{}
	}
	if scheduler == nil {
		if timerSource, ok := clock.(platformclock.TimerSource); ok {
			scheduler = timerSource
		} else {
			scheduler = platformclock.Real{}
		}
	}
	return clock, scheduler, nil
}

func isTypedNilProcessTime(value any) bool {
	return value != nil && isNilConstructionEffect(value)
}
