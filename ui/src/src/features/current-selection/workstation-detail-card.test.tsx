import { fireEvent, render, screen, within } from "@testing-library/react";
import type { DashboardWorkstationRequest } from "../../../api/dashboard/types";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import {
  buildDashboardWorkstationRequestFixture,
  dashboardWorkstationRequestFixtures,
} from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";
import { WorkstationDetailCard } from "./workstation-detail-card";

describe("WorkstationDetailCard", () => {
  it("renders selected workstation detail with active workstation runs", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    expect(activeExecution).toBeDefined();
    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getAllByText(selectedNode.workstation_name).length).toBeGreaterThan(0);
    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    expect(within(activeWorkSection!).getByText("Active Story")).toBeTruthy();
    expect(within(activeWorkSection!).getByText("work-active-story")).toBeTruthy();
    expect(within(activeWorkSection!).getAllByText("dispatch-review-active").length).toBeGreaterThan(
      0,
    );
    expect(within(activeWorkSection!).getByText("4s")).toBeTruthy();

    const summaryHeading = screen.getByRole("heading", { name: "Workstation summary" });
    const runHistoryHeading = screen.getByRole("heading", { name: "Run history" });
    expect(
      activeWorkSection!.compareDocumentPosition(summaryHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      activeWorkSection!.compareDocumentPosition(runHistoryHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Expand" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByText("Rejected Story")).toBeNull();

    const summarySection = summaryHeading.closest("section");
    expect(summarySection).toBeTruthy();
    expect(within(summarySection!).getByText("Input work types")).toBeTruthy();
    expect(within(summarySection!).getByText("Output work types")).toBeTruthy();
    expect(within(summarySection!).getByText("Active runs")).toBeTruthy();
    expect(within(summarySection!).getByText("Historical runs")).toBeTruthy();
    expect(within(summarySection!).getByText("1")).toBeTruthy();
    expect(within(summarySection!).getByText("2")).toBeTruthy();
  });

  it("renders work selection affordances for active runs and run history", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );
    const onSelectWorkID = vi.fn();

    expect(activeExecution).toBeDefined();
    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={onSelectWorkID}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    fireEvent.click(
      within(activeWorkSection!).getByRole("button", {
        name: "Select work item Active Story",
      }),
    );
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));
    fireEvent.click(
      within(runHistorySection!).getByRole("button", {
        name: "Select work item Rejected Story",
      }),
    );
    expect(onSelectWorkID).toHaveBeenCalledWith("work-rejected-story");
  });

  it("routes workstation request selection affordances through active work and request history", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );
    const onSelectWorkstationRequest = vi.fn();
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

    expect(activeExecution).toBeDefined();
    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    fireEvent.click(
      within(activeWorkSection!).getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    );
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(workstationRequests[0]);

    const requestHistorySection = screen
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    expect(requestHistorySection).toBeTruthy();
    fireEvent.click(within(requestHistorySection!).getByRole("button", { name: "Expand" }));
    fireEvent.click(
      within(requestHistorySection!).getByRole("button", {
        name: "Select request Rejected Story (dispatch-review-rejected)",
      }),
    );
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(workstationRequests[1]);
  });

  it("uses shared supporting text for unavailable work status copy", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];

    expect(activeExecution).toBeDefined();

    const executionWithoutWork = {
      ...activeExecution!,
      work_items: undefined,
    };

    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[executionWithoutWork]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const unavailableWorkStatus = screen.getByText("Work details unavailable for dispatch", {
      exact: false,
    });
    expect(unavailableWorkStatus.className).toContain(DASHBOARD_SUPPORTING_TEXT_CLASS);
    expect(unavailableWorkStatus.className).not.toContain("text-[0.78rem]");

    rerender(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.queryByText("Work details unavailable for dispatch", { exact: false })).toBeNull();
  });

  it("renders explicit unavailable work copy when a dispatch has no work item details", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];

    expect(activeExecution).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[{ ...activeExecution!, work_items: undefined }]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    expect(
      within(activeWorkSection!).getByText("Work details unavailable for dispatch", {
        exact: false,
      }),
    ).toBeTruthy();
  });

  it("expands and collapses selected workstation historical runs without hiding active work", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    expect(activeExecution).toBeDefined();
    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    expect(within(activeWorkSection!).getByText("Active Story")).toBeTruthy();
    expect(screen.queryByText("Rejected Story")).toBeNull();

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    const expandButton = within(runHistorySection!).getByRole("button", { name: "Expand" });
    fireEvent.click(expandButton);

    expect(
      within(runHistorySection!).getByRole("button", { name: "Collapse" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("true");
    expect(within(runHistorySection!).getByText("Active Story")).toBeTruthy();
    expect(within(runHistorySection!).getByText("Rejected Story")).toBeTruthy();
    expect(within(runHistorySection!).getAllByText("dispatch-review-active").length).toBeGreaterThan(
      0,
    );
    expect(within(activeWorkSection!).getByText("Active Story")).toBeTruthy();

    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Collapse" }));

    expect(
      within(runHistorySection!).getByRole("button", { name: "Expand" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("false");
    expect(screen.queryByText("Rejected Story")).toBeNull();
    expect(within(activeWorkSection!).getByText("Active Story")).toBeTruthy();
  });

  it("renders explicit Codex session log links from local JSONL metadata", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={[
          {
            dispatch_id: "dispatch-review-jsonl",
            outcome: "ACCEPTED",
            provider_session: {
              id: "sess-jsonl",
              kind: "session_id",
              local_jsonl_path: "C:\\Users\\operator\\codex\\sess-jsonl.jsonl",
              provider: "codex",
            },
            transition_id: selectedNode.transition_id,
            workstation_name: selectedNode.workstation_name,
            work_items: [
              {
                display_name: "JSONL Story",
                trace_id: "trace-jsonl-story",
                work_id: "work-jsonl-story",
                work_type_id: "story",
              },
            ],
          },
        ]}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    const sessionLogLink = within(runHistorySection!).getByRole("link", {
      name: "Codex session log",
    });
    expect(sessionLogLink.getAttribute("href")).toBe(
      "file:///C:/Users/operator/codex/sess-jsonl.jsonl",
    );
    expect(within(runHistorySection!).getByText(/codex \/ session_id \/ sess-jsonl/)).toBeTruthy();
    expect(within(runHistorySection!).queryByText("Session log unavailable")).toBeNull();
  });

  it("falls back to secondary provider metadata when no explicit session log exists", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    expect(within(runHistorySection!).getAllByText("Session log unavailable").length).toBeGreaterThan(
      0,
    );
    expect(
      within(runHistorySection!).getByText(/codex \/ session_id \/ sess-rejected-story/),
    ).toBeTruthy();
    expect(within(runHistorySection!).queryByRole("link", { name: "Codex session log" })).toBeNull();
  });

  it("renders repeater rejected history as repeated work while preserving the raw outcome", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) => attempt.outcome === "REJECTED",
    );

    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    expect(within(runHistorySection!).getByText("Repeated work")).toBeTruthy();
    expect(within(runHistorySection!).getByText("Raw outcome: REJECTED")).toBeTruthy();
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

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    expect(within(runHistorySection!).getByText("Rejected")).toBeTruthy();
    expect(within(runHistorySection!).queryByText("Repeated work")).toBeNull();
    expect(within(runHistorySection!).queryByText("Raw outcome: REJECTED")).toBeNull();
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
      screen.queryByText("No workstation runs have been recorded for this workstation yet."),
    ).toBeNull();

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    expect(
      within(runHistorySection!).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });

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

    const summarySection = screen.getByRole("heading", { name: "Workstation summary" }).closest(
      "section",
    );
    expect(summarySection).toBeTruthy();
    expect(within(summarySection!).getByText("Historical requests")).toBeTruthy();
    expect(within(summarySection!).getByText("2")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Run history" })).toBeNull();

    const requestHistorySection = screen
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    expect(requestHistorySection).toBeTruthy();
    fireEvent.click(within(requestHistorySection!).getByRole("button", { name: "Expand" }));

    expect(within(requestHistorySection!).getByText("request-script-success-story")).toBeTruthy();
    expect(within(requestHistorySection!).getByText("Script command script-tool")).toBeTruthy();
    expect(within(requestHistorySection!).getByText("dispatch-review-script-success")).toBeTruthy();

    fireEvent.click(
      within(requestHistorySection!).getByRole("button", {
        name: "Select request request-script-success-story (dispatch-review-script-success)",
      }),
    );

    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(requestHistory[1]);
  });

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
        providerSessions={reviewProviderSessions ?? []}
        selectedNode={reviewNode}
      />,
    );

    let runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));
    expect(within(runHistorySection!).getByText("Rejected Story")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={implementProviderSessions ?? []}
        selectedNode={implementNode}
      />,
    );

    runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    expect(
      within(runHistorySection!).getByRole("button", { name: "Expand" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("false");
    expect(screen.queryByText("Retry Story")).toBeNull();
    expect(screen.getAllByText("Implement").length).toBeGreaterThan(0);
  });

  it("renders selected workstation empty active-work guidance with compact counts", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.implement;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) => attempt.transition_id === selectedNode.transition_id,
    );

    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    expect(activeWorkSection).toBeTruthy();
    expect(
      within(activeWorkSection!).getByText("No active work is running on this workstation."),
    ).toBeTruthy();

    const summarySection = screen.getByRole("heading", { name: "Workstation summary" }).closest(
      "section",
    );
    expect(summarySection).toBeTruthy();
    expect(within(summarySection!).getByText("Active runs")).toBeTruthy();
    expect(within(summarySection!).getByText("Historical runs")).toBeTruthy();
    expect(within(summarySection!).getByText("0")).toBeTruthy();
    expect(within(summarySection!).getByText("1")).toBeTruthy();

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    expect(
      within(runHistorySection!).getByRole("button", { name: "Expand" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("false");

    fireEvent.click(within(runHistorySection!).getByRole("button", { name: "Expand" }));

    expect(within(runHistorySection!).getByText("Retry Story")).toBeTruthy();
    expect(within(runHistorySection!).getByText("Session log unavailable")).toBeTruthy();
  });

  it("applies shared typography helpers to workstation drill-down cards", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    expect(activeExecution).toBeDefined();
    expect(providerSessions).toBeDefined();

    render(
      <WorkstationDetailCard
        activeExecutions={[activeExecution!]}
        now={DETAIL_CARD_NOW}
        providerSessions={providerSessions ?? []}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkHeading = screen.getByRole("heading", { name: "Active work" });
    expect(activeWorkHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);
    const activeWorkCard = screen.getByText("Active Story").closest("li");
    expect(activeWorkCard?.className).toContain(DASHBOARD_BODY_TEXT_CLASS);

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    expect(runHistorySection).toBeTruthy();
    const countText = within(runHistorySection as HTMLElement).getByText("2 runs");
    expect(countText.className).toContain(DASHBOARD_SUPPORTING_TEXT_CLASS);

    fireEvent.click(within(runHistorySection as HTMLElement).getByRole("button", { name: "Expand" }));

    const dispatchPill = within(runHistorySection as HTMLElement)
      .getAllByText("dispatch-review-active")
      .find((element) => element.tagName === "SPAN");
    expect(dispatchPill?.className).toContain(DASHBOARD_SUPPORTING_CODE_CLASS);
    const sessionMetadata = within(runHistorySection as HTMLElement).getByText(
      /codex \/ session_id \/ sess-active-story/,
    );
    expect(sessionMetadata.tagName).toBe("CODE");
    expect(sessionMetadata.className).toContain(DASHBOARD_BODY_CODE_CLASS);
  });
});
