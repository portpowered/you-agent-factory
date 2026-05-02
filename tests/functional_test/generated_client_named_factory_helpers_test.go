package functional_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	generatedclient "github.com/portpowered/infinite-you/pkg/generatedclient"
)

func createNamedFactory(t *testing.T, serverURL string, namedFactory factoryapi.NamedFactory) factoryapi.NamedFactory {
	t.Helper()

	client := newGeneratedFactoryClient(t, serverURL)
	request := convertNamedFactoryForGeneratedClient(t, namedFactory)
	resp, err := client.CreateFactoryWithResponse(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateFactoryWithResponse: %v", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("CreateFactoryWithResponse status = %d, want 201: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON201 == nil {
		t.Fatal("CreateFactoryWithResponse returned nil JSON201 payload")
	}

	return convertNamedFactoryFromGeneratedClient(t, *resp.JSON201)
}

func createNamedFactoryExpectBadRequest(t *testing.T, serverURL string, namedFactory factoryapi.NamedFactory) generatedclient.CreateFactoryBadRequest {
	t.Helper()

	client := newGeneratedFactoryClient(t, serverURL)
	request := convertNamedFactoryForGeneratedClient(t, namedFactory)
	resp, err := client.CreateFactoryWithResponse(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateFactoryWithResponse: %v", err)
	}
	if resp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("CreateFactoryWithResponse status = %d, want 400: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON400 == nil {
		t.Fatal("CreateFactoryWithResponse returned nil JSON400 payload")
	}

	return *resp.JSON400
}

func getCurrentNamedFactory(t *testing.T, serverURL string) factoryapi.NamedFactory {
	t.Helper()

	client := newGeneratedFactoryClient(t, serverURL)
	resp, err := client.GetCurrentFactoryWithResponse(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactoryWithResponse: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("GetCurrentFactoryWithResponse status = %d, want 200: %s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		t.Fatal("GetCurrentFactoryWithResponse returned nil JSON200 payload")
	}

	return convertNamedFactoryFromGeneratedClient(t, *resp.JSON200)
}

func newGeneratedFactoryClient(t *testing.T, serverURL string) generatedclient.ClientWithResponsesInterface {
	t.Helper()

	client, err := generatedclient.NewClientWithResponses(serverURL)
	if err != nil {
		t.Fatalf("NewClientWithResponses(%s): %v", serverURL, err)
	}
	return client
}

func convertNamedFactoryForGeneratedClient(t *testing.T, namedFactory factoryapi.NamedFactory) generatedclient.NamedFactory {
	t.Helper()

	return convertViaJSON[generatedclient.NamedFactory](t, namedFactory)
}

func convertNamedFactoryFromGeneratedClient(t *testing.T, namedFactory generatedclient.NamedFactory) factoryapi.NamedFactory {
	t.Helper()

	return convertViaJSON[factoryapi.NamedFactory](t, namedFactory)
}

func convertViaJSON[T any](t *testing.T, value any) T {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal generated client conversion payload: %v", err)
	}

	var converted T
	if err := json.Unmarshal(payload, &converted); err != nil {
		t.Fatalf("unmarshal generated client conversion payload: %v", err)
	}
	return converted
}
