import { fireEvent, render, screen, within } from "@testing-library/react";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";
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

function expectSectionLabelledByHeading(heading: HTMLElement) {
  const section = requireValue(
    heading.closest("section"),
    `expected ${heading.textContent} section`,
  );
  const headingId = heading.getAttribute("id");

  expect(headingId).toBeTruthy();
  expect(section.getAttribute("aria-labelledby")).toBe(headingId);

  return section;
}

function expectLocalizedSelectionControlNames() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const onSelectWorkID = vi.fn();
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

  const resolvedActiveExecution = requireValue(
    activeExecution,
    "expected active workstation execution fixture",
  );
  const { rerender } = render(
    <WorkstationDetailCard
      activeExecutions={[resolvedActiveExecution]}
      locale="fr"
      now={DETAIL_CARD_NOW}
      onSelectWorkID={onSelectWorkID}
      onSelectWorkstationRequest={onSelectWorkstationRequest}
      providerSessions={[]}
      selectedNode={selectedNode}
      workstationRequests={workstationRequests}
    />,
  );

  let activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
  let resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
  let requestHistorySection = screen
    .getByRole("heading", { name: "Request history" })
    .closest("section");
  let resolvedRequestHistorySection = requireValue(
    requestHistorySection,
    "expected request history section",
  );

  expect(
    within(resolvedActiveWorkSection).getByRole("button", {
      name: "Select work item Active Story",
    }),
  ).toBeTruthy();
  expect(
    within(resolvedActiveWorkSection).getByRole("button", {
      name: "Select workstation request dispatch-review-active",
    }),
  ).toBeTruthy();

  fireEvent.click(within(resolvedRequestHistorySection).getByRole("button", { name: "Expand" }));
  expect(
    within(resolvedRequestHistorySection).getByRole("button", {
      name: "Select request Rejected Story (dispatch-review-rejected)",
    }),
  ).toBeTruthy();

  rerender(
    <WorkstationDetailCard
      activeExecutions={[resolvedActiveExecution]}
      locale="ja"
      now={DETAIL_CARD_NOW}
      onSelectWorkID={onSelectWorkID}
      onSelectWorkstationRequest={onSelectWorkstationRequest}
      providerSessions={[]}
      selectedNode={selectedNode}
      workstationRequests={workstationRequests}
    />,
  );

  activeWorkSection = screen.getByRole("heading", { name: "アクティブな作業" }).closest("section");
  resolvedActiveWorkSection = requireValue(activeWorkSection, "expected localized active work section");
  requestHistorySection = screen
    .getByRole("heading", { name: "リクエスト履歴" })
    .closest("section");
  resolvedRequestHistorySection = requireValue(
    requestHistorySection,
    "expected localized request history section",
  );

  expect(
    within(resolvedActiveWorkSection).getByRole("button", {
      name: "ワークアイテム Active Story を選択",
    }),
  ).toBeTruthy();
  expect(
    within(resolvedActiveWorkSection).getByRole("button", {
      name: "ワークステーションリクエスト dispatch-review-active を選択",
    }),
  ).toBeTruthy();

  expect(
    within(resolvedRequestHistorySection).getByRole("button", { name: "折りたたむ" }),
  ).toBeTruthy();
  expect(
    within(resolvedRequestHistorySection).getByRole("button", {
      name: "リクエスト Rejected Story (dispatch-review-rejected) を選択",
    }),
  ).toBeTruthy();
}

describe("WorkstationDetailCard provider-session selection", () => {
  it("keeps workstation run selection controls without embedding provider-session detail", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );
    const onSelectProviderSession = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        onSelectProviderSession={onSelectProviderSession}
        providerSessions={requireValue(
          providerSessions,
          "expected workstation provider sessions fixture",
        )}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = screen
      .getByRole("heading", { name: "Run history" })
      .closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected run history section",
    );

    fireEvent.click(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }),
    );
    fireEvent.click(
      within(resolvedRunHistorySection).getByRole("button", {
        name: "Select provider session codex / session_id / sess-active-story for dispatch dispatch-review-active",
      }),
    );

    expect(onSelectProviderSession).toHaveBeenCalledWith({
      dispatchID: "dispatch-review-active",
      id: "sess-active-story",
      kind: "session_id",
      provider: "codex",
    });
    expect(
      screen.queryByRole("heading", { name: "Selected session details" }),
    ).toBeNull();
  });
});

