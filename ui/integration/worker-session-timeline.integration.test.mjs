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

function workerSessionLongHistoryBody(workID, startPosition, endPosition) {
  const records = [];
  for (let position = startPosition; position <= endPosition; position += 1) {
    const cycle = position % 5;
    if (position === 1) {
      records.push(
        workerSessionFrame(
          "RECORD",
          workerSessionRecord(position, "SESSION", "STARTED", {
            attempt: 1,
            attemptId: "attempt-long-history",
            model: "gpt-5",
            providerSelection: {
              executorProvider: "ACP",
              modelProvider: "openai",
            },
            status: "RUNNING",
            workerSessionId: workerSessionID,
          }),
          workID,
        ),
      );
      continue;
    }

    const kind =
      cycle === 0
        ? "MESSAGE"
        : cycle === 1
          ? "REASONING"
          : cycle === 2
            ? "TOOL"
            : cycle === 3
              ? "USAGE"
              : "PROGRESS";
    const payload =
      kind === "MESSAGE"
        ? {
            contentBlocks: [
              { kind: "TEXT", text: `History message ${position}` },
            ],
            role: position % 2 === 0 ? "assistant" : "user",
          }
        : kind === "REASONING"
          ? { summaryDelta: `Reasoning update ${position}` }
          : kind === "TOOL"
            ? {
                resultSummary: {
                  exitCode: 0,
                  output: `Tool output ${position}`,
                },
                status: "completed",
                toolCallId: `tool-${position}`,
                toolName: "command_execution",
              }
            : kind === "USAGE"
              ? {
                  inputTokens: position,
                  model: "gpt-5",
                  outputTokens: position,
                  totalTokens: position * 2,
                }
              : { status: "RUNNING" };
    records.push(
      workerSessionFrame(
        "RECORD",
        workerSessionRecord(position, kind, "UPDATED", payload),
        workID,
      ),
    );
  }

  return records.map((frame) => `data: ${JSON.stringify(frame)}\n\n`).join("");
}

