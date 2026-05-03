import { fireEvent, render, screen, within } from "@testing-library/react";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../components/dashboard/typography";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
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

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getAllByText("Work type").length).toBeGreaterThan(0);
    expect(screen.getByText("State")).toBeTruthy();
    expect(screen.getByText("State node ID")).toBeTruthy();
    expect(screen.getAllByText("implemented").length).toBeGreaterThan(0);
    expect(screen.getAllByText("story:implemented")).toHaveLength(1);
    expect(screen.getByText("Count")).toBeTruthy();
    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(screen.getByText("work-active-story")).toBeTruthy();
    expect(screen.getByText("trace-active-story")).toBeTruthy();
  });

  it("applies shared typography helpers to the state-node selection header", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(<StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />);

    const header = screen.getByTitle("story:implemented");
    const workType = within(header).getByText("story", { selector: "span" });
    const stateValue = within(header).getByText("implemented", { selector: "span" });

    expect(workType.className).toContain(DASHBOARD_SUPPORTING_LABEL_CLASS);
    expect(stateValue.className).toContain(WIDGET_SUBTITLE_CLASS);
  });

  it("renders selected state node empty-position guidance", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(<StateNodeDetailCard currentWorkItems={[]} place={resolvedSelectedState} tokenCount={0} />);

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText("State")).toBeTruthy();
    expect(screen.getAllByText("implemented").length).toBeGreaterThan(0);
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
    expect(screen.getAllByText("complete").length).toBeGreaterThan(0);
    expect(screen.getByText("State node ID")).toBeTruthy();
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

    expect(screen.getAllByText("blocked").length).toBeGreaterThan(0);
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
});

