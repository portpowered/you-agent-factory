// @vitest-environment node

// biome-ignore lint/style/noExcessiveLinesPerFile: browser timeline acceptance scenarios share one realistic dashboard harness.
import { describe } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  loadReplayLines,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

const workerSessionID = "worker-session-wsr-007-001";
const streamGenerationID = "generation-wsr-007-001";
const providerSession = {
  id: "provider-session-wsr-007-001",
  kind: "session_id",
  provider: "codex",
};

const workerSessionObservation = {
  attemptId: "attempt-2",
  direct: false,
  durationBasis: "AUTHORITATIVE",
  durationMillis: 1_250,
  endedAt: "2026-08-13T09:00:04.000Z",
  parse: { errors: [], ignored: 0 },
  providerSessionAvailable: true,
  recordingHealth: "COMPLETE",
  startedAt: "2026-08-13T09:00:00.000Z",
  state: "COMPLETED",
  transcript: "AVAILABLE",
  turnId: "turn-wsr-007-001",
  workerSessionId: workerSessionID,
};

function workerSessionRecord(position, kind, phase, payload) {
  return {
    cursor: {
      position,
      streamGenerationId: streamGenerationID,
      workerSessionId: workerSessionID,
    },
    payload: {
      kind,
      payload,
      phase,
      provenance: {
        delivery: "NATIVE_STREAM",
        provider: "codex",
        representation: "SNAPSHOT",
      },
    },
    position,
    schemaId: "workers.draft.v1",
    sourceEventId: `worker-event-${position}`,
    sourceId: workerSessionID,
    sourceSequence: position,
    sourceType: "worker_session",
  };
}

function workerSessionFrame(
  delivery,
  event,
  workID,
  { recordingHealth = "COMPLETE", recordingHealthReason } = {},
) {
  return {
    delivery,
    errorCode: null,
    errorMessage: null,
    event,
    factorySessionId: "~default",
    providerSession,
    recordingHealth,
    ...(recordingHealthReason ? { recordingHealthReason } : {}),
    workerSessionId: workerSessionID,
    workIds: [workID],
  };
}

function workerSessionStreamBody(workID) {
  const records = [
    workerSessionFrame(
      "RECORD",
      workerSessionRecord(1, "SESSION", "STARTED", {
        attempt: 1,
        attemptId: "attempt-1",
        model: "gpt-5",
        providerSelection: {
          executorProvider: "ACP",
          modelProvider: "openai",
        },
        status: "STARTING",
        workerSessionId: workerSessionID,
      }),
      workID,
    ),
    workerSessionFrame(
      "RECORD",
      workerSessionRecord(2, "SESSION", "UPDATED", {
        attempt: 2,
        attemptId: "attempt-2",
        attemptReason: "RETRY",
        lineage: {
          previousAttemptId: "attempt-1",
          previousDispatchId: "dispatch-1",
        },
        status: "RETRYING",
      }),
      workID,
    ),
    // The retained-to-live handoff may overlap at the last acknowledged
    // position. The dashboard must ignore this exact canonical duplicate.
    workerSessionFrame(
      "RECORD",
      workerSessionRecord(2, "SESSION", "UPDATED", {
        attempt: 2,
        attemptId: "attempt-2",
        attemptReason: "RETRY",
        lineage: {
          previousAttemptId: "attempt-1",
          previousDispatchId: "dispatch-1",
        },
        status: "RETRYING",
      }),
      workID,
    ),
    workerSessionFrame(
      "RECORD",
      workerSessionRecord(3, "MESSAGE", "COMPLETED", {
        contentBlocks: [{ kind: "TEXT", text: "The retry completed." }],
        role: "assistant",
      }),
      workID,
    ),
    workerSessionFrame(
      "TERMINAL",
      workerSessionRecord(4, "SESSION", "COMPLETED", {
        continuation: {
          id: "provider-thread-wsr-007-001",
          kind: "session_id",
          provider: "codex",
        },
        lineage: {
          successorWorkerSessionId: "worker-session-wsr-007-002",
        },
        status: "COMPLETED",
      }),
      workID,
    ),
  ];

  return records.map((frame) => `data: ${JSON.stringify(frame)}\n\n`).join("");
}

function workerSessionSourceFailureBody(workID) {
  return `data: ${JSON.stringify({
    delivery: "SOURCE_FAILURE",
    errorCode: "WORKER_SESSION_STREAM_UNAVAILABLE",
    errorMessage: "retained Worker Session history unavailable",
    event: null,
    factorySessionId: "~default",
    providerSession,
    recordingHealth: "INCOMPLETE",
    recordingHealthReason: "RETAINED_HEAD_MOVED",
    workerSessionId: workerSessionID,
    workIds: [workID],
  })}\n\n`;
}

function workerSessionFailedStreamBody(workID) {
  const records = [
    workerSessionFrame(
      "TERMINAL",
      workerSessionRecord(1, "SESSION", "FAILED", {
        attempt: 1,
        attemptId: "attempt-failed-1",
        failureDetail: "Provider stopped responding.",
        status: "FAILED",
      }),
      workID,
      {
        recordingHealth: "INCOMPLETE",
        recordingHealthReason: "RETAINED_HEAD_MOVED",
      },
    ),
  ];
  return records.map((frame) => `data: ${JSON.stringify(frame)}\n\n`).join("");
}