describe("WorkstationDetailCard", () => {
  it("falls back to English workstation-detail copy for unsupported locales", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="fr"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const summaryHeading = screen.getByRole("heading", {
      name: "Workstation summary",
    });
    const configurationHeading = screen.getByRole("heading", {
      name: "Configuration",
    });
    const activeWorkHeading = screen.getByRole("heading", { name: "Active work" });
    expect(summaryHeading).toBeTruthy();
    expect(configurationHeading).toBeTruthy();
    expect(activeWorkHeading).toBeTruthy();
    expect(screen.getByText("Worker type")).toBeTruthy();
    expect(screen.getByText("Selected runner")).toBeTruthy();
    expect(screen.getByText("No active work is running on this workstation.")).toBeTruthy();
    expectHeadingBefore(summaryHeading, configurationHeading);
    expectHeadingBefore(configurationHeading, activeWorkHeading);

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected fallback run history section",
    );
    fireEvent.click(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }),
    );
    expect(
      within(resolvedRunHistorySection).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });


  it("renders workstation-detail copy from the requested locale when provided", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="ja"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const summaryHeading = screen.getByRole("heading", {
      name: "ワークステーション概要",
    });
    const configurationHeading = screen.getByRole("heading", {
      name: "構成",
    });
    const activeWorkHeading = screen.getByRole("heading", {
      name: "アクティブな作業",
    });
    expect(summaryHeading).toBeTruthy();
    expect(configurationHeading).toBeTruthy();
    expect(activeWorkHeading).toBeTruthy();
    expect(screen.getByText("ワーカータイプ")).toBeTruthy();
    expect(screen.getByText("選択中の runner")).toBeTruthy();
    expect(
      screen.getByText("このワークステーションでは現在アクティブな作業は実行されていません。"),
    ).toBeTruthy();
    expectHeadingBefore(summaryHeading, configurationHeading);
    expectHeadingBefore(configurationHeading, activeWorkHeading);

    const runHistorySection = screen
      .getByRole("heading", { name: "ラン履歴" })
      .closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected localized run history section",
    );
    fireEvent.click(
      within(resolvedRunHistorySection).getByRole("button", { name: "展開" }),
    );
    expect(
      within(resolvedRunHistorySection).getByText(
        "このワークステーションではまだワークステーションのランが記録されていません。",
      ),
    ).toBeTruthy();
    expect(
      within(resolvedRunHistorySection).getByText("0 件のラン"),
    ).toBeTruthy();
  });

  it("localizes historical run controls and statuses for fallback and translated locales", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const runHistoryAttempts = [
      {
        dispatch_id: "dispatch-review-active",
        diagnostics: {
          provider: {
            request_metadata: {
              request_time: "2026-04-08T12:00:00Z",
            },
          },
        },
        outcome: "ACCEPTED",
        provider_session: {
          id: "sess-active-story",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: selectedNode.transition_id,
        workstation_name: selectedNode.workstation_name,
        work_items: [
          {
            display_name: "Active Story",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
        ],
      },
      {
        dispatch_id: "dispatch-review-missing-work",
        outcome: "FAILED",
        provider_session: {
          id: "sess-missing-work",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: selectedNode.transition_id,
        workstation_name: selectedNode.workstation_name,
      },
    ];
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="fr"
        now={DETAIL_CARD_NOW}
        onSelectWorkID={() => {}}
        providerSessions={runHistoryAttempts}
        selectedNode={selectedNode}
        selectedWorkID="work-active-story"
      />,
    );

    let runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    let resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected fallback run history section",
    );
    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));

    expect(
      within(resolvedRunHistorySection).getByRole("link", { name: "Codex session log" }),
    ).toBeTruthy();
    expect(within(resolvedRunHistorySection).getByText("Work selected")).toBeTruthy();
    expect(
      within(resolvedRunHistorySection).getAllByText("Session log unavailable").length,
    ).toBeGreaterThan(0);
    expect(
      within(resolvedRunHistorySection).getByText("Work details unavailable for dispatch", {
        exact: false,
      }),
    ).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        locale="ja"
        now={DETAIL_CARD_NOW}
        onSelectWorkID={() => {}}
        providerSessions={runHistoryAttempts}
        selectedNode={selectedNode}
        selectedWorkID="work-active-story"
      />,
    );

    runHistorySection = screen.getByRole("heading", { name: "ラン履歴" }).closest("section");
    resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected localized run history section",
    );

    expect(
      within(resolvedRunHistorySection).getByRole("link", {
        name: "Codex セッションログ",
      }),
    ).toBeTruthy();
    expect(
      within(resolvedRunHistorySection).getByRole("button", {
        name: "ワークアイテム Active Story を選択",
      }),
    ).toBeTruthy();
    expect(within(resolvedRunHistorySection).getByText("ワークを選択済み")).toBeTruthy();
    expect(
      within(resolvedRunHistorySection).getAllByText("セッションログは利用できません").length,
    ).toBeGreaterThan(0);
    expect(
      within(resolvedRunHistorySection).getByText("ディスパッチ dispatch-review-missing-work の作業詳細は利用できません。"),
    ).toBeTruthy();
  });

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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );
    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[resolvedActiveExecution]}
        now={DETAIL_CARD_NOW}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getAllByText(selectedNode.workstation_name).length).toBeGreaterThan(0);
    const summaryHeading = screen.getByRole("heading", { name: "Workstation summary" });
    const configurationHeading = screen.getByRole("heading", { name: "Configuration" });
    const activeWorkHeading = screen.getByRole("heading", { name: "Active work" });
    const runHistoryHeading = screen.getByRole("heading", { name: "Run history" });
    expectHeadingBefore(summaryHeading, configurationHeading);
    expectHeadingBefore(configurationHeading, activeWorkHeading);
    expectHeadingBefore(activeWorkHeading, runHistoryHeading);
    expectSectionLabelledByHeading(summaryHeading);
    expectSectionLabelledByHeading(configurationHeading);
    expectSectionLabelledByHeading(activeWorkHeading);
    expectSectionLabelledByHeading(runHistoryHeading);

    const activeWorkSection = activeWorkHeading.closest("section");
    const resolvedActiveWorkSection = requireValue(
      activeWorkSection,
      "expected active work section",
    );
    expect(within(resolvedActiveWorkSection).getByText("Active Story")).toBeTruthy();
    expect(
      within(resolvedActiveWorkSection).getByText("Elapsed: 4s"),
    ).toBeTruthy();

    expect(screen.getByRole("button", { name: "Expand" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByText("Rejected Story")).toBeNull();

    const summarySection = summaryHeading.closest("section");
    const resolvedSummarySection = requireValue(summarySection, "expected workstation summary section");
    expect(within(resolvedSummarySection).getByText("Input work types")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("Output work types")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("Active runs")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("Historical runs")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("1")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("2")).toBeTruthy();
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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );
    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[resolvedActiveExecution]}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={onSelectWorkID}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    const resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
    const activeWorkCard = requireValue(
      within(resolvedActiveWorkSection).getByText("Active Story").closest("li"),
      "expected active work card",
    );
    const activeWorkActions = requireValue(
      activeWorkCard.querySelector<HTMLElement>(
        "[data-dashboard-action-row-section='actions']",
      ),
      "expected active work actions",
    );

    expect(
      within(activeWorkActions).getByRole("button", {
        name: "Select work item Active Story",
      }),
    ).toBeTruthy();

    fireEvent.click(
      within(resolvedActiveWorkSection).getByRole("button", {
        name: "Select work item Active Story",
      }),
    );
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    const resolvedRunHistorySection = requireValue(runHistorySection, "expected run history section");
    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));
    fireEvent.click(
      within(resolvedRunHistorySection).getByRole("button", {
        name: "Select work item Rejected Story",
      }),
    );
    expect(onSelectWorkID).toHaveBeenCalledWith("work-rejected-story");
  });

  it("localizes active-work and request-history selection control names", () => {
    expectLocalizedSelectionControlNames();
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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );
    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[resolvedActiveExecution]}
        now={DETAIL_CARD_NOW}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    const resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
    const activeWorkCard = requireValue(
      within(resolvedActiveWorkSection).getByText("Active Story").closest("li"),
      "expected active work card",
    );
    const activeWorkActions = requireValue(
      activeWorkCard.querySelector<HTMLElement>(
        "[data-dashboard-action-row-section='actions']",
      ),
      "expected active work actions",
    );

    expect(
      within(activeWorkActions).getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    ).toBeTruthy();

    fireEvent.click(
      within(resolvedActiveWorkSection).getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    );
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(workstationRequests[0]);

    const requestHistorySection = screen
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    const resolvedRequestHistorySection = requireValue(
      requestHistorySection,
      "expected request history section",
    );
    fireEvent.click(within(resolvedRequestHistorySection).getByRole("button", { name: "Expand" }));
    fireEvent.click(
      within(resolvedRequestHistorySection).getByRole("button", {
        name: "Select request Rejected Story (dispatch-review-rejected)",
      }),
    );
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(workstationRequests[1]);
  });

  it("renders work selection affordances for projected request history", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
    const onSelectWorkID = vi.fn();
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
        onSelectWorkID={onSelectWorkID}
        providerSessions={[]}
        selectedNode={selectedNode}
        workstationRequests={workstationRequests}
      />,
    );

    const requestHistorySection = requireValue(
      screen.getByRole("heading", { name: "Request history" }).closest("section"),
      "expected request history section",
    );

    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );
    fireEvent.click(
      within(requestHistorySection).getByRole("button", {
        name: "Select work item Rejected Story",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-rejected-story");
  });

  it("uses shared supporting text for unavailable work status copy", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );

    const executionWithoutWork = {
      ...resolvedActiveExecution,
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
        activeExecutions={[resolvedActiveExecution]}
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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[{ ...resolvedActiveExecution, work_items: undefined }]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    const resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
    expect(
      within(resolvedActiveWorkSection).getByText("Work details unavailable for dispatch", {
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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );
    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[resolvedActiveExecution]}
        now={DETAIL_CARD_NOW}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    const resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
    expect(within(resolvedActiveWorkSection).getByText("Active Story")).toBeTruthy();
    expect(screen.queryByText("Rejected Story")).toBeNull();

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    const resolvedRunHistorySection = requireValue(runHistorySection, "expected run history section");
    const expandButton = within(resolvedRunHistorySection).getByRole("button", { name: "Expand" });
    fireEvent.click(expandButton);

    expect(
      within(resolvedRunHistorySection).getByRole("button", { name: "Collapse" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("true");
    expect(within(resolvedRunHistorySection).getByText("Active Story")).toBeTruthy();
    expect(within(resolvedRunHistorySection).getByText("Rejected Story")).toBeTruthy();
    expect(within(resolvedRunHistorySection).getAllByText("dispatch-review-active").length).toBeGreaterThan(
      0,
    );
    expect(within(resolvedActiveWorkSection).getByText("Active Story")).toBeTruthy();

    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Collapse" }));

    expect(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("false");
    expect(screen.queryByText("Rejected Story")).toBeNull();
    expect(within(resolvedActiveWorkSection).getByText("Active Story")).toBeTruthy();
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
    const resolvedRunHistorySection = requireValue(runHistorySection, "expected run history section");
    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));

    const sessionLogLink = within(resolvedRunHistorySection).getByRole("link", {
      name: "Codex session log",
    });
    expect(sessionLogLink.getAttribute("href")).toBe(
      "file:///C:/Users/operator/codex/sess-jsonl.jsonl",
    );
    expect(within(resolvedRunHistorySection).getByText(/codex \/ session_id \/ sess-jsonl/)).toBeTruthy();
    expect(within(resolvedRunHistorySection).queryByText("Session log unavailable")).toBeNull();
  });

  it("falls back to secondary provider metadata when no explicit session log exists", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
      />,
    );

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    const resolvedRunHistorySection = requireValue(runHistorySection, "expected run history section");
    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));

    expect(within(resolvedRunHistorySection).getAllByText("Session log unavailable").length).toBeGreaterThan(
      0,
    );
    expect(
      within(resolvedRunHistorySection).getByText(/codex \/ session_id \/ sess-rejected-story/),
    ).toBeTruthy();
    expect(within(resolvedRunHistorySection).queryByRole("link", { name: "Codex session log" })).toBeNull();
  });

  it("renders selected workstation empty active-work guidance with compact counts", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.implement;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) => attempt.transition_id === selectedNode.transition_id,
    );

    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={resolvedProviderSessions}
        selectedNode={selectedNode}
      />,
    );

    const activeWorkSection = screen.getByRole("heading", { name: "Active work" }).closest("section");
    const resolvedActiveWorkSection = requireValue(activeWorkSection, "expected active work section");
    expect(
      within(resolvedActiveWorkSection).getByText("No active work is running on this workstation."),
    ).toBeTruthy();

    const summarySection = screen.getByRole("heading", { name: "Workstation summary" }).closest(
      "section",
    );
    const resolvedSummarySection = requireValue(summarySection, "expected workstation summary section");
    expect(within(resolvedSummarySection).getByText("Active runs")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("Historical runs")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("0")).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("1")).toBeTruthy();

    const runHistorySection = screen.getByRole("heading", { name: "Run history" }).closest("section");
    const resolvedRunHistorySection = requireValue(runHistorySection, "expected run history section");
    expect(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("false");

    fireEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));

    expect(within(resolvedRunHistorySection).getByText("Retry Story")).toBeTruthy();
    expect(within(resolvedRunHistorySection).getByText("Session log unavailable")).toBeTruthy();
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

    const resolvedActiveExecution = requireValue(
      activeExecution,
      "expected active workstation execution fixture",
    );
    const resolvedProviderSessions = requireValue(
      providerSessions,
      "expected workstation provider sessions fixture",
    );

    render(
      <WorkstationDetailCard
        activeExecutions={[resolvedActiveExecution]}
        now={DETAIL_CARD_NOW}
        providerSessions={resolvedProviderSessions}
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
