import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";

import type { DashboardTrace } from "../../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { TraceGridBentoCard } from "../trace-grid-card";

const populatedTrace: DashboardTrace = {
  dispatches: [
    {
      current_chaining_trace_id: "trace-review-chain",
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      input_items: [
        {
          display_name: "Active Story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Reviewed Story",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      start_time: "2026-04-08T12:00:00Z",
      transition_id: "plan",
      workstation_name: "Plan",
    },
    {
      current_chaining_trace_id: "trace-implement-chain",
      dispatch_id: "dispatch-implement-active",
      duration_millis: 2000,
      end_time: "2026-04-08T12:00:04Z",
      input_items: [
        {
          display_name: "Reviewed Story",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Implemented Story",
          work_id: "work-implemented-story",
          work_type_id: "story",
        },
      ],
      previous_chaining_trace_ids: ["trace-review-chain"],
      start_time: "2026-04-08T12:00:02Z",
      transition_id: "implement",
      workstation_name: "Implement",
    },
  ],
  trace_id: "trace-active-story",
  transition_ids: ["plan", "implement"],
  work_ids: ["work-active-story"],
  work_items: [
    {
      display_name: "Active Story",
      work_id: "work-active-story",
      work_type_id: "story",
    },
    {
      display_name: "Reviewed Story",
      work_id: "work-reviewed-story",
      work_type_id: "story",
    },
    {
      display_name: "Implemented Story",
      work_id: "work-implemented-story",
      work_type_id: "story",
    },
  ],
  workstation_sequence: ["Plan", "Implement"],
};

describe("TraceGridBentoCard owner contract", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders dispatch content and delegates expanded work-item selection", () => {
    const onSelectWorkID = mock(() => {});
    render(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expect(within(card).getByText("trace-active-story")).toBeTruthy();
    expect(within(card).getByRole("table")).toBeTruthy();
    expect(within(card).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(card).getByText("dispatch-review-active")).toBeTruthy();
    expect(within(card).getByText("dispatch-implement-active")).toBeTruthy();
    expect(within(card).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(card).getByText("Accepted · 2s")).toBeTruthy();

    const workItems = within(card).getByRole("region", {
      name: "3 work items",
    });
    fireEvent.click(within(workItems).getByRole("button", { name: "Expand" }));
    fireEvent.click(
      within(workItems).getByRole("button", {
        name: "(story):Active Story",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");
  });

  it("renders explicit idle, empty, loading, and error states", () => {
    const { container, rerender } = render(
      <TraceGridBentoCard state={{ message: "Select work", status: "idle" }} />,
    );

    expect(screen.getByText("No trace selected")).toBeTruthy();

    rerender(
      <TraceGridBentoCard
        state={{ status: "empty", workID: "work-missing" }}
      />,
    );
    expect(screen.getByText("Trace history unavailable")).toBeTruthy();

    rerender(
      <TraceGridBentoCard
        state={{ status: "loading", workID: "work-active" }}
      />,
    );
    expect(screen.getByText("Loading trace")).toBeTruthy();
    expect(
      screen.getByText("Reconstructing dispatch history for work-active."),
    ).toBeTruthy();
    expect(container.querySelectorAll(".animate-pulse")).toHaveLength(3);

    rerender(
      <TraceGridBentoCard
        state={{ message: "network failed", status: "error" }}
      />,
    );
    expect(screen.getByText("Trace lookup failed")).toBeTruthy();
    expect(screen.getByText("network failed")).toBeTruthy();
  });

  it("renders the trace shell and table labels in zh-CN", () => {
    render(
      <TraceGridBentoCard
        locale="zh-CN"
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "追踪下钻" });
    expect(within(card).getByText("追踪分派表")).toBeTruthy();
    expect(within(card).getByText("分派流")).toBeTruthy();
    expect(
      within(card).getByRole("columnheader", { name: "输入项" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("columnheader", { name: "输出项" }),
    ).toBeTruthy();
  });
});