async function installWorkerSessionRoutes(
  page,
  { streamBodyForRequest = (workID) => workerSessionStreamBody(workID) } = {},
) {
  const requests = {
    observations: [],
    events: [],
  };

  await page.route(
    "**/factory-sessions/**/worker-sessions**",
    async (route) => {
      const request = route.request();
      if (request.method() !== "GET") {
        await route.continue();
        return;
      }

      const url = new URL(request.url());
      if (url.pathname.endsWith("/events")) {
        requests.events.push(url.toString());
        const workID = url.searchParams.get("workId") ?? "work-wsr-007-001";
        await route.fulfill({
          body: streamBodyForRequest(workID, requests.events.length),
          contentType: "text/event-stream",
          headers: {
            "Access-Control-Allow-Origin": "*",
            "Cache-Control": "no-cache, no-transform",
            Connection: "keep-alive",
          },
          status: 200,
        });
        return;
      }

      if (url.pathname.endsWith("/worker-sessions")) {
        const workID = url.searchParams.get("workId") ?? "work-wsr-007-001";
        requests.observations.push(url.toString());
        await route.fulfill({
          body: JSON.stringify({
            sessions: [{ ...workerSessionObservation, workIds: [workID] }],
          }),
          contentType: "application/json",
          headers: { "Access-Control-Allow-Origin": "*" },
          status: 200,
        });
        return;
      }

      await route.continue();
    },
  );

  return requests;
}

async function selectReplayWork(page) {
  const workstationButton = page.getByRole("button", {
    name: /^Select plan workstation$/i,
  });
  await workstationButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await workstationButton.scrollIntoViewIfNeeded();
  await workstationButton.click({ force: true });

  await page.getByRole("article", { name: "Current selection" }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  const workItemButton = page
    .getByRole("button", { name: /^Select work item / })
    .first();
  await workItemButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await workItemButton.click({ force: true });
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: browser acceptance scenarios keep their dashboard setup and assertions together.
describe("Worker Session timeline browser integration", () => {
  it(
    "WSR-UI-001 renders retained and live canonical records once, preserves retry and continuation identity, and settles on the terminal outcome",
    async ({ expect, openBrowserPage, preview }) => {
      const replayServer = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: { name: "Worker Session Timeline Browser Factory" },
        eventLines: await loadReplayLines("event-stream-replay.jsonl"),
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "worker-session-timeline-wsr-ui-001",
      });

      try {
        const requests = await installWorkerSessionRoutes(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ timeout: uiInteractionTimeoutMs });
        await selectReplayWork(browserPage.page);

        const timelineCard = browserPage.page.locator(
          '[data-bento-card-id="worker-session-timeline"]',
        );
        await timelineCard
          .getByRole("heading", {
            name: "Worker Session timeline",
            exact: true,
          })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
        const timeline = timelineCard.locator(
          'section[aria-label="Worker Session timeline"]',
        );
        await timeline.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            () => timeline.getAttribute("data-worker-session-timeline-status"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("completed");

        expect(requests.observations.length).toBeGreaterThan(0);
        expect(requests.events.length).toBeGreaterThan(0);
        const rows = timeline.locator(
          "li[data-worker-session-timeline-entry-position]",
        );
        await rows.first().waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        expect(await rows.count()).toBe(4);
        expect(
          await rows.evaluateAll((elements) =>
            elements.map((element) =>
              element.getAttribute(
                "data-worker-session-timeline-entry-position",
              ),
            ),
          ),
        ).toEqual(["1", "2", "3", "4"]);
        expect(
          await timeline
            .locator('[data-worker-session-terminal-outcome="SUCCESS"]')
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline.getByText("openai", { exact: true }).count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline.getByText("gpt-5", { exact: true }).count(),
        ).toBeGreaterThan(0);
        const detailButtons = timeline.locator("button[aria-expanded]");
        for (let index = 0; index < (await detailButtons.count()); index += 1) {
          await detailButtons.nth(index).click();
        }
        expect(
          await timeline
            .getByText("The retry completed.", { exact: true })
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline
            .getByText("worker-session-wsr-007-002", { exact: true })
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline
            .getByText("codex / session_id / provider-thread-wsr-007-001", {
              exact: true,
            })
            .count(),
        ).toBeGreaterThan(0);

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await replayServer.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "keeps an incomplete recording notice separate from a failed terminal outcome and recovers through Retry",
    async ({ expect, openBrowserPage, preview }) => {
      const replayServer = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: { name: "Worker Session Timeline Failure Factory" },
        eventLines: await loadReplayLines("event-stream-replay.jsonl"),
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "worker-session-timeline-source-failure",
      });

      try {
        const requests = await installWorkerSessionRoutes(browserPage.page, {
          streamBodyForRequest: (workID, requestNumber) =>
            requestNumber === 1
              ? workerSessionSourceFailureBody(workID)
              : workerSessionFailedStreamBody(workID),
        });
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ timeout: uiInteractionTimeoutMs });
        await selectReplayWork(browserPage.page);

        const timeline = browserPage.page
          .locator('[data-bento-card-id="worker-session-timeline"]')
          .locator('section[aria-label="Worker Session timeline"]');
        await timeline.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            () => timeline.getAttribute("data-worker-session-timeline-status"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("source-error");
        expect(
          await timeline
            .getByText("retained Worker Session history unavailable", {
              exact: true,
            })
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline
            .getByText("Recording: INCOMPLETE", { exact: true })
            .count(),
        ).toBeGreaterThan(0);
        await timeline.getByRole("button", { name: "Retry" }).click();

        await expect
          .poll(() => requests.events.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(2);
        await expect
          .poll(
            () => timeline.getAttribute("data-worker-session-timeline-status"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("completed");
        expect(
          await timeline
            .locator('[data-worker-session-terminal-outcome="FAILURE"]')
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline
            .getByText("Provider stopped responding.", { exact: true })
            .count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline.getByText(/RETAINED_HEAD_MOVED/).count(),
        ).toBeGreaterThan(0);
        expect(
          await timeline
            .getByRole("heading", {
              name: "Worker Session records could not be loaded",
            })
            .count(),
        ).toBe(0);

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await replayServer.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
