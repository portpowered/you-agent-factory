// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  defaultFactorySessionID,
  expectNoBrowserErrors,
  initialEditableFactoryDefinitionVersion,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
  waitForDurableCheckpoint,
} from "./browser-test-harness.mjs";
import {
  buildStreamIdentity,
  clearTimelineCheckpoints,
  defaultFactoryDefinition,
  eventStreamHasCursor,
  eventStreamOmitsCursor,
  installNetworkCapture,
  replayWorldStateWithProviderSessionRef,
  resolvedFactorySessionID,
  seedTimelineCheckpoint,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: manual recovery scenarios share one preview harness and IndexedDB helpers.
describe.sequential("dashboard session recovery manual scenarios", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "preserves a valid reconnect cursor across a backend restart when identity still matches",
    async () => {
      const identity = buildStreamIdentity();
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-restart-preserved-history",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await seedTimelineCheckpoint(browserPage.page, identity, {
          afterEventId: "manual-restart-event-5",
          afterSequence: 5,
          selectedTick: 5,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "restart cursor reuse",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "manual-restart-event-5"),
            );
          },
        );

        await network.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "second restart cursor reuse",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "manual-restart-event-5"),
            );
          },
        );

        const sessionReads = network.captured.factorySessionReads;
        expect(sessionReads.length).toBeGreaterThan(0);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "drops stream-derived state after a clean restart when stream generation changes",
    async () => {
      const currentIdentity = buildStreamIdentity();
      const staleIdentity = buildStreamIdentity({
        streamGenerationID: "2026-01-01T00:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-clean-restart",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleIdentity, {
          afterEventId: "stale-clean-restart-event-11",
          afterSequence: 11,
          selectedTick: 11,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "clean restart without stale cursor",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) =>
            eventStreamHasCursor(url, "stale-clean-restart-event-11"),
          ),
        ).toBe(false);
        expect(currentIdentity.streamGenerationID).not.toBe(
          staleIdentity.streamGenerationID,
        );
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "remaps ~default to a resolved factorySessionID without reusing an old cursor",
    async () => {
      const remappedIdentity = buildStreamIdentity({
        factorySessionID: defaultFactorySessionID,
        streamGenerationID: initialEditableFactoryDefinitionVersion.physical,
      });
      const staleRemapIdentity = buildStreamIdentity({
        factorySessionID: resolvedFactorySessionID,
        streamGenerationID: "2026-06-29T12:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-session-remap",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleRemapIdentity, {
          afterEventId: "remap-stale-event-3",
          afterSequence: 3,
          selectedTick: 3,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "remap reconnect without old cursor",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) => eventStreamHasCursor(url, "remap-stale-event-3")),
        ).toBe(false);
        expect(remappedIdentity.factorySessionID).not.toBe(
          staleRemapIdentity.factorySessionID,
        );
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "never sends a stale cursor after switching backend scope",
    async () => {
      const localScopeIdentity = buildStreamIdentity({
        backendScopeID: "/local/factory::browser-integration",
      });
      const cloudScopeIdentity = buildStreamIdentity({
        backendScopeID: "/cloud/factory::browser-integration",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: {
          ...defaultFactoryDefinition,
          sourceDirectory: "/cloud/factory",
        },
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-backend-scope-switch",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, localScopeIdentity, {
          afterEventId: "local-scope-event-8",
          afterSequence: 8,
          selectedTick: 8,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "backend scope switch reconnect",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) => eventStreamHasCursor(url, "local-scope-event-8")),
        ).toBe(false);
        expect(cloudScopeIdentity.backendScopeID).not.toBe(
          localScopeIdentity.backendScopeID,
        );
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "never sends a stale cursor or prior provider-session detail after switching provider account scope",
    async () => {
      const priorProviderScopeIdentity = buildStreamIdentity({
        backendScopeID:
          "/provider/local-account/factory::browser-integration",
      });
      const nextProviderScopeIdentity = buildStreamIdentity({
        backendScopeID:
          "/provider/remote-account/factory::browser-integration",
      });
      const priorProviderSessionRef =
        "provider-session/local-account/browser-integration";
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: {
          ...defaultFactoryDefinition,
          sourceDirectory: "/provider/remote-account/factory",
        },
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-provider-account-scope-switch",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(
          browserPage.page,
          priorProviderScopeIdentity,
          {
            afterEventId: "provider-scope-event-6",
            afterSequence: 6,
            selectedTick: 6,
            replayState: replayWorldStateWithProviderSessionRef(
              6,
              priorProviderSessionRef,
            ),
          },
        );
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "provider account scope switch reconnect",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        const syncPreflightReads = network.captured.syncPreflightReads;
        const allCapturedRequests = [
          ...urls,
          ...network.captured.factorySessionReads,
          ...syncPreflightReads,
        ];
        expect(
          urls.some((url) =>
            eventStreamHasCursor(url, "provider-scope-event-6"),
          ),
        ).toBe(false);
        expect(
          allCapturedRequests.some((url) =>
            url.includes(priorProviderSessionRef),
          ),
        ).toBe(false);
        expect(nextProviderScopeIdentity.backendScopeID).not.toBe(
          priorProviderScopeIdentity.backendScopeID,
        );
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "keeps tab-local reloads isolated while invalidating shared checkpoints on stream identity change",
    async () => {
      const identity = buildStreamIdentity();
      const staleIdentity = buildStreamIdentity({
        streamGenerationID: "2026-01-15T00:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-multi-tab-isolation",
      });

      try {
        const tabOneNetwork = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, identity, {
          afterEventId: "multi-tab-event-4",
          afterSequence: 4,
          selectedTick: 4,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "tab one cursor reuse",
          async () => {
            const urls = await tabOneNetwork.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "multi-tab-event-4"),
            );
          },
        );

        const tabTwoPage = await browserPage.context.newPage();
        const tabTwoNetwork = await installNetworkCapture(tabTwoPage);
        await tabTwoPage.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });

        await waitForDurableCheckpoint(
          "tab two shared checkpoint cursor reuse",
          async () => {
            const urls = await tabTwoNetwork.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "multi-tab-event-4"),
            );
          },
          uiInteractionTimeoutMs,
        );

        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleIdentity, {
          afterEventId: "multi-tab-stale-event-9",
          afterSequence: 9,
          selectedTick: 9,
        });

        await tabOneNetwork.resetEventStreamURLs();
        await tabTwoNetwork.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });
        await tabTwoPage.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "tab one stale identity reconnect",
          async () => {
            const urls = await tabOneNetwork.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );
        await waitForDurableCheckpoint(
          "tab two stale identity reconnect",
          async () => {
            const urls = await tabTwoNetwork.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const tabOneURLs = await tabOneNetwork.readEventStreamURLs();
        const tabTwoURLs = await tabTwoNetwork.readEventStreamURLs();
        expect(
          tabOneURLs.some((url) =>
            eventStreamHasCursor(url, "multi-tab-stale-event-9"),
          ),
        ).toBe(false);
        expect(
          tabTwoURLs.some((url) =>
            eventStreamHasCursor(url, "multi-tab-stale-event-9"),
          ),
        ).toBe(false);

        await tabTwoPage.close();
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
