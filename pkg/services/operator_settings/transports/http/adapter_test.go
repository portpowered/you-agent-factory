package http

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestAdapter_BindsSettingsRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	configPath := "/home/operator/.you-agent-factory/config.json"
	scopeID := "local-00000000-0000-4000-8000-000000000010"
	var invoked bool
	fake := &rootFake{
		loadDocument: func(
			request operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			invoked = true
			if request.Path != configPath || !request.RequireExisting {
				t.Fatalf("LoadDocumentRequest = %#v, want path %q with RequireExisting", request, configPath)
			}
			return operatorsettings.LoadDocumentResult{
				Found: true,
				Document: operatorsettings.Document{
					BackendScopeID: scopeID,
					Defaults: operatorsettings.DocumentDefaults{
						WorkerModelProvider: "codex",
						WorkerModel:         "gpt-5",
					},
					Runtime: operatorsettings.EmptyDocument.Runtime,
				},
			}, nil
		},
	}

	adapter := NewAdapter(fake)

	result, err := adapter.invokeLoadDocument(context.Background(), operatorsettings.LoadDocumentRequest{
		Path:            configPath,
		RequireExisting: true,
	})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Settings root")
	}
	if err != nil {
		t.Fatalf("invokeLoadDocument error = %v", err)
	}
	if !result.Found || result.Document.BackendScopeID != scopeID {
		t.Fatalf("LoadDocumentResult = %#v, want found document for %q", result, scopeID)
	}
}

func TestAdapter_PropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.invokeLoadDocument(context.Background(), operatorsettings.LoadDocumentRequest{
		Path:            "/tmp/missing.json",
		RequireExisting: true,
	})
	if !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		t.Fatalf("invokeLoadDocument error = %v, want ErrDocumentNotFound", err)
	}
}

// adapterOperations exercises the three decoded-input boundaries with valid inputs.
func adapterOperations() map[string]func(*Adapter, context.Context) error {
	return map[string]func(*Adapter, context.Context) error{
		"load": func(a *Adapter, ctx context.Context) error {
			_, err := a.LoadDocument(ctx, LoadDocumentInput{Path: "/tmp/config.json"})
			return err
		},
		"update": func(a *Adapter, ctx context.Context) error {
			_, err := a.ApplyDocumentUpdate(ctx, ApplyDocumentUpdateInput{Path: "/tmp/config.json", Model: stringPointer("gpt-5")})
			return err
		},
		"resolve": func(a *Adapter, ctx context.Context) error {
			_, err := a.ResolveEffective(ctx, ResolveEffectiveInput{})
			return err
		},
	}
}

func failingRoot(err error) *rootFake {
	return &rootFake{
		loadDocument: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, err
		},
		applyDocumentUpdate: func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, err
		},
		resolveEffective: func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, err
		},
	}
}

func TestAdapter_DocumentFailureFacts(t *testing.T) {
	t.Parallel()
	for _, kind := range []operatorsettings.DocumentFailureKind{
		operatorsettings.DocumentFailureKindMalformed, operatorsettings.DocumentFailureKindUnsupported,
		operatorsettings.DocumentFailureKindConflict, operatorsettings.DocumentFailureKindNotFound,
	} {
		for _, wrapped := range []bool{false, true} {
			for name, invoke := range adapterOperations() {
				if name == "resolve" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/wrapped=%v", name, kind, wrapped), func(t *testing.T) {
					t.Parallel()
					failure := operatorsettings.DocumentFailure{Kind: kind, Path: "/private/config.json", Message: "owner details"}
					var ownerErr error = failure
					if wrapped {
						ownerErr = fmt.Errorf("owner: %w", failure)
					}
					err := invoke(NewAdapter(failingRoot(ownerErr)), nil)
					var got operatorsettings.DocumentFailure
					//nolint:errorlint // This boundary must return the exact owner error unchanged, including its wrapper.
					if err != ownerErr || !errors.Is(err, failure.Unwrap()) || !errors.As(err, &got) || got != failure {
						t.Fatalf("error = %#v, want unchanged owner error and facts %#v", err, failure)
					}
					status, response, handled := RootErrorResponse(err)
					wantStatus, wantResponse, _ := RootErrorResponse(failure.Unwrap())
					if !handled || status != wantStatus || response != wantResponse {
						t.Fatalf("mapped error = (%d, %#v, %v), want (%d, %#v, true)", status, response, handled, wantStatus, wantResponse)
					}
				})
			}
		}
	}
}

