import "../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { formatLocalDateTime } from "../../../../components/ui/formatters";
import { CurrentSelectionLocaleProvider } from "../../base/components/presentation/current-selection-locale";
import type {
  EditableWorkStateConfigurationState,
  EditableWorkStateSaveState,
} from "../lib/detail-card-types";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";
import { StateNodeDetailCard } from "./state-node-detail";

const CURRENT_SELECTION_FORM_FIELDS_SELECTOR = ".grid.grid-cols-1.gap-3";

import { EditableWorkStateConfigurationHeaderActions } from "./work-state-save-controls";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function sectionByHeading(name: string): HTMLElement {
  const heading = screen.getByRole("heading", { name });
  const section = heading.closest("section");
  if (!section) {
    throw new Error(`expected ${name} section`);
  }

  return section;
}

describe("StateNodeDetailCard", () => {
  const activeStoryStartedAt = "2026-04-08T12:00:01Z";
  const doneStoryStartedAt = "2026-04-08T12:00:06Z";
  const failedStoryStartedAt = "2026-04-08T12:00:08Z";

  it("renders selected state node detail with current work item references", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

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

    const summarySection = sectionByHeading("Summary");
    const summaryDetails = within(summarySection)
      .getByText("Count")
      .closest("dl");

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText("story: implemented").className).toContain(
      "type-display-large",
    );
    expect(
      within(summarySection)
        .getByRole("button", { name: "Collapse" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(summaryDetails).toBeTruthy();
    expect(
      within(
        requireValue(summaryDetails, "expected summary details"),
      ).getByText("Work type"),
    ).toBeTruthy();
    expect(within(summarySection).getByText("story")).toBeTruthy();
    expect(
      within(
        requireValue(summaryDetails, "expected summary details"),
      ).getByText("State"),
    ).toBeTruthy();
    expect(within(summarySection).getByText("implemented")).toBeTruthy();
    expect(
      within(
        requireValue(summaryDetails, "expected summary details"),
      ).getByText("State node ID"),
    ).toBeTruthy();
    expect(within(summarySection).getByText("story:implemented")).toBeTruthy();
    expect(within(summarySection).getByText("Count")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
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
  });

  it("omits the supporting time row when current work has no started-at timestamp", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

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

    const summarySection = sectionByHeading("Summary");
    const summaryDetails = within(summarySection)
      .getByText("Count")
      .closest("dl");

    expect(summaryDetails).toBeTruthy();
    expect(
      within(
        requireValue(summaryDetails, "expected summary details"),
      ).getByText("Work type"),
    ).toBeTruthy();
    expect(screen.queryByText(/^Started at /)).toBeNull();
  });

  it("renders the state-node selection title with canonical typography", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    const title = screen.getByText("story: implemented");
    expect(title.className).toContain("type-display-large");
    expect(screen.getByRole("heading", { name: "Summary" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
  });

  it("renders selected state node empty-position guidance", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText("story: implemented")).toBeTruthy();
    expect(sectionByHeading("Summary")).toBeTruthy();
    expect(screen.getByText("State")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
    expect(screen.queryByText("Token count")).toBeNull();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
    expect(
      screen.getByText("No current work is occupying this place."),
    ).toBeTruthy();
  });

  it("renders selected terminal state node detail from terminal-history occupancy", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected terminal state fixture",
    );

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

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText("story: complete")).toBeTruthy();
    expect(screen.getByText("State node ID")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
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
    expect(
      screen.queryByText("No current work is occupying this place."),
    ).toBeNull();
  });

  it("renders failed terminal state diagnostics from retained failed-work details", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.implement.output_places?.find(
        (place) => place.place_id === "story:blocked",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected failed state fixture",
    );

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        failedWorkDetailsByWorkID={{
          "work-failed-story": {
            dispatch_id: "dispatch-failed-story",
            failure_message:
              "Provider rate limit exceeded while generating the repair.",
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
    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
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
    expect(
      screen.getByText(
        "Provider rate limit exceeded while generating the repair.",
      ),
    ).toBeTruthy();
  });

  it("distinguishes empty terminal state positions from unavailable terminal history", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected terminal state fixture",
    );

    const { rerender } = render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    expect(
      screen.getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/terminal history/i)).toBeNull();

    rerender(
      <StateNodeDetailCard
        currentWorkItems={[]}
        place={resolvedSelectedState}
        tokenCount={1}
      />,
    );

    expect(
      screen.getByText(
        "Represented work is unavailable for this place at the selected tick.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/terminal history/i)).toBeNull();
  });

  it("calls the selection callback when a listed work item is clicked", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const onSelectWorkItem = vi.fn();

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

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

    fireEvent.click(
      screen.getByRole("button", { name: "Select work item Active Story" }),
    );

    expect(onSelectWorkItem).toHaveBeenCalledWith({
      display_name: "Active Story",
      trace_id: "trace-active-story",
      work_id: "work-active-story",
      work_type_id: "story",
    });
  });

  it("renders state-node supporting copy from the zh-CN locale catalog", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected terminal state fixture",
    );

    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <StateNodeDetailCard
          currentWorkItems={[]}
          place={resolvedSelectedState}
          tokenCount={0}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("状态")).toBeTruthy();
    expect(screen.getByText("状态节点 ID")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "当前工作" })).toBeTruthy();
    expect(
      screen.getByText("在所选时间刻度，这个位置暂时没有记录到工作。"),
    ).toBeTruthy();
  });

  it("renders Started at with the same canonical formatter output as dispatch history for zh-CN", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

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

function workStateDetailHeaderActionSection() {
  const card = screen.getByRole("article", { name: "Current selection" });
  const undoButton = within(card).getByRole("button", {
    name: "Undo selection",
  });
  const actionSection = undoButton.closest(
    "[data-action-row-section='actions']",
  );
  if (!actionSection) {
    throw new Error("expected header action section");
  }

  return actionSection as HTMLElement;
}

function editableWorkStateConfigurationForm() {
  const panel = screen.getByRole("article", { name: "Current selection" });
  const nameInput = within(panel).getByLabelText(
    getWorkStateDetailMessages().nameFieldLabel,
  );
  const form = nameInput.closest("form");
  if (!form) {
    throw new Error("expected editable work state configuration form");
  }

  return form;
}

function buildWorkStateHeaderActions({
  canDiscard = false,
  canSave,
  onDiscard = vi.fn(),
  onSave = vi.fn(),
  saveState = { status: "idle" },
}: {
  canDiscard?: boolean;
  canSave: boolean;
  onDiscard?: () => void;
  onSave?: () => void;
  saveState?: EditableWorkStateSaveState;
}) {
  return (
    <EditableWorkStateConfigurationHeaderActions
      canDiscard={canDiscard}
      canSave={canSave}
      onDiscard={onDiscard}
      onSave={onSave}
      saveState={saveState}
    />
  );
}

describe("StateNodeDetailCard editable work state configuration", () => {
  const messages = getWorkStateDetailMessages();

  it("renders editable name and read-only lifecycle type when configuration state is ready", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );
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
        headerAction={buildWorkStateHeaderActions({ canSave: false })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    expect(screen.getByLabelText(messages.nameFieldLabel)).toBeTruthy();
    expect(
      screen.getByText(messages.localizeWorkStateType("PROCESSING")),
    ).toBeTruthy();
    expect(screen.getByText("story: implemented").className).toContain(
      "type-display-large",
    );
    expect(
      screen.getByText(messages.editableConfigurationHeading),
    ).toBeTruthy();
  });

  it("shows duplicate-name validation with aria-invalid and role alert", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );
    const duplicateMessage =
      messages.editableConfigurationNameDuplicate("complete");

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
        headerAction={buildWorkStateHeaderActions({ canSave: false })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    const nameInput = screen.getByLabelText(messages.nameFieldLabel);
    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(duplicateMessage)).toBeTruthy();
    const headerActions = workStateDetailHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: messages.editableConfigurationSaveAction,
    });
    expect(saveButtons).toHaveLength(1);
    expect(saveButtons[0]?.getAttribute("disabled")).not.toBeNull();
  });

  it("stacks configuration fields vertically and renders header save and discard only when dirty", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );
    const onSave = vi.fn();
    const onDiscard = vi.fn();

    const { container } = render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        editableConfigurationState={{
          baseVersion: buildFactoryDocument().version,
          canSave: true,
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
          isDirty: true,
          markChangesSaved: vi.fn(),
          onNameChange: vi.fn(),
          onResetToLatest: vi.fn(),
          originalStateName: "implemented",
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
          workTypeName: "story",
        }}
        headerAction={buildWorkStateHeaderActions({
          canDiscard: true,
          canSave: true,
          onDiscard,
          onSave,
        })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    const fieldGroup = container.querySelector(
      CURRENT_SELECTION_FORM_FIELDS_SELECTOR,
    );
    expect(fieldGroup).not.toBeNull();
    expect(fieldGroup?.className).not.toMatch(/sm:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/md:grid-cols-\d/);

    const headerActions = workStateDetailHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: messages.editableConfigurationSaveAction,
    });
    const discardButtons = within(headerActions).getAllByRole("button", {
      name: messages.discardDraftAction,
    });
    expect(saveButtons).toHaveLength(1);
    expect(discardButtons).toHaveLength(1);

    fireEvent.click(saveButtons[0]);
    expect(onSave).toHaveBeenCalledTimes(1);

    fireEvent.click(discardButtons[0]);
    expect(onDiscard).toHaveBeenCalledTimes(1);

    expect(
      within(editableWorkStateConfigurationForm()).queryByRole("button", {
        name: messages.editableConfigurationSaveAction,
      }),
    ).toBeNull();
    expect(
      within(editableWorkStateConfigurationForm()).queryByRole("button", {
        name: messages.discardDraftAction,
      }),
    ).toBeNull();
  });

  it("omits global unsaved helper paragraphs for dirty ready-state work state drafts", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        editableConfigurationState={{
          baseVersion: buildFactoryDocument().version,
          canSave: true,
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
          isDirty: true,
          markChangesSaved: vi.fn(),
          onNameChange: vi.fn(),
          onResetToLatest: vi.fn(),
          originalStateName: "implemented",
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
          workTypeName: "story",
        }}
        headerAction={buildWorkStateHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        place={resolvedSelectedState}
        tokenCount={0}
      />,
    );

    expect(
      screen.queryByText("You have unsaved changes for this work state."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Changes stay local to this edit session until you save the running factory.",
      ),
    ).toBeNull();
    expect(
      screen.getAllByRole("button", {
        name: messages.editableConfigurationSaveAction,
      }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: messages.discardDraftAction }),
    ).toBeTruthy();
  });

  it("keeps runtime work list content when editable configuration is ready", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedState =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const resolvedSelectedState = requireValue(
      selectedState,
      "expected implemented state fixture",
    );

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

    expect(screen.getByRole("heading", { name: "Current work" })).toBeTruthy();
    expect(screen.getByText("Active Story")).toBeTruthy();
  });
});
