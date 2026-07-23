// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  defaultFactorySessionID,
  expectNoBrowserErrors,
  openBrowserPage,
  openDashboardWithSeededCheckpoint,
  resolvedDefaultFactorySessionID,
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
  seedTimelineCheckpoint,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";

async function tabReconnectWithoutStaleCursor(
  network,
  { defaultFactorySessionID, resolvedDefaultFactorySessionID },
) {
  const urls = await network.readEventStreamURLs();
  if (
    urls.some(
      (url) =>
        url.includes(
          `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
        ) && eventStreamOmitsCursor(url),
    )
  ) {
    return true;
  }
  return network.captured.syncPreflightReads.some(
    (url) =>
      url.includes(
        `/factory-sessions/${defaultFactorySessionID}/sync-preflight`,
      ) &&
      !url.includes("after_event_id") &&
      !url.includes("after_sequence"),
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: scope-switch scenarios share one preview harness and IndexedDB helpers.
describe.sequential("dashboard session recovery manual scope-switch scenarios", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "never sends a stale cursor or prior provider-session detail after switching provider account scope",
    async () => {
      const priorProviderScopeIdentity = buildStreamIdentity({
        backendScopeID: "/provider/local-account/factory::browser-integration",
      });
      const nextProviderScopeIdentity = buildStreamIdentity({
        backendScopeID: "/provider/remote-account/factory::browser-integration",
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
                  `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
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
        await openDashboardWithSeededCheckpoint(
          browserPage.page,
          preview.previewURL,
          async () => {
            await clearTimelineCheckpoints(browserPage.page);
            await seedTimelineCheckpoint(browserPage.page, identity, {
              afterEventId: "multi-tab-event-4",
              afterSequence: 4,
              selectedTick: 4,
            });
          },
        );

        await waitForDurableCheckpoint("tab one cursor reuse", async () => {
          const urls = await tabOneNetwork.readEventStreamURLs();
          return urls.some((url) =>
            eventStreamHasCursor(url, "multi-tab-event-4"),
          );
        });

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

        // Close tab two before mutating shared IndexedDB so its checkpoint persist
        // timer cannot rewrite the matching-identity envelope after we clear it.
        await tabTwoPage.close();

        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleIdentity, {
          afterEventId: "multi-tab-stale-event-9",
          afterSequence: 9,
          selectedTick: 9,
        });

        await tabOneNetwork.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        const staleReconnectTimeoutMs = 120_000;

        await waitForDurableCheckpoint(
          "tab one stale identity reconnect",
          async () =>
            tabReconnectWithoutStaleCursor(tabOneNetwork, {
              defaultFactorySessionID,
              resolvedDefaultFactorySessionID,
            }),
          staleReconnectTimeoutMs,
        );

        const tabTwoReloadPage = await browserPage.context.newPage();
        const tabTwoReloadNetwork =
          await installNetworkCapture(tabTwoReloadPage);
        await tabTwoReloadPage.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await waitForDurableCheckpoint(
          "tab two stale identity reconnect",
          async () =>
            tabReconnectWithoutStaleCursor(tabTwoReloadNetwork, {
              defaultFactorySessionID,
              resolvedDefaultFactorySessionID,
            }),
          staleReconnectTimeoutMs,
        );

        const tabOneURLs = await tabOneNetwork.readEventStreamURLs();
        const tabTwoURLs = await tabTwoReloadNetwork.readEventStreamURLs();
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

        await tabTwoReloadPage.close();
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
