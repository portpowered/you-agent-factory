import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { formatLocalDateTime } from "../../../../components/ui/formatters";
import { describe, expect, it, vi } from "vitest";
import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import type {
  EditableWorkStateConfigurationState,
  EditableWorkStateSaveState,
} from "../lib/detail-card-types";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";
import { EditableWorkStateSaveHeaderAction } from "./work-state-save-controls";
import { StateNodeDetailCard } from "./state-node-detail";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

describe("StateNodeDetailCard", () => {
  const activeStoryStartedAt = "2026-04-08T12:00:01Z";
  const doneStoryStartedAt = "2026-04-08T12:00:06Z";
  const failedStoryStartedAt = "2026-04-08T12:00:08Z";

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
            started_at: activeStoryStartedAt,
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
    expect(
      screen.getByText(
        `Started at ${formatLocalDateTime(activeStoryStartedAt, "Unavailable")}`,
      ),
    ).toBeTruthy();
    const startedAtTime = requireValue(
      screen.getByText(/^Started at /).closest("time"),
      "expected started-at time element",
    );
    expect(startedAtTime.getAttribute("dateTime")).toBe(activeStoryStartedAt);
    expect(startedAtTime.getAttribute("title")).toBe(activeStoryStartedAt);
    expect(screen.queryByText("trace-active-story")).toBeNull();
    expect(screen.queryByText("story")).toBeNull();
  });

  it("omits the supporting time row when current work has no started-at timestamp", () => {
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
    expect(screen.queryByText(/^Started at /)).toBeNull();
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
            started_at: doneStoryStartedAt,
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
    expect(
      screen.getByText(
        `Started at ${formatLocalDateTime(doneStoryStartedAt, "Unavailable")}`,
      ),
    ).toBeTruthy();
    expect(screen.queryByText("trace-done-story")).toBeNull();
    expect(screen.queryByText(/^story$/)).toBeNull();
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
            started_at: failedStoryStartedAt,
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
    expect(
      screen.getByText(
        `Started at ${formatLocalDateTime(failedStoryStartedAt, "Unavailable")}`,
      ),
    ).toBeTruthy();
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

    expect(screen.queryByText("状态")).toBeNull();
    expect(screen.queryByText("状态节点 ID")).toBeNull();
    expect(screen.getByText("当前工作")).toBeTruthy();
    expect(screen.getByText("在所选时间刻度，这个位置暂时没有记录到工作。")).toBeTruthy();
  });

  it("renders Started at with the same canonical formatter output as dispatch history for zh-CN", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");

    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <StateNodeDetailCard
          currentWorkItems={[
            {
              display_name: "Active Story",
              started_at: activeStoryStartedAt,
              work_id: "work-active-story",
            },
          ]}
          place={resolvedSelectedState}
          tokenCount={1}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByText(
        `开始时间 ${formatLocalDateTime(activeStoryStartedAt, "不可用", "zh-CN")}`,
      ),
    ).toBeTruthy();
  });
});

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [],
    workstations: [],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "implemented", type: "PROCESSING" },
          { name: "complete", type: "TERMINAL" },
        ],
      },
    ],
    ...overrides,
  };
}

function buildWorkStateHeaderSaveAction({
  canSave,
  onClick = vi.fn(),
  saveState = { status: "idle" },
}: {
  canSave: boolean;
  onClick?: () => void;
  saveState?: EditableWorkStateSaveState;
}) {
  return (
    <EditableWorkStateSaveHeaderAction
      canSave={canSave}
      onClick={onClick}
      saveState={saveState}
    />
  );
}

describe("StateNodeDetailCard editable work state configuration", () => {
  const messages = getWorkStateDetailMessages();

  it("renders editable name and read-only lifecycle type when configuration state is ready", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );
    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");
    const editableConfigurationState: EditableWorkStateConfigurationState = {
      baseVersion: buildFactoryDocument().version,
      canSave: false,
      draft: {
        name: "implemented",
        type: "PROCESSING",
      },
      hasValidationErrors: false,
      initialValues: {
        stateName: "implemented",
        stateNamesInWorkType: ["implemented", "complete"],
        stateType: "PROCESSING",
        workTypeName: "story",
      },
      isDirty: false,
      markChangesSaved: vi.fn(),
      onNameChange: vi.fn(),
      onResetToLatest: vi.fn(),
      originalStateName: "implemented",
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
      workTypeName: "story",
    };

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkStateHeaderSaveAction({ canSave: false })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    expect(screen.getByLabelText(messages.nameFieldLabel)).toBeTruthy();
    expect(screen.getByText(messages.localizeWorkStateType("PROCESSING"))).toBeTruthy();
    expect(screen.queryByText("story: implemented")).toBeNull();
    expect(screen.getByText(messages.editableConfigurationHeading)).toBeTruthy();
  });

  it("shows duplicate-name validation with aria-invalid and role alert", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );
    const resolvedSelectedState = requireValue(selectedState, "expected implemented state fixture");
    const duplicateMessage = messages.editableConfigurationNameDuplicate("complete");

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        editableConfigurationState={{
          baseVersion: buildFactoryDocument().version,
          canSave: false,
          draft: {
            name: "complete",
            type: "PROCESSING",
          },
          hasValidationErrors: true,
          initialValues: {
            stateName: "implemented",
            stateNamesInWorkType: ["implemented", "complete"],
            stateType: "PROCESSING",
            workTypeName: "story",
          },
          isDirty: true,
          markChangesSaved: vi.fn(),
          onNameChange: vi.fn(),
          onResetToLatest: vi.fn(),
          originalStateName: "implemented",
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {
            name: duplicateMessage,
          },
          workTypeName: "story",
        }}
        headerAction={buildWorkStateHeaderSaveAction({ canSave: false })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    const nameInput = screen.getByLabelText(messages.nameFieldLabel);
    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(duplicateMessage)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.editableConfigurationSaveAction })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("keeps runtime work list content when editable configuration is ready", () => {
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
            work_id: "work-active-story",
          },
        ]}
        editableConfigurationState={{
          baseVersion: buildFactoryDocument().version,
          canSave: false,
          draft: {
            name: "implemented",
            type: "PROCESSING",
          },
          hasValidationErrors: false,
          initialValues: {
            stateName: "implemented",
            stateNamesInWorkType: ["implemented", "complete"],
            stateType: "PROCESSING",
            workTypeName: "story",
          },
          isDirty: false,
          markChangesSaved: vi.fn(),
          onNameChange: vi.fn(),
          onResetToLatest: vi.fn(),
          originalStateName: "implemented",
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
          workTypeName: "story",
        }}
        place={resolvedSelectedState}
        tokenCount={1}
      />,
    );

    expect(screen.getByText("Current work")).toBeTruthy();
    expect(screen.getByText("Active Story")).toBeTruthy();
  });
});
