import "@testing-library/jest-dom/vitest";

import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DashboardSnapshot, DashboardTrace } from "./api/dashboard";
import { dashboardWorkstationRequestFixtures } from "./components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS } from "./features/bento/hooks/dashboardLayoutSchema";
import { useCurrentWorkstationPromptTemplateValidation } from "./features/current-selection/workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import {
  registerAppDashboardTestLifecycle,
  renderApp,
  terminalSnapshot,
} from "./testing/app-shell-test-utils";
import { seedTimelineSnapshot } from "./testing/app-shell-timeline-seed-utils";

vi.mock(
  "./features/current-selection/workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation: vi.fn(),
  }),
);

const activeWorkID = "work-active-story";
const completedWorkID = "work-complete";
const failedWorkID = "work-failed-story";
const traceSnapshot: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: [activeWorkID],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      transition_id: "plan",
      workstation_name: "Plan",
      outcome: "ACCEPTED",
      provider_session: {
        provider: "codex",
        kind: "session_id",
        id: "sess-active-story",
      },
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [
        {
          token_id: "tok-plan-in",
          place_id: "story:init",
          work_id: activeWorkID,
          work_type_id: "story",
          trace_id: "trace-active-story",
          created_at: "2026-04-08T11:59:58Z",
          entered_at: "2026-04-08T11:59:59Z",
        },
      ],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-plan-in",
          from_place: "story:init",
          to_place: "story:ready",
          resulting_token: {
            token_id: "tok-plan-out",
            place_id: "story:ready",
            work_id: activeWorkID,
            work_type_id: "story",
            trace_id: "trace-active-story",
            created_at: "2026-04-08T12:00:01Z",
            entered_at: "2026-04-08T12:00:01Z",
          },
        },
      ],
    },
  ],
};

const activeStoryTraceFixtures = {
  [activeWorkID]: traceSnapshot,
} satisfies Record<string, DashboardTrace>;

const reworkTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  transition_ids: ["plan", "review", "plan"],
  workstation_sequence: ["Plan", "Review", "Plan"],
  dispatches: [
    ...traceSnapshot.dispatches,
    {
      dispatch_id: "dispatch-review-rejected",
      transition_id: "review",
      workstation_name: "Review",
      outcome: "REJECTED",
      start_time: "2026-04-08T12:00:01Z",
      end_time: "2026-04-08T12:03:13Z",
      duration_millis: 192_000,
      consumed_tokens: [],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-review-in",
          from_place: "story:implemented",
          to_place: "story:ready",
          reason: "review rejected story",
        },
      ],
    },
  ],
};

const activeStoryReworkTraceFixtures = {
  [activeWorkID]: reworkTraceSnapshot,
} satisfies Record<string, DashboardTrace>;

const completedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-done-story",
  work_ids: [completedWorkID],
  workstation_sequence: ["Complete"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-done-story",
      workstation_name: "Complete",
    },
  ],
};

const failedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-failed-story",
  work_ids: [failedWorkID],
  workstation_sequence: ["Review", "Failure"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-failed-story",
      outcome: "FAILED",
      workstation_name: "Failure",
    },
  ],
};

const terminalStateTraceFixtures = {
  [completedWorkID]: completedTraceSnapshot,
  [failedWorkID]: failedTraceSnapshot,
} satisfies Record<string, DashboardTrace>;

const readyDispatchWorkstationRequestsByDispatchID = {
  [dashboardWorkstationRequestFixtures.ready.dispatch_id]:
    dashboardWorkstationRequestFixtures.ready,
} satisfies Record<string, DashboardWorkstationRequest>;

const refreshedDispatchWorkstationRequestsByDispatchID = {
  [dashboardWorkstationRequestFixtures.ready.dispatch_id]: {
    ...dashboardWorkstationRequestFixtures.ready,
    counts: {
      dispatched_count: 2,
      errored_count: 1,
      responded_count: 1,
    },
    failure_message: "Projection refresh exposed the terminal failure.",
    failure_reason: "projection_refresh_failure",
    outcome: "FAILED",
  },
} satisfies Record<string, DashboardWorkstationRequest>;