func TestAdapter_ResolutionFailureFacts(t *testing.T) {
	t.Parallel()
	for _, kind := range []operatorsettings.ResolutionFailureKind{
		operatorsettings.ResolutionFailureKindInvalidInput, operatorsettings.ResolutionFailureKindUnsupportedOverride,
		operatorsettings.ResolutionFailureKindConflict,
	} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wrapped=%v", kind, wrapped), func(t *testing.T) {
				t.Parallel()
				failure := operatorsettings.ResolutionFailure{Kind: kind, Field: "workerModel", Message: "owner details"}
				var ownerErr error = failure
				if wrapped {
					ownerErr = fmt.Errorf("owner: %w", failure)
				}
				err := adapterOperations()["resolve"](NewAdapter(failingRoot(ownerErr)), nil)
				var got operatorsettings.ResolutionFailure
				//nolint:errorlint // This boundary must return the exact owner error unchanged, including its wrapper.
				if err != ownerErr || !errors.Is(err, failure.Unwrap()) || !errors.As(err, &got) || got != failure {
					t.Fatalf("error = %#v, want unchanged owner error and facts %#v", err, failure)
				}
				status, response, handled := RootErrorResponse(err)
				wantStatus, wantResponse, _ := RootErrorResponse(failure.Unwrap())
				if !handled || status != wantStatus || response != wantResponse {
					t.Fatalf("mapped error = (%d, %#v, %v), want (%d, %#v, true)", status, response, handled, wantStatus, wantResponse)
				}
			})
		}
	}
}

func TestAdapter_InvalidInputPrecedesCanceledContext(t *testing.T) {
	t.Parallel()
	expected := operatorsettings.DocumentDefaults{WorkerModel: "different"}
	cases := []struct {
		name   string
		invoke func(*Adapter, context.Context) error
		want   error
	}{
		{"load path", func(a *Adapter, ctx context.Context) error {
			_, err := a.LoadDocument(ctx, LoadDocumentInput{Path: " "})
			return err
		}, ErrInvalidLoadPath},
		{"update path", func(a *Adapter, ctx context.Context) error {
			_, err := a.ApplyDocumentUpdate(ctx, ApplyDocumentUpdateInput{})
			return err
		}, ErrInvalidUpdatePath},
		{"update fields", func(a *Adapter, ctx context.Context) error {
			_, err := a.ApplyDocumentUpdate(ctx, ApplyDocumentUpdateInput{Path: "/tmp/config.json"})
			return err
		}, operatorsettings.ErrDocumentMalformed},
		{"resolve baseline", func(a *Adapter, ctx context.Context) error {
			_, err := a.ResolveEffective(ctx, ResolveEffectiveInput{ExpectedDocumentBaseline: &expected})
			return err
		}, operatorsettings.ErrResolutionConflict},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			fake := &rootFake{
				loadDocument: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
					calls.Add(1)
					return operatorsettings.LoadDocumentResult{}, nil
				},
				applyDocumentUpdate: func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
					calls.Add(1)
					return operatorsettings.ApplyDocumentUpdateResult{}, nil
				},
				resolveEffective: func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
					calls.Add(1)
					return operatorsettings.ResolveEffectiveResult{}, nil
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := test.invoke(NewAdapter(fake), ctx)
			if !errors.Is(err, test.want) || calls.Load() != 0 {
				t.Fatalf("error = %v, calls = %d; want %v without owner entry", err, calls.Load(), test.want)
			}
		})
	}
}

func TestAdapter_EmptyLoadResultPreservesRepresentation(t *testing.T) {
	t.Parallel()
	adapter := NewAdapter(failingRoot(nil))
	load, err := adapter.LoadDocument(context.Background(), LoadDocumentInput{Path: "/tmp/config.json"})
	if err != nil || load.Found || load.Path != "" || load.Document.Defaults != nil || load.Document.BackendScopeID != nil {
		t.Fatalf("load = %#v, %v; want empty metadata and absent optional document fields", load, err)
	}
	// Runtime remains represented, even for a zero document; no defaults are invented.
	if load.Document.Runtime == nil || load.Document.Runtime.Logging == nil ||
		*load.Document.Runtime.Logging.MaxSizeMB != 0 {
		t.Fatalf("empty runtime = %#v, want represented zero runtime", load.Document.Runtime)
	}
}

func TestAdapter_EmptyUpdateResultPreservesRepresentation(t *testing.T) {
	t.Parallel()
	adapter := NewAdapter(failingRoot(nil))
	update, err := adapter.ApplyDocumentUpdate(context.Background(), ApplyDocumentUpdateInput{Path: "/tmp/config.json", Provider: stringPointer("")})
	if err != nil || update.Persisted || update.Path != "" || update.Document.Defaults != nil {
		t.Fatalf("update = %#v, %v; want empty, non-persisted result", update, err)
	}
}

func TestAdapter_EmptyResolutionPreservesRepresentation(t *testing.T) {
	t.Parallel()
	adapter := NewAdapter(failingRoot(nil))
	resolve, err := adapter.ResolveEffective(context.Background(), ResolveEffectiveInput{})
	if err != nil || !reflect.DeepEqual(resolve.Selection, EffectiveSelectionResponse{}) {
		t.Fatalf("resolve = %#v, %v; want empty selection without source labels", resolve, err)
	}
}
