import { fireEvent, render, screen, within } from "@testing-library/react";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import { StateNodeDetailCard } from "./state-node-detail";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

describe("StateNodeDetailCard", () => {
  it("renders selected state node detail with current work item references", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(
      <StateNodeDetailCard
        currentWorkItems={[
          {
            display_name: "Active Story",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
        ]}
        place={resolvedSelectedState}
        tokenCount={1}
      />,
    );

    const summaryDetails = screen.getByText("Count").closest("dl");

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText("story: implemented")).toBeTruthy();
    expect(summaryDetails).toBeTruthy();
    expect(within(requireValue(summaryDetails, "expected summary details")).queryByText("Work type")).toBeNull();
    expect(within(requireValue(summaryDetails, "expected summary details")).queryByText("State")).toBeNull();
    expect(within(requireValue(summaryDetails, "expected summary details")).queryByText("State node ID")).toBeNull();
    expect(screen.getByText("Count")).toBeTruthy();
    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(screen.getByText("work-active-story")).toBeTruthy();
    expect(screen.getByText("trace-active-story")).toBeTruthy();
  });

  it("renders an English fallback when current work has no work type", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(
      <StateNodeDetailCard
        currentWorkItems={[
          {
            display_name: "Active Story",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
          },
        ]}
        place={resolvedSelectedState}
        tokenCount={1}
      />,
    );

    const summaryDetails = screen.getByText("Count").closest("dl");

    expect(summaryDetails).toBeTruthy();
    expect(within(requireValue(summaryDetails, "expected summary details")).queryByText("Work type")).toBeNull();
    expect(screen.getByText("Unknown")).toBeTruthy();
    expect(screen.queryByText("不明")).toBeNull();
  });

  it("renders the state-node selection header as one combined summary", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(<StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />);

    const header = screen.getByTitle("story:implemented");
    const summary = within(header).getByText("story: implemented", { selector: "p" });

    expect(summary.className).toContain(WIDGET_SUBTITLE_CLASS);
  });

  it("renders selected state node empty-position guidance", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(<StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />);

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText("story: implemented")).toBeTruthy();
    expect(screen.queryByText("State")).toBeNull();
    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(screen.getByText("No current work is occupying this place.")).toBeTruthy();
  });

  it("renders selected terminal state node detail from terminal-history occupancy", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
      (place) => place.place_id === "story:complete",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected terminal state fixture");

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        place={resolvedSelectedState}
        terminalHistoryWorkItems={[
          {
            display_name: "Done Story",
            trace_id: "trace-done-story",
            work_id: "work-done-story",
            work_type_id: "story",
          },
        ]}
        tokenCount={1}
      />,
    );

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText("story: complete")).toBeTruthy();
    expect(screen.queryByText("State node ID")).toBeNull();
    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(screen.getByText("Done Story")).toBeTruthy();
    expect(screen.getByText("work-done-story")).toBeTruthy();
    expect(screen.getByText("trace-done-story")).toBeTruthy();
    expect(screen.getAllByText("story").length).toBeGreaterThan(0);
    expect(screen.queryByText("No current work is occupying this place.")).toBeNull();
  });

  it("renders failed terminal state diagnostics from retained failed-work details", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.implement.output_places?.find(
      (place) => place.place_id === "story:blocked",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected failed state fixture");

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        failedWorkDetailsByWorkID={{
          "work-failed-story": {
            dispatch_id: "dispatch-failed-story",
            failure_message: "Provider rate limit exceeded while generating the repair.",
            failure_reason: "provider_rate_limit",
            transition_id: "repair",
            work_item: {
              display_name: "Failed Story",
              trace_id: "trace-failed-story",
              work_id: "work-failed-story",
              work_type_id: "story",
            },
          },
        }}
        place={resolvedSelectedState}
        terminalHistoryWorkItems={[
          {
            display_name: "Failed Story",
            trace_id: "trace-failed-story",
            work_id: "work-failed-story",
            work_type_id: "story",
          },
        ]}
        tokenCount={1}
      />,
    );

    expect(screen.getByText("story: blocked")).toBeTruthy();
    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(screen.getByText("Failed Story")).toBeTruthy();
    expect(screen.getByText("work-failed-story")).toBeTruthy();
    expect(screen.getByText("Failure reason")).toBeTruthy();
    expect(screen.getByText("provider_rate_limit")).toBeTruthy();
    expect(screen.getByText("Failure message")).toBeTruthy();
    expect(screen.getByText("Provider rate limit exceeded while generating the repair.")).toBeTruthy();
  });

  it("distinguishes empty terminal state positions from unavailable terminal history", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
      (place) => place.place_id === "story:complete",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected terminal state fixture");

    const { rerender } = render(
      <StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />,
    );

    expect(screen.getByText("No work is recorded for this place at the selected tick.")).toBeTruthy();
    expect(screen.queryByText(/terminal history/i)).toBeNull();

    rerender(<StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={1} />);

    expect(screen.getByText("Represented work is unavailable for this place at the selected tick.")).toBeTruthy();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
  });

  it("calls the selection callback when a listed work item is clicked", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );
    const onSelectWorkItem = vi.fn();

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(
      <StateNodeDetailCard
        currentWorkItems={[
          {
            display_name: "Active Story",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
        ]}
        onSelectWorkItem={onSelectWorkItem}
        place={resolvedSelectedState}
        tokenCount={1}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Select work item Active Story" }));

    expect(onSelectWorkItem).toHaveBeenCalledWith({
      display_name: "Active Story",
      trace_id: "trace-active-story",
      work_id: "work-active-story",
      work_type_id: "story",
    });
  });

  it("renders state-node supporting copy from the zh-CN locale catalog", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
      (place) => place.place_id === "story:complete",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected terminal state fixture");

    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("story: complete")).toBeTruthy();
    expect(screen.queryByText("状态")).toBeNull();
    expect(screen.queryByText("状态节点 ID")).toBeNull();
    expect(screen.getByText("当前工作")).toBeTruthy();
    expect(screen.getByText("在所选时间刻度，这个位置暂时没有记录到工作。")).toBeTruthy();
  });
});
