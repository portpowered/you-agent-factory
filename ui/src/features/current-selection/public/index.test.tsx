import { fireEvent, render, screen, within } from "@testing-library/react";
import type { DashboardWorkstationRequest } from "../../../api/dashboard/types";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "../components/detail-card-test-helpers";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { WorkstationDetailCard } from "./index";

describe("current-selection public barrel", () => {
  it("keeps workstation-detail consumers rendering through the public barrel while message catalogs stay direct imports", () => {
    const messages = getWorkstationDetailMessages("zh-CN");
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const workstationRequests: DashboardWorkstationRequest[] = [
      {
        dispatch_id: "dispatch-review-active",
        dispatched_request_count: 1,
        errored_request_count: 0,
        inference_attempts: [],
        prompt: "Review the active story and decide whether it is ready.",
        request_id: "req-active-story",
        responded_request_count: 1,
        started_at: "2026-04-08T12:00:00Z",
        total_duration_millis: 4_000,
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
    ];

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    expect(
      screen.getByRole("heading", { name: messages.summaryHeading }),
    ).toBeTruthy();

    const requestHistorySection = screen
      .getByRole("heading", { name: messages.requestHistoryHeading })
      .closest("section");

    if (!requestHistorySection) {
      throw new Error("expected request history section");
    }

    fireEvent.click(
      within(requestHistorySection).getByRole("button", {
        name: messages.expandAction,
      }),
    );

    expect(requestHistorySection.textContent).toContain(
      `${messages.totalRuntimeLabel}: 4s`,
    );
  });
});
