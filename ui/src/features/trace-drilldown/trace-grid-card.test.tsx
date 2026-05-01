import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "vitest";

vi.mock("./trace-workstation-path", () => ({
  TraceWorkstationPath: () => (
    <section aria-label="Dispatch relationship graph">Dispatch relationship graph</section>
  ),
}));

import { TraceGridBentoCard } from "./trace-grid-card";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../components/dashboard/typography";
import { installDashboardBrowserTestShims } from "../../components/dashboard/test-browser-shims";
import type { DashboardTrace } from "../../api/dashboard/types";

const populatedTrace: DashboardTrace = {
  dispatches: [
    {
      input_items: [
        {
          display_name: "Active Story",
          current_chaining_trace_id: "trace-active-story-chain",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-review-chain",
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      start_time: "2026-04-08T12:00:00Z",
      transition_id: "plan",
      workstation_name: "Plan",
    },
    {
      input_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-implement-chain",
      dispatch_id: "dispatch-implement-active",
      duration_millis: 2000,
      end_time: "2026-04-08T12:00:04Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Implemented Story",
          current_chaining_trace_id: "trace-implement-chain",
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
  trace_id: "trace-active-story",
  relations: [
    {
      request_id: "request-story-batch",
      required_state: "DONE",
      source_work_id: "work-active-story",
      source_work_name: "Active Story",
      target_work_id: "work-reviewed-story",
      target_work_name: "Reviewed Story",
      type: "PARENT_CHILD",
    },
  ],
  transition_ids: ["plan", "implement"],
  work_ids: ["work-active-story"],
  workstation_sequence: ["Plan", "Implement"],
};

describe("TraceGridBentoCard", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders populated trace data as a bento card table", () => {
    const onSelectWorkID = vi.fn();
    const { rerender } = render(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expect(within(card).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(card).getByText("Dispatch flow")).toBeTruthy();
    expect(
      within(card).getByRole("region", { name: "Dispatch relationship graph" }),
    ).toBeTruthy();
    const table = within(card).getByRole("table");
    expect(table.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    const caption = within(card).getByText("Trace dispatch grid");
    expect(caption.className).toContain(DASHBOARD_SUPPORTING_LABEL_CLASS);
    const inputHeader = within(card).getByRole("columnheader", { name: "Input items" });
    expect(inputHeader.className).toContain(DASHBOARD_SUPPORTING_LABEL_CLASS);
    expect(within(card).getAllByText("Plan").length).toBeGreaterThan(0);
    expect(within(card).getAllByText("Implement").length).toBeGreaterThan(0);
    const dispatchPill = within(card)
      .getAllByText("dispatch-review-active")
      .find((element) => element.tagName === "SPAN");
    if (!dispatchPill) {
      throw new Error("Expected dispatch pill to render in the trace grid table.");
    }
    expect(dispatchPill.className).toContain(DASHBOARD_SUPPORTING_CODE_CLASS);
    expect(within(card).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(card).getByText("Accepted · 2s")).toBeTruthy();
    const workItemsSection = within(card)
      .getByText("Work items")
      .closest("div");
    expect(workItemsSection).toBeTruthy();
    const expandButton = within(workItemsSection!).getByRole("button", { name: "Expand" });
    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(workItemsSection!).queryByText('story:"Implemented Story"'),
    ).toBeNull();

    fireEvent.click(expandButton);

    expect(expandButton.getAttribute("aria-expanded")).toBe("true");
    expect(within(card).getAllByText('story:"Active Story"').length).toBeGreaterThan(0);
    expect(within(card).getAllByText('story:"Reviewed Story"').length).toBeGreaterThan(0);
    expect(within(card).getAllByText('story:"Implemented Story"').length).toBeGreaterThan(0);
    expect(within(card).getByRole("region", { name: "Batch relation graph" })).toBeTruthy();
    expect(within(card).queryByRole("columnheader", { name: "Consumed tokens" })).toBeNull();
    expect(within(card).queryByRole("columnheader", { name: "Output mutations" })).toBeNull();
    expect(within(card).queryByRole("columnheader", { name: "Workstation run" })).toBeNull();
    fireEvent.click(within(card).getAllByRole("button", { name: 'story:"Active Story"' })[0]);
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");

    rerender(
      <TraceGridBentoCard
        onSelectWorkID={onSelectWorkID}
        state={{ status: "ready", trace: populatedTrace }}
      />,
    );

    expect(within(card).getByRole("region", { name: "Batch relation graph" })).toBeTruthy();
  });

  it("renders explicit empty, loading, and error states", () => {
    const { rerender } = render(
      <TraceGridBentoCard state={{ status: "empty", workID: "work-missing" }} />,
    );

    expect(screen.getByText("Trace history unavailable")).toBeTruthy();

    rerender(<TraceGridBentoCard state={{ status: "loading", workID: "work-active" }} />);
    expect(screen.getByText("Loading trace")).toBeTruthy();
    expect(screen.getByText("Reconstructing dispatch history for work-active.")).toBeTruthy();

    rerender(<TraceGridBentoCard state={{ status: "error", message: "network failed" }} />);
    expect(screen.getByText("Trace lookup failed")).toBeTruthy();
    expect(screen.getByText("network failed")).toBeTruthy();
  });
});
