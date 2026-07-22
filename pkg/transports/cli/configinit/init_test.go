package configinitcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

type fakeSystemInitializationService struct {
	initialize func(context.Context, systeminitialization.Request) (systeminitialization.Result, error)
}

func (fake fakeSystemInitializationService) Initialize(
	ctx context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	return fake.initialize(ctx, request)
}

func TestInitializerPropagatesContextAndRequestAndRendersText(t *testing.T) {
	t.Parallel()

	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "invocation")
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	calls := 0
	initialize := NewInitializer(fakeSystemInitializationService{
		initialize: func(
			gotContext context.Context,
			request systeminitialization.Request,
		) (systeminitialization.Result, error) {
			calls++
			if gotContext != ctx {
				t.Fatal("Initialize context was not propagated unchanged")
			}
			if request.HomeDir != "/tmp/operator" {
				t.Fatalf("request.HomeDir = %q, want trimmed home", request.HomeDir)
			}
			return systeminitialization.Result{
				HomeDir:             request.HomeDir,
				ConfigPath:          "/tmp/operator/config.json",
				NamedFactoriesRoot:  "/tmp/operator/factories",
				SystemConfigOutcome: systeminitialization.SystemConfigCreated,
				PackagedFactories: []systeminitialization.PackagedFactoryResult{
					{
						Name:       "@you/goal",
						FactoryDir: "/tmp/operator/factories/@you/goal",
						Outcome:    systeminitialization.PackagedFactoryCreated,
					},
					{
						Name:       "@you/subagent",
						FactoryDir: "/tmp/operator/factories/@you/subagent",
						Outcome:    systeminitialization.PackagedFactorySkipped,
					},
				},
			}, nil
		},
	})

	err := initialize(InitConfig{
		Context:     ctx,
		HomeDir:     " /tmp/operator ",
		Output:      &output,
		Diagnostics: &diagnostics,
		Verbose:     true,
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Initialize calls = %d, want 1", calls)
	}
	for _, expected := range []string{
		"Created system config at /tmp/operator/config.json",
		"Created packaged factory @you/goal at /tmp/operator/factories/@you/goal",
		"Packaged factory @you/subagent already present at /tmp/operator/factories/@you/subagent",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output = %q, want %q", output.String(), expected)
		}
	}
	if !strings.Contains(diagnostics.String(), "config init request homeDir=/tmp/operator") ||
		!strings.Contains(diagnostics.String(), "config init complete") {
		t.Fatalf("diagnostics = %q, want request and completion", diagnostics.String())
	}
}

