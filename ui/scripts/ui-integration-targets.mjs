/** Focused browser-integration entry points for reuse outside the full lane. */

export const mockedBackendBrowserIntegrationFiles = [
  "integration/browser-test-harness.artifacts.integration.test.mjs",
  "integration/dashboard-session-recovery-manual-scenarios-switching.integration.test.mjs",
  "integration/dashboard-session-recovery-manual-scenarios.integration.test.mjs",
  "integration/dashboard-session-recovery.integration.test.mjs",
  "integration/dashboard-session-tabs.integration.test.mjs",
  "integration/dashboard-shared-indexeddb-browser-contexts.integration.test.mjs",
  "integration/event-stream-replay.integration.test.mjs",
  "integration/factory-graph-editor-node-placement.integration.test.mjs",
  "integration/factory-graph-editor-selection-no-panel-delete.integration.test.mjs",
  "integration/factory-graph-editor-session-switch.integration.test.mjs",
  "integration/factory-graph-editor.integration.test.mjs",
  "integration/factory-import-second-session.integration.test.mjs",
  "integration/hosted-exact-session-replay.integration.test.mjs",
  "integration/maintainer-phantom-worker-graph.integration.test.mjs",
  "integration/packaged-factories-hosted-route.integration.test.mjs",
];

export const mockedBackendBrowserIntegrationPhaseName =
  "Mocked-backend browser integration Vitest pass";

export const durableSessionRealBackendIntegrationFiles = [
  "integration/durable-session-real-backend.integration.test.mjs",
  "integration/browser-test-harness.real-backend-setup.integration.test.mjs",
];

export const durableSessionRealBackendIntegrationPhaseName =
  "Durable session real-backend browser integration Vitest pass";