function workerSessionTerminalBody(workID, position) {
  return `data: ${JSON.stringify(
    workerSessionFrame(
      "TERMINAL",
      workerSessionRecord(position, "SESSION", "COMPLETED", {
        status: "COMPLETED",
      }),
      workID,
    ),
  )}\n\n`;
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
  {
    beforeStreamResponse = null,
    streamBodyForRequest = (workID) => workerSessionStreamBody(workID),
  } = {},
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
        const requestNumber = requests.events.length;
        await beforeStreamResponse?.(workID, requestNumber);
        await route.fulfill({
          body: await streamBodyForRequest(workID, requestNumber),
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

async function selectReplayWork(page, expect) {
  // Replay hydration can render the Work picker after the shared interaction
  // timeout under CI load; keep this recovery local to the replay scenario.
  const replaySelectionTimeoutMs = 30_000;
  const workstationButton = page.getByRole("button", {
    name: /^Select plan workstation$/i,
  });
  await workstationButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await workstationButton.focus();
  await workstationButton.press("Enter");

  await page.getByRole("article", { name: "Current selection" }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  const workItemButton = page
    .getByRole("button", { name: /^Select work item / })
    .first();
  await expect
    .poll(() => workItemButton.count(), {
      timeout: replaySelectionTimeoutMs,
    })
    .toBeGreaterThan(0);
  await workItemButton.waitFor({
    state: "visible",
    timeout: replaySelectionTimeoutMs,
  });
  await workItemButton.focus();
  await workItemButton.press("Enter");
}

async function waitForTimelinePosition(expect, rows, position) {
  await expect
    .poll(
      () =>
        rows
          .first()
          .getAttribute("data-worker-session-timeline-entry-position"),
      { timeout: uiInteractionTimeoutMs },
    )
    .toBe(String(position));
}

async function assertNoPageOverflow(page, expect) {
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

async function assertInitialLongHistoryWindow(expect, page, timeline, rows) {
  await expect
    .poll(() => rows.count(), { timeout: uiInteractionTimeoutMs })
    .toBe(200);
  expect(
    await rows
      .first()
      .getAttribute("data-worker-session-timeline-entry-position"),
  ).toBe("2");
  expect(
    await rows
      .last()
      .getAttribute("data-worker-session-timeline-entry-position"),
  ).toBe("201");
  expect(
    await timeline
      .locator("ol[data-worker-session-timeline-events='true']")
      .getAttribute("data-worker-session-timeline-visible-count"),
  ).toBe("200");
  await assertNoPageOverflow(page, expect);
}

async function navigateTimelineWindows(expect, page, timeline, rows) {
  const earlierEventsButton = timeline.getByRole("button", {
    name: "Earlier events",
    exact: true,
  });
  await earlierEventsButton.focus();
  await page.keyboard.press("Enter");
  await waitForTimelinePosition(expect, rows, 1);

  const laterEventsButton = timeline.getByRole("button", {
    name: "Later events",
    exact: true,
  });
  await laterEventsButton.focus();
  await page.keyboard.press("Enter");
  await waitForTimelinePosition(expect, rows, 2);

  await earlierEventsButton.focus();
  await page.keyboard.press("Enter");
  await waitForTimelinePosition(expect, rows, 1);

  const focusedDetailsButton = rows
    .first()
    .getByRole("button", { name: "Show details", exact: true });
  await focusedDetailsButton.scrollIntoViewIfNeeded();
  await focusedDetailsButton.focus();
  expect(
    await focusedDetailsButton.evaluate(
      (element) => element === document.activeElement,
    ),
  ).toBe(true);
  return focusedDetailsButton;
}

async function assertPendingLiveActivity(
  expect,
  page,
  timeline,
  rows,
  requests,
  releaseLiveStream,
  focusedDetailsButton,
) {
  await expect
    .poll(() => requests.events.length, { timeout: uiInteractionTimeoutMs })
    .toBeGreaterThanOrEqual(2);
  releaseLiveStream();

  const newActivityButton = timeline.getByRole("button", {
    name: "View 1 new event",
    exact: true,
  });
  await newActivityButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  expect(
    await focusedDetailsButton.evaluate(
      (element) => element === document.activeElement,
    ),
  ).toBe(true);
  expect(await rows.count()).toBe(200);

  await newActivityButton.focus();
  await page.keyboard.press("Enter");
  await waitForTimelinePosition(expect, rows, 3);
}

async function assertCompletedTimelineResponsive(expect, page, timeline, rows) {
  await expect
    .poll(() => timeline.getAttribute("data-worker-session-timeline-status"), {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe("completed");
  expect(
    await timeline
      .locator('[data-worker-session-terminal-outcome="SUCCESS"]')
      .count(),
  ).toBeGreaterThan(0);
  expect(await rows.count()).toBe(200);
  expect(
    await rows
      .first()
      .getAttribute("data-worker-session-timeline-entry-position"),
  ).toBe("4");
  expect(
    await rows
      .last()
      .getAttribute("data-worker-session-timeline-entry-position"),
  ).toBe("203");

  await page.setViewportSize({ height: 844, width: 390 });
  await assertNoPageOverflow(page, expect);
  expect(
    await timeline.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  expect(await rows.count()).toBe(200);
  expect(
    await timeline.getAttribute("data-worker-session-timeline-status"),
  ).toBe("completed");
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
          .getByRole("heading", { level: 1, name: "You Agent Factory", exact: true })
          .waitFor({ timeout: uiInteractionTimeoutMs });
        await selectReplayWork(browserPage.page, expect);

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
          .getByRole("heading", { level: 1, name: "You Agent Factory", exact: true })
          .waitFor({ timeout: uiInteractionTimeoutMs });
        await selectReplayWork(browserPage.page, expect);

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

  it(
    "WSR-UI-002 bounds long history, preserves focus during live append, and stays usable across viewports",
    async ({ expect, openBrowserPage, preview }) => {
      const replayServer = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: { name: "Worker Session Timeline Window Factory" },
        eventLines: await loadReplayLines("event-stream-replay.jsonl"),
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "worker-session-timeline-wsr-ui-002",
      });
      let releaseLiveStream = () => {};
      let releaseTerminalStream = () => {};
      const liveStreamReady = new Promise((resolve) => {
        releaseLiveStream = resolve;
      });
      const terminalStreamReady = new Promise((resolve) => {
        releaseTerminalStream = resolve;
      });

      try {
        const requests = await installWorkerSessionRoutes(browserPage.page, {
          beforeStreamResponse: async (_workID, requestNumber) => {
            if (requestNumber === 2) {
              await liveStreamReady;
            }
            if (requestNumber === 3) {
              await terminalStreamReady;
            }
          },
          streamBodyForRequest: (workID, requestNumber) => {
            if (requestNumber === 1) {
              return workerSessionLongHistoryBody(workID, 1, 201);
            }
            if (requestNumber === 2) {
              return workerSessionLongHistoryBody(workID, 202, 202);
            }
            return workerSessionTerminalBody(workID, 203);
          },
        });
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "You Agent Factory", exact: true })
          .waitFor({ timeout: uiInteractionTimeoutMs });
        await selectReplayWork(browserPage.page, expect);
        await browserPage.page.setViewportSize({ height: 900, width: 1440 });

        const timeline = browserPage.page
          .locator('[data-bento-card-id="worker-session-timeline"]')
          .locator('section[aria-label="Worker Session timeline"]');
        await timeline.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        const rows = timeline.locator(
          "li[data-worker-session-timeline-entry-position]",
        );
        await assertInitialLongHistoryWindow(
          expect,
          browserPage.page,
          timeline,
          rows,
        );
        const focusedDetailsButton = await navigateTimelineWindows(
          expect,
          browserPage.page,
          timeline,
          rows,
        );
        await assertPendingLiveActivity(
          expect,
          browserPage.page,
          timeline,
          rows,
          requests,
          releaseLiveStream,
          focusedDetailsButton,
        );

        await expect
          .poll(() => requests.events.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBeGreaterThanOrEqual(3);
        releaseTerminalStream();
        await assertCompletedTimelineResponsive(
          expect,
          browserPage.page,
          timeline,
          rows,
        );

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        releaseLiveStream();
        releaseTerminalStream();
        await browserPage.close();
        await replayServer.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