func TestInitializerRendersJSONResult(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	initialize := NewInitializer(fakeSystemInitializationService{
		initialize: func(
			_ context.Context,
			request systeminitialization.Request,
		) (systeminitialization.Result, error) {
			return systeminitialization.Result{
				HomeDir:             request.HomeDir,
				ConfigPath:          "/home/operator/config.json",
				NamedFactoriesRoot:  "/home/operator/factories",
				SystemConfigOutcome: systeminitialization.SystemConfigSkipped,
				PackagedFactories: []systeminitialization.PackagedFactoryResult{{
					Name:       "@you/goal",
					FactoryDir: "/home/operator/factories/@you/goal",
					Outcome:    systeminitialization.PackagedFactorySkipped,
				}},
			}, nil
		},
	})

	err := initialize(InitConfig{
		Context: t.Context(),
		HomeDir: "/home/operator",
		JSON:    true,
		Output:  &output,
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	var result InitResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.HomeDir != "/home/operator" ||
		result.SystemConfigOutcome != string(systeminitialization.SystemConfigSkipped) ||
		len(result.PackagedFactories) != 1 ||
		result.PackagedFactories[0].Outcome != string(systeminitialization.PackagedFactorySkipped) {
		t.Fatalf("result = %#v", result)
	}
}

func TestInitializerRejectsMissingEdgesBeforeCallingService(t *testing.T) {
	calls := 0
	initialize := NewInitializer(fakeSystemInitializationService{
		initialize: func(
			context.Context,
			systeminitialization.Request,
		) (systeminitialization.Result, error) {
			calls++
			return systeminitialization.Result{}, nil
		},
	})

	tests := []struct {
		name string
		cfg  InitConfig
		want string
	}{
		{
			name: "context",
			cfg:  InitConfig{HomeDir: "/home/operator", Output: &bytes.Buffer{}},
			want: "context is required",
		},
		{
			name: "output",
			cfg:  InitConfig{Context: t.Context(), HomeDir: "/home/operator"},
			want: "output is required",
		},
		{
			name: "home",
			cfg:  InitConfig{Context: t.Context(), HomeDir: "  ", Output: &bytes.Buffer{}},
			want: "home directory is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := initialize(test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("Initialize calls = %d, want 0", calls)
	}
}

func TestInitializerRejectsMissingService(t *testing.T) {
	t.Parallel()

	err := NewInitializer(nil)(InitConfig{
		Context: t.Context(),
		HomeDir: "/home/operator",
		Output:  &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "service is required") {
		t.Fatalf("error = %v, want required service", err)
	}
}

func TestInitializerPropagatesServiceFailure(t *testing.T) {
	t.Parallel()

	expected := errors.New("initialize failed")
	err := NewInitializer(fakeSystemInitializationService{
		initialize: func(
			context.Context,
			systeminitialization.Request,
		) (systeminitialization.Result, error) {
			return systeminitialization.Result{}, expected
		},
	})(InitConfig{
		Context: t.Context(),
		HomeDir: "/home/operator",
		Output:  &bytes.Buffer{},
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
}

func TestInitializerRejectsUnknownSystemConfigOutcome(t *testing.T) {
	t.Parallel()

	for _, jsonOutput := range []bool{false, true} {
		jsonOutput := jsonOutput
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := NewInitializer(fakeSystemInitializationService{
				initialize: func(
					context.Context,
					systeminitialization.Request,
				) (systeminitialization.Result, error) {
					return systeminitialization.Result{
						SystemConfigOutcome: systeminitialization.SystemConfigOutcome("unexpected"),
					}, nil
				},
			})(InitConfig{
				Context: t.Context(),
				HomeDir: "/home/operator",
				JSON:    jsonOutput,
				Output:  &output,
			})
			if err == nil || !strings.Contains(err.Error(), `unknown system config outcome "unexpected"`) {
				t.Fatalf("error = %v, want unknown system config outcome", err)
			}
			if output.Len() != 0 {
				t.Fatalf("output = %q, want no output", output.String())
			}
		})
	}
}

func TestInitializerRejectsUnknownPackagedFactoryOutcome(t *testing.T) {
	t.Parallel()

	for _, jsonOutput := range []bool{false, true} {
		jsonOutput := jsonOutput
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := NewInitializer(fakeSystemInitializationService{
				initialize: func(
					context.Context,
					systeminitialization.Request,
				) (systeminitialization.Result, error) {
					return systeminitialization.Result{
						SystemConfigOutcome: systeminitialization.SystemConfigCreated,
						PackagedFactories: []systeminitialization.PackagedFactoryResult{
							{
								Name:    "@you/unknown",
								Outcome: systeminitialization.PackagedFactoryOutcome("unexpected"),
							},
						},
					}, nil
				},
			})(InitConfig{
				Context: t.Context(),
				HomeDir: "/home/operator",
				JSON:    jsonOutput,
				Output:  &output,
			})
			if err == nil || !strings.Contains(err.Error(), `unknown packaged factory outcome "unexpected" for "@you/unknown"`) {
				t.Fatalf("error = %v, want unknown packaged factory outcome", err)
			}
			if output.Len() != 0 {
				t.Fatalf("output = %q, want no output", output.String())
			}
		})
	}
}
