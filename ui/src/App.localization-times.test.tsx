import "../testing/bun-app-shell-module-mocks";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, it, vi } from "bun:test";

import {
  dashboardWorkstationRequestFixtures,
} from "./components/dashboard/fixtures";
import {
  formatDurationMillis,
  formatLocalDateTime,
} from "./components/ui/formatters";
import type { DashboardTrace } from "./api/dashboard";
import {
  activeSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";

const activeWorkID = "work-active-story";
const now = Date.parse("2026-04-08T12:00:05Z");
const currentSelectionName = /^(Current selection|当前选择)$/;
const requestHistoryName = /^(Request history|请求历史)$/;
const inferenceAttemptsName = /^(Inference attempts|推理尝试)$/;
const activeStoryTraceFixtures = {
  [activeWorkID]: {
    dispatches: [
      {
        consumed_tokens: [],
        dispatch_id: "dispatch-review-active",
        duration_millis: 1_000,
        end_time: "2026-04-08T12:00:01Z",
        outcome: "ACCEPTED",
        output_mutations: [],
        provider_session: {
          id: "sess-active-story",
          kind: "session_id",
          provider: "codex",
        },
        start_time: "2026-04-08T12:00:00Z",
        transition_id: "plan",
        workstation_name: "Plan",
      },
    ],
    trace_id: "trace-active-story",
    transition_ids: ["plan", "review"],
    work_ids: [activeWorkID],
    workstation_sequence: ["Plan", "Review"],
  },
} satisfies Record<string, DashboardTrace>;

function getCurrentSelection(): HTMLElement {
  return screen.getByRole("article", { name: currentSelectionName });
}

function openReviewWorkstation(): void {
  fireEvent.click(
    screen.getByRole("button", { name: "Select Review workstation" }),
  );
}

function switchLocale(localeLabel: "English" | "简体中文"): void {
  const toolbar =
    screen.queryByRole("region", { name: "dashboard summary" }) ??
    screen.getByRole("region", { name: "仪表板概览" });

  fireEvent.click(
    within(toolbar).getByRole("button", {
      name: /^(Change language|切换语言)$/,
    }),
  );
  fireEvent.click(screen.getByRole("menuitemradio", { name: localeLabel }));
}

function requestHistorySection(selection: HTMLElement): HTMLElement {
  const section = within(selection)
    .getByRole("heading", { name: requestHistoryName })
    .closest("section");
  if (!(section instanceof HTMLElement)) {
    throw new Error("expected request history section");
  }
  return section;
}

function ensureRequestHistoryExpanded(selection: HTMLElement): void {
  const toggle = within(requestHistorySection(selection)).getByRole("button", {
    name: /^(Expand|Collapse|展开|收起)$/,
  });
  if (toggle.getAttribute("aria-expanded") !== "true") {
    fireEvent.click(toggle);
  }
}

function selectReviewRequest(dispatchID: string): void {
  const selection = getCurrentSelection();
  ensureRequestHistoryExpanded(selection);
  fireEvent.click(
    within(requestHistorySection(selection)).getByRole("button", {
      name: new RegExp(
        `\\(${dispatchID.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\)$`,
      ),
    }),
  );
}

function expandedAttempt(attemptNumber: number): HTMLElement {
  const inferenceAttempts = within(getCurrentSelection()).getByRole("region", {
    name: inferenceAttemptsName,
  });
  const attempt = within(inferenceAttempts).getByRole("article", {
    name: new RegExp(`^(Inference attempt|推理尝试) ${attemptNumber}$`),
  });

  fireEvent.click(
    within(attempt).getByRole("button", {
      name: new RegExp(`^(Expand|展开) (attempt|尝试) ${attemptNumber}$`),
    }),
  );
  return attempt;
}

function openAttemptBodies(
  attempt: HTMLElement,
  requestLabel: string,
  responseLabel: string,
): void {
  fireEvent.click(within(attempt).getByRole("button", { name: requestLabel }));
  fireEvent.click(within(attempt).getByRole("button", { name: responseLabel }));
}

function assertAttemptPayload(
  attempt: HTMLElement,
  requestText: string,
  responseText: string,
): void {
  expect(within(attempt).getByText(requestText)).toBeTruthy();
  expect(within(attempt).getByText(responseText)).toBeTruthy();
}

registerAppDashboardTestLifecycle();

it("rerenders current-selection request history and request-detail times when the locale changes", async () => {
  const dateNowSpy = vi.spyOn(Date, "now").mockReturnValue(now);

  try {
    renderApp({
      initialLocale: "en",
      snapshot: activeSnapshot,
      traceFixtures: activeStoryTraceFixtures,
      workstationRequestsByDispatchID: {
        [dashboardWorkstationRequestFixtures.ready.dispatch_id]:
          dashboardWorkstationRequestFixtures.ready,
      },
    });

    await screen.findByRole("button", { name: "Select Review workstation" });
    openReviewWorkstation();

    const englishSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    ensureRequestHistoryExpanded(englishSelection);
    expect(englishSelection.textContent).toContain("Total runtime:");
    expect(englishSelection.textContent).toContain(
      formatDurationMillis(63_000, "en"),
    );

    selectReviewRequest(dashboardWorkstationRequestFixtures.ready.dispatch_id);

    const englishAttempt = expandedAttempt(2);
    const englishRequestTime = formatLocalDateTime(
      "2026-04-08T12:00:01Z",
      "Unavailable",
      "en",
    );
    const englishResponseTime = formatLocalDateTime(
      "2026-04-08T12:00:04Z",
      "Unavailable",
      "en",
    );

    expect(within(getCurrentSelection()).getByText("request-ready-story")).toBeTruthy();
    expect(within(englishAttempt).getAllByText(englishRequestTime).length).toBeGreaterThan(
      0,
    );
    expect(within(englishAttempt).getAllByText(englishResponseTime).length).toBeGreaterThan(
      0,
    );
    openAttemptBodies(
      englishAttempt,
      "Expand request body",
      "Expand response body",
    );
    assertAttemptPayload(
      englishAttempt,
      "Retry the review with the latest context.",
      "Ready for the next workstation.",
    );
    expect(getCurrentSelection().textContent).toContain(
      formatDurationMillis(63_000, "en"),
    );

    switchLocale("简体中文");

    await waitFor(() => {
      expect(screen.getByRole("article", { name: "当前选择" })).toBeTruthy();
    });

    const localizedSelection = getCurrentSelection();
    const localizedAttempt = within(
      within(localizedSelection).getByRole("region", { name: "推理尝试" }),
    ).getByRole("article", { name: "推理尝试 2" });
    const chineseRequestTime = formatLocalDateTime(
      "2026-04-08T12:00:01Z",
      "不可用",
      "zh-CN",
    );
    const chineseResponseTime = formatLocalDateTime(
      "2026-04-08T12:00:04Z",
      "不可用",
      "zh-CN",
    );

    expect(within(localizedSelection).getByText("request-ready-story")).toBeTruthy();
    expect(within(localizedAttempt).getAllByText(chineseRequestTime).length).toBeGreaterThan(
      0,
    );
    expect(within(localizedAttempt).getAllByText(chineseResponseTime).length).toBeGreaterThan(
      0,
    );
    expect(within(localizedAttempt).queryByText(englishRequestTime)).toBeNull();
    expect(within(localizedAttempt).queryByText(englishResponseTime)).toBeNull();
    assertAttemptPayload(
      localizedAttempt,
      "Retry the review with the latest context.",
      "Ready for the next workstation.",
    );
    expect(localizedSelection.textContent).toContain(
      formatDurationMillis(63_000, "zh-CN"),
    );
    expect(localizedSelection.textContent).not.toContain(
      formatDurationMillis(63_000, "en"),
    );
  } finally {
    dateNowSpy.mockRestore();
  }
});

it("shows localized fallback copy for invalid request-detail timestamps without leaking broken raw values", async () => {
  const dateNowSpy = vi.spyOn(Date, "now").mockReturnValue(now);

  try {
    const invalidRequest = {
      ...dashboardWorkstationRequestFixtures.ready,
      inference_attempts: [
        {
          ...dashboardWorkstationRequestFixtures.ready.inference_attempts[0],
          request_time: " definitely-not-a-date ",
          response_time: undefined,
        },
      ],
      request_id: "request-invalid-story",
    };

    renderApp({
      initialLocale: "en",
      snapshot: activeSnapshot,
      traceFixtures: activeStoryTraceFixtures,
      workstationRequestsByDispatchID: {
        [invalidRequest.dispatch_id]: invalidRequest,
      },
    });

    await screen.findByRole("button", { name: "Select Review workstation" });
    openReviewWorkstation();
    selectReviewRequest(invalidRequest.dispatch_id);

    const englishAttempt = expandedAttempt(1);
    expect(within(getCurrentSelection()).getByText("request-invalid-story")).toBeTruthy();
    expect(within(englishAttempt).getAllByText("Unavailable")).toHaveLength(2);
    expect(within(englishAttempt).queryByText(" definitely-not-a-date ")).toBeNull();

    switchLocale("简体中文");

    await waitFor(() => {
      expect(screen.getByRole("article", { name: "当前选择" })).toBeTruthy();
    });

    const localizedAttempt = within(
      within(getCurrentSelection()).getByRole("region", { name: "推理尝试" }),
    ).getByRole("article", { name: "推理尝试 1" });
    expect(within(localizedAttempt).getAllByText("不可用")).toHaveLength(2);
    expect(within(localizedAttempt).queryByText(" definitely-not-a-date ")).toBeNull();
  } finally {
    dateNowSpy.mockRestore();
  }
});
