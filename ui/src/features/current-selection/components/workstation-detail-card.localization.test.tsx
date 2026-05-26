import { fireEvent, render, screen, within } from "@testing-library/react";
import type { DashboardWorkstationRequest } from "../../../api/dashboard/types";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";
import { WorkstationDetailCard } from "./workstation-detail-card";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

describe("WorkstationDetailCard localization", () => {
  it("localizes completed request-history runtime copy for english fallback and zh-CN", () => {
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

    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="fr"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    let requestHistorySection = screen
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    let resolvedRequestHistorySection = requireValue(
      requestHistorySection,
      "expected request history section",
    );

    fireEvent.click(
      within(resolvedRequestHistorySection).getByRole("button", {
        name: "Expand",
      }),
    );
    expect(resolvedRequestHistorySection.textContent).toContain(
      "Total runtime: 4s",
    );

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    requestHistorySection = screen
      .getByRole("heading", { name: "请求历史" })
      .closest("section");
    resolvedRequestHistorySection = requireValue(
      requestHistorySection,
      "expected localized request history section",
    );
    expect(resolvedRequestHistorySection.textContent).toContain("总运行时间: 4s");
  });
});