function getActiveStorySelectionButton(): HTMLElement {
  const explicitSelectionButton = screen.queryByLabelText(
    "Select work item Active Story",
  );
  if (explicitSelectionButton) {
    return explicitSelectionButton;
  }

  const activeStoryButton = screen
    .getAllByRole("button")
    .find((button) =>
      /^Active Story/.test(
        button.getAttribute("aria-label") ?? button.textContent ?? "",
      ),
    );

  if (!(activeStoryButton instanceof HTMLElement)) {
    throw new Error("expected an Active Story selection button");
  }

  return activeStoryButton;
}

function mockSuccessfulWorkstationPromptValidation(): void {
  vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue({
    data: {
      diagnostics: [],
      valid: true,
    },
    error: null,
    isError: false,
    isPending: false,
    isSuccess: true,
    status: "success",
  } as never);
}

function resizeDashboardViewport(width: number): void {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    value: width,
    writable: true,
  });
  Object.defineProperty(window, "innerHeight", {
    configurable: true,
    value: width < 720 ? 720 : 900,
    writable: true,
  });
  window.dispatchEvent(new Event("resize"));
}

describe("App current selection", () => {
  registerAppDashboardTestLifecycle();

  beforeEach(() => {
    mockSuccessfulWorkstationPromptValidation();
  });

  it("renders a trace drill-down for a selected work item", async () => {
    renderApp({
      snapshot: semanticWorkflowDashboardSnapshot,
      traceFixtures: activeStoryTraceFixtures,
    });

    await screen.findByRole("button", {
      name: "Select work item Active Story",
    });
    fireEvent.click(getActiveStorySelectionButton());

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByText(
        "codex / Session ID / sess-active-story",
      ),
    ).toBeTruthy();
    expect(
      document
        .querySelector("[data-bento-card-id='trace']")
        ?.getAttribute("id"),
    ).toBe(DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace);
    expect(
      within(currentSelection).getByRole("heading", {
        name: /Work operations|Workstation dispatches/,
      }),
    ).toBeTruthy();
    expect(
      within(currentSelection).getAllByText(
        /codex \/ Session ID \/ sess-active-story/,
      )[0],
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
    const traceCard = screen.getByRole("article", { name: "Trace drill-down" });
    expect(traceCard).toBeTruthy();
    expect(within(traceCard).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(traceCard).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(traceCard).queryByText("Workstation run")).toBeNull();
    expect(within(traceCard).queryByText("Consumed tokens")).toBeNull();
    expect(within(traceCard).queryByText("Output mutations")).toBeNull();
  });

  it("follows the explicit selection contract: clicking work selects work, clicking a request selects a request", async () => {
    renderApp({
      snapshot: semanticWorkflowDashboardSnapshot,
      traceFixtures: activeStoryTraceFixtures,
      workstationRequestsByDispatchID:
        readyDispatchWorkstationRequestsByDispatchID,
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select work item Active Story",
      }),
    );

    const workDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(workDetail).getByText(activeWorkID)).toBeTruthy();
    expect(
      within(workDetail).getByRole("heading", {
        name: "Work operations",
      }),
    ).toBeTruthy();
    expect(
      within(workDetail).queryByRole("heading", {
        name: "Request history",
      }),
    ).toBeNull();
    act(() => {
      seedTimelineSnapshot(
        {
          ...semanticWorkflowDashboardSnapshot,
          tick_count: semanticWorkflowDashboardSnapshot.tick_count + 1,
        } satisfies DashboardSnapshot,
        activeStoryTraceFixtures,
        readyDispatchWorkstationRequestsByDispatchID,
      );
    });
    await waitFor(() => {
      expect(within(workDetail).getByText(activeWorkID)).toBeTruthy();
    });

    fireEvent.click(
      await screen.findByLabelText("Select Review workstation"),
    );

    const workstationDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    const requestHistorySection = within(workstationDetail)
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    if (!(requestHistorySection instanceof HTMLElement)) {
      throw new Error("expected workstation request history section");
    }

    fireEvent.click(
      within(requestHistorySection).getByRole("button", {
        name: "Expand",
      }),
    );
    const requestButton = within(requestHistorySection).getByRole("button", {
      name: "Select workstation request dispatch-review-ready",
    });
    expect(requestButton.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(requestButton);

    const requestDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(requestDetail).getByRole("heading", {
        name: "Request details",
      }),
    ).toBeTruthy();
    expect(
      within(requestDetail).queryByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeNull();
    expect(
      within(requestDetail).getAllByText("dispatch-review-ready").length,
    ).toBeGreaterThan(0);

    act(() => {
      seedTimelineSnapshot(
        {
          ...semanticWorkflowDashboardSnapshot,
          tick_count: semanticWorkflowDashboardSnapshot.tick_count + 2,
        } satisfies DashboardSnapshot,
        activeStoryTraceFixtures,
        refreshedDispatchWorkstationRequestsByDispatchID,
      );
    });
    await waitFor(() => {
      expect(
        within(requestDetail).getByRole("heading", {
          name: "Request details",
        }),
      ).toBeTruthy();
    });
    expect(
      within(requestDetail).getAllByText("dispatch-review-ready").length,
    ).toBeGreaterThan(0);
    expect(
      within(requestDetail).getByText(
        "Projection refresh exposed the terminal failure.",
      ),
    ).toBeTruthy();
    expect(
      within(requestDetail).getAllByText("projection_refresh_failure").length,
    ).toBeGreaterThan(0);
  });

  it("keeps shared workstation selection usable after React Flow zoom", async () => {
    renderApp({
      snapshot: semanticWorkflowDashboardSnapshot,
      traceFixtures: activeStoryTraceFixtures,
    });

    await screen.findByRole("button", {
      name: "Select work item Active Story",
    });

    const workGraphViewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });
    fireEvent.click(
      within(workGraphViewport).getByRole("button", { name: "Zoom In" }),
    );

    const planButton = await screen.findByLabelText("Select Plan workstation");
    fireEvent.click(planButton);
    await waitFor(() => {
      expect(planButton.getAttribute("aria-pressed")).toBe("true");
    });
    expect(screen.getByText("Input work types")).toBeTruthy();

    const reviewButton = await screen.findByLabelText(
      "Select Review workstation",
    );
    fireEvent.click(reviewButton);
    await waitFor(() => {
      expect(
        screen
          .getByLabelText("Select Review workstation")
          .getAttribute("aria-pressed"),
      ).toBe("true");
      expect(
        screen
          .getByLabelText("Select Plan workstation")
          .getAttribute("aria-pressed"),
      ).toBeNull();
    });
  }, 30_000);

  it("separates workstation selection from active work selection", async () => {
    renderApp({
      snapshot: semanticWorkflowDashboardSnapshot,
      traceFixtures: activeStoryTraceFixtures,
    });

    await screen.findByRole("button", {
      name: "Select work item Active Story",
    });

    const reviewButton = await screen.findByLabelText(
      "Select Review workstation",
    );
    fireEvent.click(reviewButton);
    await waitFor(() => {
      expect(reviewButton.getAttribute("aria-pressed")).toBe("true");
    });
    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();

    const workButton = (
      await screen.findAllByRole("button", { name: /Active Story/ })
    )[0];
    fireEvent.click(workButton);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Current selection" }),
      ).toBeTruthy();
    });
    expect(
      screen
        .getByLabelText("Select Review workstation")
        .getAttribute("aria-pressed"),
    ).toBeNull();
    expect(
      within(screen.getByRole("article", { name: "Current selection" }))
        .getByText(activeWorkID),
    ).toBeTruthy();

    const planButton = await screen.findByLabelText("Select Plan workstation");
    fireEvent.click(planButton);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Current selection" }),
      ).toBeTruthy();
    });
    expect(
      screen
        .getByLabelText("Select Plan workstation")
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen
        .getByLabelText("Select Review workstation")
        .getAttribute("aria-pressed"),
    ).toBeNull();
  });

  describe("layout", () => {
    it("keeps selection detail out of the workflow graph inspector layer", async () => {
      renderApp({
        snapshot: semanticWorkflowDashboardSnapshot,
        traceFixtures: activeStoryTraceFixtures,
      });

      await screen.findByRole("button", {
        name: "Select work item Active Story",
      });

      expect(
        screen.getByRole("region", { name: "Work graph viewport" }),
      ).toBeTruthy();
      expect(
        screen.queryByRole("complementary", { name: "Workstation Info" }),
      ).toBeNull();
      expect(
        screen.queryByRole("button", { name: "Collapse inspector" }),
      ).toBeNull();
      expect(
        screen.queryByRole("button", { name: "Expand inspector" }),
      ).toBeNull();
      expect(
        screen.getByRole("article", { name: "Current selection" }),
      ).toBeTruthy();
    });

    it("renders selected work and traces on the shared dashboard grid", async () => {
      renderApp({
        snapshot: semanticWorkflowDashboardSnapshot,
        traceFixtures: activeStoryTraceFixtures,
      });

      fireEvent.click(
        await screen.findByRole("button", {
          name: "Select work item Active Story",
        }),
      );

      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      const workInfo = await within(dashboardGrid).findByRole("article", {
        name: "Current selection",
      });
      expect(workInfo).toBeTruthy();
      expect(screen.getByLabelText("Work graph viewport")).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Completed and failed work",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Trace drill-down",
        }),
      ).toBeTruthy();
      expect(within(dashboardGrid).getByText("Trace drill-down")).toBeTruthy();
      expect(
        await within(dashboardGrid).findByText("Trace dispatch grid"),
      ).toBeTruthy();
    });

    it("supports rearranging shared-grid widgets without replacing graph selection", async () => {
      renderApp({
        snapshot: semanticWorkflowDashboardSnapshot,
        traceFixtures: activeStoryTraceFixtures,
      });

      fireEvent.click(
        await screen.findByRole("button", {
          name: "Select work item Active Story",
        }),
      );

      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      const traceWidget = await within(dashboardGrid).findByRole("article", {
        name: "Trace drill-down",
      });
      const traceGridItem = traceWidget.closest(
        ".react-grid-item",
      ) as HTMLElement;
      const initialStyle = traceGridItem.getAttribute("style");
      const initialGraphSelectionState = screen
        .getByLabelText("Select Review workstation")
        .getAttribute("aria-pressed");

      fireEvent.mouseDown(
        within(traceWidget).getByRole("heading", {
          name: "Trace drill-down",
        }),
        {
          button: 0,
          buttons: 1,
          clientX: 120,
          clientY: 40,
        },
      );
      fireEvent.mouseMove(document, {
        buttons: 1,
        clientX: 360,
        clientY: 40,
      });
      fireEvent.mouseUp(document, {
        button: 0,
        clientX: 360,
        clientY: 40,
      });

      await waitFor(() => {
        expect(traceGridItem.getAttribute("style")).not.toBe(initialStyle);
      });
      const storedLayout = window.localStorage.getItem(
        "agent-factory.dashboard.layout.v2",
      );
      expect(storedLayout).toContain(
        `"id":"${DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace}"`,
      );

      const movedStyle = traceGridItem.getAttribute("style");
      act(() => {
        seedTimelineSnapshot(
          {
            ...semanticWorkflowDashboardSnapshot,
            tick_count: semanticWorkflowDashboardSnapshot.tick_count + 1,
          } satisfies DashboardSnapshot,
          activeStoryTraceFixtures,
        );
      });

      await waitFor(() => {
        expect(traceGridItem.getAttribute("style")).toBe(movedStyle);
      });
      expect(
        (await screen.findByLabelText("Select Review workstation")).getAttribute(
          "aria-pressed",
        ),
      ).toBe(initialGraphSelectionState);
      expect(
        await within(dashboardGrid).findByText("Trace dispatch grid"),
      ).toBeTruthy();
    });

    it("keeps retry, rework, and timing trends hidden when selected trace data is available", async () => {
      renderApp({
        snapshot: terminalSnapshot,
        traceFixtures: activeStoryReworkTraceFixtures,
      });

      await screen.findByRole("heading", { name: "U" });
      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      expect(
        within(dashboardGrid).queryByRole("article", { name: "Failure trend" }),
      ).toBeNull();

      fireEvent.click(getActiveStorySelectionButton());

      const workDetail = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      expect(
        await within(workDetail).findByRole("article", {
          name: "Current selection",
        }),
      ).toBeTruthy();
      expect(
        within(workDetail).getByRole("article", { name: "Trace drill-down" }),
      ).toBeTruthy();
      expect(
        within(workDetail).queryByRole("article", {
          name: "Retry and rework trend",
        }),
      ).toBeNull();
      expect(
        within(workDetail).queryByRole("article", { name: "Timing trend" }),
      ).toBeNull();
      expect(
        workDetail.querySelector('[data-bento-card-id="rework-trend"]'),
      ).toBeNull();
      expect(
        workDetail.querySelector('[data-bento-card-id="timing-trend"]'),
      ).toBeNull();
    });

    it("smoke tests the composed bento dashboard at a narrow viewport", async () => {
      resizeDashboardViewport(640);
      renderApp({
        snapshot: terminalSnapshot,
        traceFixtures: activeStoryTraceFixtures,
      });

      await screen.findByRole("heading", { name: "U" });
      expect(
        screen.getAllByRole("region", {
          name: "you-agent-factory bento board",
        }),
      ).toHaveLength(1);
      expect(
        screen.getByRole("article", { name: "Factory graph" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("region", { name: "Work graph viewport" }),
      ).toBeTruthy();

      const activeWorkButton = (
        await screen.findAllByRole("button", { name: /Active Story/ })
      )[0];
      fireEvent.click(activeWorkButton);

      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Work outcome chart",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", { name: "Submit work" }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("heading", {
          name: "No work outcome samples",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).queryByRole("img", {
          name: "Work outcome chart for Session",
        }),
      ).toBeNull();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Trace drill-down",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Completed and failed work",
        }),
      ).toBeTruthy();
      expect(
        await within(dashboardGrid).findByText("Trace dispatch grid"),
      ).toBeTruthy();
      await waitFor(() => {
        expect(
          screen
            .getAllByRole("button", { name: /Active Story/ })[0]
            ?.getAttribute("aria-pressed"),
        ).toBe("true");
      });

      const outcomeWidget = within(dashboardGrid).getByRole("article", {
        name: "Work outcome chart",
      });
      const outcomeGridItem = outcomeWidget.closest(
        ".react-grid-item",
      ) as HTMLElement;
      const initialOutcomeStyle = outcomeGridItem.getAttribute("style");

      fireEvent.mouseDown(
        within(outcomeWidget).getByRole("heading", {
          name: "Work outcome chart",
        }),
        {
          button: 0,
          buttons: 1,
          clientX: 120,
          clientY: 40,
        },
      );
      fireEvent.mouseMove(document, {
        buttons: 1,
        clientX: 360,
        clientY: 40,
      });
      fireEvent.mouseUp(document, {
        button: 0,
        clientX: 360,
        clientY: 40,
      });

      expect(outcomeGridItem.getAttribute("style")).toBe(initialOutcomeStyle);

      const terminalWidget = within(dashboardGrid).getByRole("article", {
        name: "Completed and failed work",
      });
      const completedRow = within(terminalWidget)
        .getByRole("heading", { name: "Completed" })
        .closest("section");
      const failedRow = within(terminalWidget)
        .getByRole("heading", { name: "Failed" })
        .closest("section");

      if (
        !(completedRow instanceof HTMLElement) ||
        !(failedRow instanceof HTMLElement)
      ) {
        throw new Error(
          "expected completed and failed rows to render as terminal sections",
        );
      }

      fireEvent.click(
        within(completedRow).getByRole("button", { name: "Collapse" }),
      );
      fireEvent.click(
        within(failedRow).getByRole("button", { name: "Collapse" }),
      );
      fireEvent.click(
        within(completedRow).getByRole("button", { name: "Expand" }),
      );
      fireEvent.click(
        within(failedRow).getByRole("button", { name: "Expand" }),
      );

      expect(
        within(completedRow).getByRole("button", {
          name: "Select work item Done Story",
        }),
      ).toBeTruthy();
      expect(
        within(failedRow).getByRole("button", {
          name: "Select work item Failed Story",
        }),
      ).toBeTruthy();
      expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
        window.innerWidth,
      );
    });
  });

  describe("terminal states", () => {
    it("opens completed and failed work summaries and updates the trace card", async () => {
      renderApp({
        snapshot: terminalSnapshot,
        traceFixtures: terminalStateTraceFixtures,
      });

      await screen.findByRole("heading", { name: "U" });
      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      const completedRow = within(dashboardGrid)
        .getByRole("heading", { name: "Completed" })
        .closest("section");
      const failedRow = within(dashboardGrid)
        .getByRole("heading", { name: "Failed" })
        .closest("section");

      if (
        !(completedRow instanceof HTMLElement) ||
        !(failedRow instanceof HTMLElement)
      ) {
        throw new Error("expected terminal work sections");
      }

      fireEvent.click(
        within(completedRow).getByRole("button", {
          name: "Select work item Done Story",
        }),
      );

      const completedDetail = await screen.findByRole("article", {
        name: "Current selection",
      });
      expect(
        within(completedRow)
          .getByRole("button", { name: "Select work item Done Story" })
          .getAttribute("aria-pressed"),
      ).toBe("true");
      expect(within(completedDetail).getByText("Done Story")).toBeTruthy();
      expect(
        within(completedDetail).queryByRole("heading", {
          name: "Execution details",
        }),
      ).toBeNull();
      expect(
        within(completedDetail).queryByRole("heading", {
          name: "Request details",
        }),
      ).toBeNull();
      expect(
        within(completedDetail).queryByRole("heading", {
          name: "Request counts",
        }),
      ).toBeNull();
      expect(within(completedDetail).queryByText("Failure reason")).toBeNull();
      expect(completedDetail).toBeTruthy();
      expect(
        await within(dashboardGrid).findByText("dispatch-done-story"),
      ).toBeTruthy();

      fireEvent.click(
        within(failedRow).getByRole("button", {
          name: "Select work item Failed Story",
        }),
      );

      const failedDetail = await screen.findByRole("article", {
        name: "Current selection",
      });
      expect(
        within(completedRow)
          .getByRole("button", { name: "Select work item Done Story" })
          .getAttribute("aria-pressed"),
      ).toBe("false");
      expect(
        within(failedRow)
          .getByRole("button", { name: "Select work item Failed Story" })
          .getAttribute("aria-pressed"),
      ).toBe("true");
      expect(within(failedDetail).getByText("Failed Story")).toBeTruthy();
      expect(
        within(failedDetail).queryByRole("heading", {
          name: "Execution details",
        }),
      ).toBeNull();
      expect(
        within(failedDetail).queryByRole("heading", {
          name: "Request details",
        }),
      ).toBeNull();
      expect(
        within(failedDetail).queryByRole("heading", {
          name: "Request counts",
        }),
      ).toBeNull();
      expect(
        within(failedDetail).getByRole("heading", {
          name: "Work operations",
        }),
      ).toBeTruthy();
      expect(
        within(failedDetail).getAllByText(/FAILED|Failed/).length,
      ).toBeGreaterThanOrEqual(1);
      expect(within(failedDetail).queryByText("Failure reason")).toBeNull();
      expect(within(failedDetail).getByText("Current dispatch")).toBeTruthy();
      expect(
        within(failedDetail).getByText("Session log unavailable"),
      ).toBeTruthy();
      expect(
        within(failedDetail).getByText(
          "codex / Session ID / sess-failed-story",
        ),
      ).toBeTruthy();
      expect(
        within(failedDetail).queryByText(
          "Terminal summaries are reconstructed from retained runtime state.",
        ),
      ).toBeNull();
      expect(
        await within(dashboardGrid).findByText("dispatch-repair-failed"),
      ).toBeTruthy();
    });
  });
});
