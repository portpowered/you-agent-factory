import "../testing/bun-app-shell-module-mocks";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import type { DashboardTrace } from "./api/dashboard";
import { describe, expect, it } from "vitest";
import {
  activeSnapshot,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import {
  activeWorkID,
  buildLegacyTraceTimelineEvents,
  buildTraceFanInTimelineEvents,
  fanInResultLabel,
  fanInResultWorkID,
  renderTraceDrilldownHarness,
} from "./testing/app-shell-trace-follow-up-test-utils";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";

const traceSnapshot: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: [activeWorkID],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      consumed_tokens: [
        {
          created_at: "2026-04-08T11:59:58Z",
          entered_at: "2026-04-08T11:59:59Z",
          place_id: "story:init",
          token_id: "tok-plan-in",
          trace_id: "trace-active-story",
          work_id: activeWorkID,
          work_type_id: "story",
        },
      ],
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      outcome: "ACCEPTED",
      output_mutations: [
        {
          from_place: "story:init",
          resulting_token: {
            created_at: "2026-04-08T12:00:01Z",
            entered_at: "2026-04-08T12:00:01Z",
            place_id: "story:ready",
            token_id: "tok-plan-out",
            trace_id: "trace-active-story",
            work_id: activeWorkID,
            work_type_id: "story",
          },
          to_place: "story:ready",
          token_id: "tok-plan-in",
          type: "MOVE",
        },
      ],
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
};

describe("App follow-up trace flows", () => {
  registerAppDashboardTestLifecycle();

  it("smoke tests predecessor-aware trace drill-down from streamed events through selected work resolution", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: fanInResultWorkID,
      timelineEvents: buildTraceFanInTimelineEvents(),
    });

    const snapshot = useFactoryTimelineStore.getState().worldViewCache[8];
    const dispatchRequest =
      snapshot?.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-implement"
      ];
    expect(
      dispatchRequest?.request?.input_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Research Context",
        trace_id: "chain-b",
        work_id: "work-research-context",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Reviewed Story",
        trace_id: "chain-a",
        work_id: "work-reviewed-story",
        work_type_id: "story",
      },
    ]);
    expect(
      dispatchRequest?.response?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: fanInResultLabel,
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: fanInResultWorkID,
        work_type_id: "story",
      },
    ]);

    const traceCard = await screen.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(await within(traceCard).findByText("Trace dispatch grid")).toBeTruthy();
    expect(
      await within(traceCard).findByRole("region", {
        name: "Dispatch relationship graph",
      }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(within(traceCard).getByText("dispatch-plan")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-research")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-implement")).toBeTruthy();
    });
    expect(within(traceCard).getAllByText(/Reviewed Story/).length).toBeGreaterThan(0);
    expect(within(traceCard).getAllByText(/Research Context/).length).toBeGreaterThan(0);
    expect(within(traceCard).getAllByText(new RegExp(fanInResultLabel)).length).toBeGreaterThan(0);
  });

  it("smoke tests legacy trace drill-down fallback from streamed events without predecessor metadata", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: "work-legacy-done",
      timelineEvents: buildLegacyTraceTimelineEvents(),
    });

    const traceCard = await screen.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(await within(traceCard).findByText("Trace dispatch grid")).toBeTruthy();
    expect(
      await within(traceCard).findByRole("region", {
        name: "Dispatch relationship graph",
      }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(within(traceCard).getByText("dispatch-legacy-review")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-legacy-complete")).toBeTruthy();
    });
    expect(within(traceCard).queryByText("dispatch-research")).toBeNull();
  });

  it("resolves trace drill-down from selected-tick events without fetching current trace state", async () => {
    const { fetchMock } = renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);
    await screen.findByText("Trace dispatch grid");

    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });
});
