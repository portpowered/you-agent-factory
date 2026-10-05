package process_time_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const webhookSecret = "opaque-signing-material-47921"

type webhookEffects struct {
	routes   map[string]*journeyRoute
	resolved chan string
	letters  chan []byte
}

func configureWebhookEffects(t *testing.T, c *timeCohort, edges *serviceedges.Edges, specialized bool) {
	t.Helper()
	c.webhook = c.process
	if specialized {
		c.webhook = newJourneyScheduler(c.wall.Now().Add(96 * time.Hour))
		edges.FactoryWebhookClock = c.webhook
	}
	effects := &webhookEffects{routes: map[string]*journeyRoute{}, resolved: make(chan string, 16), letters: make(chan []byte, 16)}
	c.webhookEffects = effects
	for _, key := range []string{"recover", "exhaust", "closing", "healthy", "badstatus", "secretfail", "storefail"} {
		effects.routes[key] = &journeyRoute{calls: make(chan journeyCall, 16)}
		config := idleTimeConfig()
		config["webhooks"] = []map[string]any{{"name": key, "enabled": true,
			"url":              "https://selected-webhook.invalid/" + key + "?token=" + webhookSecret,
			"signingSecretRef": key, "filter": map[string]any{"eventTypes": []string{"WORK_STATE_CHANGE"}},
			"deliveryPolicy": map[string]any{"maxAttempts": 3, "initialBackoff": "1s", "maxBackoff": "2s", "backoffMultiplier": 2.0, "requestTimeout": "30s"}}}
		c.dirs[key] = support.ScaffoldFactory(t, config)
		support.ClearSeedInputs(t, c.dirs[key])
	}
	edges.FactoryWebhookHTTPClient = journeyHTTP{routes: effects.routes}
	edges.FactoryWebhookSecretResolver = func(_ context.Context, _ factorydefinitions.LoadedFactorySource, ref string) (string, error) {
		effects.resolved <- ref
		if ref == "secretfail" {
			return "", fmt.Errorf("sensitive resolver failure: %s", webhookSecret)
		}
		return webhookSecret, nil
	}
	edges.FactoryWebhookDeadLetterAppender = func(_ string, line []byte) error {
		effects.letters <- append([]byte(nil), line...)
		var record struct {
			Endpoint string `json:"endpointName"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if record.Endpoint == "storefail" {
			return fmt.Errorf("sensitive storage failure: %s", webhookSecret)
		}
		return nil
	}
}

func openWebhookSession(t *testing.T, c *timeCohort, key string) string {
	t.Helper()
	// Webhook delivery requires an initial recorded runtime, rather than the
	// folder editor's unrecorded activation. Reuse this cohort's root process.
	id := uuid.NewString()
	dir := c.dirs[key]
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--session", id,
		"--record", filepath.Join(dir, "selected.recording.json"), "--continuously", "--quiet"})
	inputs.Env = c.env
	inputs.WorkingDirectory = dir
	command := support.StartProcessCommand(t, c.cli, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	select {
	case got := <-c.webhookEffects.resolved:
		if got != key {
			t.Fatalf("resolved %q, want %q", got, key)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("webhook secret activation not observed")
	}
	return id
}

func executeWebhookCommand(t *testing.T, c *timeCohort, id string, args ...string) []byte {
	t.Helper()
	command := append([]string{"you", "--server", c.url, "--json"}, args...)
	command = append(command, "--session", id)
	inputs := support.FakeInputs(t.Context(), command)
	inputs.Env = c.env
	if err := c.cli.Execute(inputs.Input); err != nil {
		t.Fatalf("webhook command: %v\n%s\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	return []byte(inputs.Stdout())
}

func emitWebhookWork(t *testing.T, c *timeCohort, id string) string {
	t.Helper()
	payload := filepath.Join(t.TempDir(), "work.md")
	if err := os.WriteFile(payload, []byte("selected webhook time witness"), 0600); err != nil {
		t.Fatal(err)
	}
	data := executeWebhookCommand(t, c, id, "submit", "--name", "selected-time", "--work-type-name", "story", "--payload", payload)
	var result struct {
		WorkID string `json:"workId"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.WorkID == "" {
		t.Fatalf("submit=%s, error=%v", data, err)
	}
	executeWebhookCommand(t, c, id, "work", "move", result.WorkID, "queued")
	return result.WorkID
}
