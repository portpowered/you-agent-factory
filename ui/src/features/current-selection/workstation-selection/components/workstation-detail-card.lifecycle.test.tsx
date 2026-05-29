import { fireEvent, render, screen, within } from "@testing-library/react";
import type { DashboardWorkstationRequest } from "../../../../api/dashboard/types";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "../../base/components/detail-card-test-helpers";
import { WorkstationDetailCard } from "./workstation-detail-card";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function expectHeadingBefore(first: HTMLElement, second: HTMLElement) {
  expect(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
}

describe("WorkstationDetailCard lifecycle separation", () => {
  it("keeps active work ahead of request history without duplicating summary facts", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const workstationRequests: DashboardWorkstationRequest[] = [
      {
        dispatch_id: "dispatch-review-active",
        dispatched_request_count: 1,
        errored_request_count: 0,
        inference_attempts: [],
        prompt: "Review the active story and decide whether it is ready.",
        responded_request_count: 1,
        transition_id: selectedNode.transition_id,
        work_items: [
          {
            display_name: "Active Story",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
        ],
        workstation_name: selectedNode.workstation_name,
        workstation_node_id: selectedNode.node_id,
      },
      {
        dispatch_id: "dispatch-review-rejected",
        dispatched_request_count: 1,
        errored_request_count: 0,
        inference_attempts: [],
        prompt: "Retry the review with the latest context.",
        responded_request_count: 0,
        transition_id: selectedNode.transition_id,
        work_items: [
          {
            display_name: "Rejected Story",
            trace_id: "trace-rejected-story",
            work_id: "work-rejected-story",
            work_type_id: "story",
          },
        ],
        workstation_name: selectedNode.workstation_name,
        workstation_node_id: selectedNode.node_id,
      },
    ];

    render(
      <WorkstationDetailCard
        activeExecutions={[
          requireValue(
            activeExecution,
            "expected active workstation execution fixture",
          ),
        ]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    const summaryHeading = screen.getByRole("heading", {
      name: "Workstation summary",
    });
    const activeWorkHeading = screen.getByRole("heading", { name: "Active work" });
    const requestHistoryHeading = screen.getByRole("heading", {
      name: "Request history",
    });
    expectHeadingBefore(summaryHeading, activeWorkHeading);
    expectHeadingBefore(activeWorkHeading, requestHistoryHeading);

    const activeWorkSection = requireValue(
      activeWorkHeading.closest("section"),
      "expected active work section",
    );
    const requestHistorySection = requireValue(
      requestHistoryHeading.closest("section"),
      "expected request history section",
    );

    expect(within(activeWorkSection).getByText("Active Story")).toBeTruthy();
    expect(
      within(activeWorkSection).queryByText("No active work is running on this workstation."),
    ).toBeNull();
    expect(within(activeWorkSection).queryByText("Rejected Story")).toBeNull();

    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(within(requestHistorySection).getByText("Rejected Story")).toBeTruthy();
    expect(within(requestHistorySection).queryByText("Input work types")).toBeNull();
    expect(within(requestHistorySection).queryByText("Output work types")).toBeNull();
    expect(within(requestHistorySection).queryByText("Active runs")).toBeNull();
    expect(within(requestHistorySection).queryByText("Historical requests")).toBeNull();
  });
});
