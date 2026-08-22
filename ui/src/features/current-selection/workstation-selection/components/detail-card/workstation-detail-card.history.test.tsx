import "../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "vitest";
import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
  dashboardWorkstationRequestFixtures,
} from "../../../../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "../../../base/components/detail-card/detail-card-test-helpers";
import { WorkstationDetailCard } from "./workstation-detail-card";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function buildMultiWorkstationRequest(dispatchID: string) {
  const workItems = [
    ["First Story", "work-first-story"],
    ["Second Story", "work-second-story"],
    ["Third Story", "work-third-story"],
  ].map(([displayName, workID]) => ({
    display_name: displayName,
    trace_id: `trace-${workID}`,
    work_id: workID,
    work_type_id: "story",
  }));

  return buildDashboardWorkstationRequestFixture(dispatchID, {
    inference_attempts: [
      buildDashboardInferenceAttemptFixture(dispatchID, {
        attempt: 2,
        inference_request_id: `${dispatchID}/inference-request/2`,
        outcome: "SUCCEEDED",
      }),
      buildDashboardInferenceAttemptFixture(dispatchID, {
        attempt: 1,
        inference_request_id: `${dispatchID}/inference-request/1`,
        outcome: "FAILED",
      }),
    ],
    request_id: "request-review-multi-work",
    script_request: {
      attempt: 1,
      command: "review-script",
      script_request_id: `${dispatchID}/script-request/1`,
    },
    script_response: {
      duration_millis: 125,
      outcome: "SUCCEEDED",
      script_request_id: `${dispatchID}/script-request/1`,
    },
    work_items: workItems,
  });
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

describe("WorkstationDetailCard preserves every request Work and attempt", () => {
  it("renders every work item and recorded inference or script attempt", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onSelectWorkID = vi.fn();
    const dispatchID = "dispatch-review-multi-work";
    const workstationRequest = buildMultiWorkstationRequest(dispatchID);

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={onSelectWorkID}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={[workstationRequest]}
      />,
    );

    const requestHistorySection = requireValue(
      screen
        .getByRole("heading", { name: "Request history" })
        .closest("section"),
      "expected request history section",
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(
      within(requestHistorySection).getByText("Work ID: work-first-story"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText("Work ID: work-second-story"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText("Work ID: work-third-story"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText(
        `${dispatchID}/inference-request/1`,
      ),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText(
        `${dispatchID}/inference-request/2`,
      ),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getAllByText(
        `${dispatchID}/script-request/1`,
      ).length,
    ).toBeGreaterThan(1);

    const historyText = requestHistorySection.textContent ?? "";
    expect(
      historyText.indexOf(`${dispatchID}/inference-request/1`),
    ).toBeLessThan(historyText.indexOf(`${dispatchID}/inference-request/2`));

    for (const work of [
      ["First Story", "work-first-story"],
      ["Second Story", "work-second-story"],
      ["Third Story", "work-third-story"],
    ]) {
      fireEvent.click(
        within(requestHistorySection).getByRole("button", {
          name: `Select work item ${work[0]}`,
        }),
      );
    }

    expect(onSelectWorkID.mock.calls.map(([workID]) => workID)).toEqual([
      "work-first-story",
      "work-second-story",
      "work-third-story",
    ]);
  });
});

describe("WorkstationDetailCard request history empty work", () => {
  it("renders an explicit unknown work state when a request has no work items", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={vi.fn()}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={[
          buildDashboardWorkstationRequestFixture("dispatch-review-empty", {
            request_id: "request-review-empty",
            work_items: [],
          }),
        ]}
      />,
    );

    const requestHistorySection = requireValue(
      screen
        .getByRole("heading", { name: "Request history" })
        .closest("section"),
      "expected request history section",
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    expect(
      within(requestHistorySection).getByText("Unknown work"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).queryByRole("button", {
        name: /Select work item/,
      }),
    ).toBeNull();
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
      screen
        .getByRole("heading", { name: "Workstation summary" })
        .closest("section"),
      "expected workstation summary section",
    );
    expect(
      within(summarySection).getByText("Historical requests"),
    ).toBeTruthy();
    expect(within(summarySection).getByText("2")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Run history" })).toBeNull();

    const requestHistorySection = requireValue(
      screen
        .getByRole("heading", { name: "Request history" })
        .closest("section"),
      "expected request history section",
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    const requestHistoryList = within(requestHistorySection).getByRole("list");
    expect(requestHistoryList.className).toContain("gap-2.5");
    const requestHistoryRows =
      within(requestHistoryList).getAllByRole("listitem");
    expect(requestHistoryRows).toHaveLength(2);
    expect(requestHistoryRows[0]?.className).toContain("rounded-lg");

    const pendingRuntimePill = within(requestHistorySection).getByText(
      "Elapsed: 0ms",
    );
    expect(pendingRuntimePill.className).toContain("border-af-info-border");
    expect(
      within(requestHistorySection).getAllByText("Active Story").length,
    ).toBeGreaterThan(0);
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
        name: "Select workstation request dispatch-review-script-success",
      }),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getAllByText("Open request details").length,
    ).toBeGreaterThan(0);

    fireEvent.click(
      within(requestHistorySection).getByRole("button", {
        name: "Select workstation request dispatch-review-script-success",
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
      screen
        .getByRole("heading", { name: "Request history" })
        .closest("section"),
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
    const reviewProviderSessions =
      snapshot.runtime.session.provider_sessions?.filter(
        (attempt) =>
          attempt.transition_id === reviewNode.transition_id ||
          attempt.workstation_name === reviewNode.workstation_name,
      );
    const implementProviderSessions =
      snapshot.runtime.session.provider_sessions?.filter(
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
