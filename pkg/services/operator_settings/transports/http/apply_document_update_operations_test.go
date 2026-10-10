package http

import (
	"context"
	"errors"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestAdapter_ApplyDocumentUpdateInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	configPath := "/home/operator/.you-agent-factory/config.json"
	scopeID := "local-00000000-0000-4000-8000-000000000010"
	nextModel := "gpt-5.2"
	provider := " codex "
	var invoked bool
	fake := &rootFake{
		applyDocumentUpdate: func(
			request operatorsettings.ApplyDocumentUpdateRequest,
		) (operatorsettings.ApplyDocumentUpdateResult, error) {
			invoked = true
			if request.Path != configPath || request.ExpectedBackendScope != scopeID {
				t.Fatalf(
					"ApplyDocumentUpdateRequest = %#v, want path %q with scope %q",
					request,
					configPath,
					scopeID,
				)
			}
			if request.ProviderModel.Provider == nil || *request.ProviderModel.Provider != "codex" {
				t.Errorf("provider = %#v, want codex", request.ProviderModel.Provider)
			}
			if request.ProviderModel.Model == nil || *request.ProviderModel.Model != nextModel {
				t.Fatalf("ApplyDocumentUpdateRequest.ProviderModel = %#v, want model %q", request.ProviderModel, nextModel)
			}
			return operatorsettings.ApplyDocumentUpdateResult{
				Path:      configPath,
				Persisted: true,
				Document: operatorsettings.Document{
					BackendScopeID: scopeID,
					Defaults: operatorsettings.DocumentDefaults{
						WorkerModelProvider: "codex",
						WorkerModel:         nextModel,
					},
					Runtime: operatorsettings.EmptyDocument.Runtime,
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.ApplyDocumentUpdate(context.Background(), ApplyDocumentUpdateInput{
		Path:                 " " + configPath + " ",
		ExpectedBackendScope: " " + scopeID + " ",
		Provider:             &provider,
		Model:                &nextModel,
	})
	if !invoked {
		t.Fatal("ApplyDocumentUpdate did not invoke the injected Settings root")
	}
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate error = %v", err)
	}
	if response.Path != configPath || !response.Persisted {
		t.Fatalf("response metadata = path=%q persisted=%v, want persisted update at %q", response.Path, response.Persisted, configPath)
	}
	if response.Document.BackendScopeID == nil || *response.Document.BackendScopeID != scopeID {
		t.Fatalf("response.Document.BackendScopeID = %#v, want %q", response.Document.BackendScopeID, scopeID)
	}
	if response.Document.Defaults == nil ||
		response.Document.Defaults.WorkerModel == nil ||
		*response.Document.Defaults.WorkerModel != nextModel {
		t.Fatalf("response.Document.Defaults = %#v, want updated model %q", response.Document.Defaults, nextModel)
	}
}

func TestAdapter_ApplyDocumentUpdateRejectsInvalidInputBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		applyDocumentUpdate: func(
			operatorsettings.ApplyDocumentUpdateRequest,
		) (operatorsettings.ApplyDocumentUpdateResult, error) {
			t.Fatal("fake root must not be invoked for invalid update input")
			return operatorsettings.ApplyDocumentUpdateResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.ApplyDocumentUpdate(context.Background(), ApplyDocumentUpdateInput{Model: stringPointer("gpt-5")})
	if err == nil || !IsApplyDocumentUpdateBadRequest(err) {
		t.Fatalf("ApplyDocumentUpdate error = %v, want typed bad request", err)
	}
}

func TestAdapter_ApplyDocumentUpdatePropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "malformed", err: operatorsettings.ErrDocumentMalformed},
		{name: "unsupported", err: operatorsettings.ErrDocumentUnsupported},
		{name: "conflict", err: operatorsettings.ErrDocumentConflict},
		{name: "not_found", err: operatorsettings.ErrDocumentNotFound},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &rootFake{
				applyDocumentUpdate: func(
					operatorsettings.ApplyDocumentUpdateRequest,
				) (operatorsettings.ApplyDocumentUpdateResult, error) {
					return operatorsettings.ApplyDocumentUpdateResult{}, test.err
				},
			}
			adapter := NewAdapter(fake)
			model := "gpt-5"

			_, err := adapter.ApplyDocumentUpdate(context.Background(), ApplyDocumentUpdateInput{
				Path:  "/tmp/config.json",
				Model: &model,
			})
			if err == nil || !errors.Is(err, test.err) {
				t.Fatalf("ApplyDocumentUpdate error = %v, want %v", err, test.err)
			}
		})
	}
}

func TestAdapter_ApplyDocumentUpdateForwardsOptionalFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                    string
		provider, model         *string
		wantProvider, wantModel *string
	}{
		{"provider only", stringPointer(" codex "), nil, stringPointer("codex"), nil},
		{"model only", nil, stringPointer(" gpt-5 "), nil, stringPointer("gpt-5")},
		{"explicit clear", stringPointer(" "), stringPointer(" "), stringPointer(""), stringPointer("")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requests := make(chan operatorsettings.ApplyDocumentUpdateRequest, 1)
			adapter := NewAdapter(&rootFake{
				applyDocumentUpdate: func(request operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
					requests <- request
					return operatorsettings.ApplyDocumentUpdateResult{}, nil
				},
			})
			_, err := adapter.ApplyDocumentUpdate(context.Background(), ApplyDocumentUpdateInput{
				Path: " /tmp/config.json ", ExpectedBackendScope: " scope ",
				Provider: test.provider, Model: test.model,
			})
			if err != nil {
				t.Fatal(err)
			}
			want := operatorsettings.ApplyDocumentUpdateRequest{
				Path: "/tmp/config.json", ExpectedBackendScope: "scope",
				ProviderModel: operatorsettings.DocumentProviderModelUpdate{Provider: test.wantProvider, Model: test.wantModel},
			}
			if got := <-requests; !reflect.DeepEqual(got, want) {
				t.Fatalf("owner request = %#v, want %#v", got, want)
			}
		})
	}
}
