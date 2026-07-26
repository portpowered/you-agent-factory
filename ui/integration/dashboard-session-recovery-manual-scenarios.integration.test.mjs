// @vitest-environment node

import { describe } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  initialEditableFactoryDefinitionVersion,
  openDashboardWithSeededCheckpoint,
  resolvedDefaultFactorySessionID,
  startFactoryApiServer,
  waitForDurableCheckpoint,
} from "./browser-test-harness.mjs";
import {
  buildStreamIdentity,
  clearTimelineCheckpoints,
  defaultFactoryDefinition,
  eventStreamHasCursor,
  eventStreamOmitsCursor,
  installNetworkCapture,
  resolvedFactorySessionID,
  seedTimelineCheckpoint,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: manual recovery scenarios share one preview harness and IndexedDB helpers.
describe.concurrent("dashboard session recovery manual scenarios", () => {
  it(
    "preserves a valid reconnect cursor across a backend restart when identity still matches",
    async ({ expect, openBrowserPage, preview }) => {
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
        await openDashboardWithSeededCheckpoint(
          browserPage.page,
          preview.previewURL,
          () =>
            seedTimelineCheckpoint(browserPage.page, identity, {
              afterEventId: "manual-restart-event-5",
              afterSequence: 5,
              selectedTick: 5,
            }),
        );

        await waitForDurableCheckpoint("restart cursor reuse", async () => {
          const urls = await network.readEventStreamURLs();
          return urls.some((url) =>
            eventStreamHasCursor(url, "manual-restart-event-5"),
          );
        });

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

        const syncPreflightReads = network.captured.syncPreflightReads;
        expect(syncPreflightReads.length).toBeGreaterThan(0);
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
    async ({ expect, openBrowserPage, preview }) => {
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
                  `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
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
    async ({ expect, openBrowserPage, preview }) => {
      const remappedIdentity = buildStreamIdentity({
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
                  `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) => eventStreamHasCursor(url, "remap-stale-event-3")),
        ).toBe(false);
        expect(remappedIdentity.streamGenerationID).not.toBe(
          staleRemapIdentity.streamGenerationID,
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
    async ({ expect, openBrowserPage, preview }) => {
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
                  `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
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
});
