import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "vitest";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import {
  buildDashboardWorkstationRequestFixture,
  dashboardWorkstationRequestFixtures,
} from "../../../../components/dashboard/fixtures";
import { DETAIL_CARD_NOW } from "../../base/components/detail-card-test-helpers";
import { WorkstationDetailCard } from "./workstation-detail-card";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

describe("WorkstationDetailCard run history", () => {
  it("renders repeater rejected history as repeated work while preserving the raw outcome", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) => attempt.outcome === "REJECTED",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={requireValue(
          providerSessions,
          "expected rejected provider sessions fixture",
        )}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = requireValue(
      screen.getByRole("heading", { name: "Run history" }).closest("section"),
      "expected run history section",
    );
    fireEvent.click(
      within(runHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(within(runHistorySection).getByText("Repeated work")).toBeTruthy();
    expect(
      within(runHistorySection).getByText("Raw outcome: REJECTED"),
    ).toBeTruthy();
  });

  it("keeps non-repeater rejected history labeled as rejected", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.repair;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={[
          {
            dispatch_id: "dispatch-repair-rejected",
            outcome: "REJECTED",
            transition_id: selectedNode.transition_id,
            workstation_name: selectedNode.workstation_name,
            work_items: [
              {
                display_name: "Repair Review",
                trace_id: "trace-repair-review",
                work_id: "work-repair-review",
                work_type_id: "story",
              },
            ],
          },
        ]}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = requireValue(
      screen.getByRole("heading", { name: "Run history" }).closest("section"),
      "expected run history section",
    );
    fireEvent.click(
      within(runHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(within(runHistorySection).getByText("Rejected")).toBeTruthy();
    expect(within(runHistorySection).queryByText("Repeated work")).toBeNull();
    expect(
      within(runHistorySection).queryByText("Raw outcome: REJECTED"),
    ).toBeNull();
  });

  it("renders selected workstation historical empty state only after expansion", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.document;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      screen.queryByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeNull();

    const runHistorySection = requireValue(
      screen.getByRole("heading", { name: "Run history" }).closest("section"),
      "expected run history section",
    );
    fireEvent.click(
      within(runHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(
      within(runHistorySection).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });

});

describe("WorkstationDetailCard request history", () => {
  it("renders dispatch-keyed request history as the primary historical surface when projections exist", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onSelectWorkstationRequest = vi.fn();
    const requestHistory = [
      buildDashboardWorkstationRequestFixture("dispatch-review-history-b", {
        request_id: "request-history-b",
        started_at: "2026-04-08T12:00:05Z",
      }),
      {
        ...dashboardWorkstationRequestFixtures.scriptSuccess,
        started_at: "2026-04-08T12:00:06Z",
      },
    ];

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        providerSessions={snapshot.runtime.session.provider_sessions ?? []}
        selectedNode={selectedNode}
        workstationRequests={requestHistory}
      />,
    );

    const summarySection = requireValue(
      screen.getByRole("heading", { name: "Workstation summary" }).closest(
        "section",
      ),
      "expected workstation summary section",
    );
    expect(
      within(summarySection).getByText("Historical requests"),
    ).toBeTruthy();
    expect(within(summarySection).getByText("2")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Run history" })).toBeNull();

    const requestHistorySection = requireValue(
      screen.getByRole("heading", { name: "Request history" }).closest(
        "section",
      ),
      "expected request history section",
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    const pendingRuntimePill = within(requestHistorySection).getByText(
      "Elapsed: 0ms",
    );
    expect(pendingRuntimePill.className).toContain("border-af-info-border");
    expect(
      within(requestHistorySection).getByText("request-script-success-story"),
    ).toBeTruthy();
    const successfulRuntimePill = within(requestHistorySection).getByText(
      "Total runtime: 222ms",
    );
    expect(successfulRuntimePill.className).toContain(
      "border-af-success-border",
    );
    expect(
      within(requestHistorySection).getByRole("button", {
        name: "Select request request-script-success-story (dispatch-review-script-success)",
      }),
    ).toBeTruthy();

    fireEvent.click(
      within(requestHistorySection).getByRole("button", {
        name: "Select request request-script-success-story (dispatch-review-script-success)",
      }),
    );

    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(requestHistory[1]);
  });

  it("marks failed completed request-history runtime pills as danger", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={[dashboardWorkstationRequestFixtures.scriptFailed]}
      />,
    );

    const requestHistorySection = requireValue(
      screen.getByRole("heading", { name: "Request history" }).closest(
        "section",
      ),
      "expected request history section",
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    const failedRuntimePill = within(requestHistorySection).getByText(
      "Total runtime: 500ms",
    );
    expect(failedRuntimePill.className).toContain("border-af-danger-border");
  });

});

describe("WorkstationDetailCard history state", () => {
  it("resets selected workstation historical runs to collapsed when the workstation changes", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const implementNode = snapshot.topology.workstation_nodes_by_id.implement;
    const reviewProviderSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === reviewNode.transition_id ||
        attempt.workstation_name === reviewNode.workstation_name,
    );
    const implementProviderSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === implementNode.transition_id ||
        attempt.workstation_name === implementNode.workstation_name,
    );

    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={requireValue(
          reviewProviderSessions,
          "expected review provider sessions fixture",
        )}
        selectedNode={reviewNode}
      />,
    );

    let runHistorySection = requireValue(
      screen.getByRole("heading", { name: "Run history" }).closest("section"),
      "expected run history section",
    );
    fireEvent.click(
      within(runHistorySection).getByRole("button", { name: "Expand" }),
    );
    expect(within(runHistorySection).getByText("Rejected Story")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={requireValue(
          implementProviderSessions,
          "expected implement provider sessions fixture",
        )}
        selectedNode={implementNode}
      />,
    );

    runHistorySection = requireValue(
      screen.getByRole("heading", { name: "Run history" }).closest("section"),
      "expected run history section",
    );
    expect(
      within(runHistorySection)
        .getByRole("button", { name: "Expand" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.queryByText("Retry Story")).toBeNull();
    expect(screen.getAllByText("Implement").length).toBeGreaterThan(0);
  });
});
