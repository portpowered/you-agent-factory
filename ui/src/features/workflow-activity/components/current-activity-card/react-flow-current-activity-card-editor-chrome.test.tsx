// @component-test-runner vitest: components package declarations contain relative imports Bun cannot execute.
import "../../../../testing/vitest-dom-capabilities.setup";

import "@testing-library/jest-dom/vitest";
import "./test-support/react-flow-current-activity-card-component.mocks";

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  buildMockGraphSavePayload,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
  wireMockEditableFactoryGraph,
  workerDenseFactoryDefinitionDocument,
} from "../../../../testing/graph-editor-harness";
import { selectLabeledComboboxOption } from "../../../../testing/select-test-helpers";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useFactoryGraphDraftState } from "../../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { useEditableFactoryGraph } from "../../../factory-graph-editor/hooks/use-editable-factory-graph";
import { removeFactoryGraphNode } from "../../../factory-graph-editor/lib/operations/factory-graph-operations";
import {
  dashboardSnapshotWithActiveWorkItemCount,
  dashboardSnapshotWithEditableFactory,
  defaultDraftState,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
  workerDenseSnapshot,
} from "./test-support/react-flow-current-activity-card-component.harness";

describe("ReactFlowCurrentActivityCard editor chrome", () => {
  registerCurrentActivityCardTestLifecycle();

  it("uses the shared observer renderer until graph editor mode is enabled", () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    expect(screen.getByRole("button", { name: "Edit mode" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Factory graph editor tools" }),
    ).toBeNull();
    expect(screen.getByText("Observe")).toBeTruthy();
  });

  it("shows the add and delete toolbar in editor mode", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(within(toolbar).getByRole("button", { name: "Add" })).toBeTruthy();
    expect(
      within(toolbar).getByRole("button", {
        name: "Delete, no graph items selected",
      }),
    ).toBeTruthy();
    expect(screen.queryByText("Editor mode active")).toBeNull();
  });

  it("keeps classifier workstations out of the editable graph flow", async () => {
    const snapshot = dashboardSnapshotWithActiveWorkItemCount(0);
    snapshot.topology.workstation_nodes_by_id.review.workstation_kind =
      "CLASSIFIER_WORKSTATION";
    snapshot.factory = structuredClone(baseFactoryDefinition);
    const reviewWorkstation = snapshot.factory?.workstations?.find(
      (workstation) => workstation.name === "review",
    );
    if (reviewWorkstation) {
      reviewWorkstation.type = "CLASSIFIER_WORKSTATION";
      reviewWorkstation.classificationRoutes = [
        {
          label: "approved",
          output: {
            state: "complete",
            workType: "story",
          },
        },
      ];
    }

    renderCurrentActivity({
      currentFactoryDocument: null,
      currentFactoryDocumentStatus: "pending",
      snapshot,
    });

    const unavailableEditorButton = screen.getByRole("button", {
      name: 'Factory graph editing does not yet support classifier workstation routes. "review" stays read-only in this view until labeled route editing is available.',
    });
    expect(unavailableEditorButton.getAttribute("disabled")).not.toBeNull();
    expect(
      screen.getByText(
        'Editor unavailable: Factory graph editing does not yet support classifier workstation routes. "review" stays read-only in this view until labeled route editing is available.',
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Factory graph editor tools" }),
    ).toBeNull();
    expect(screen.queryByText("Editor mode active")).toBeNull();
  });

  it("lists the supported add-entity options and validates duplicate worker names before mutating the draft", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add" }));

    expect(screen.getByRole("button", { name: "Workstation" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Worker" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work type" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work state" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Resource" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Worker" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "writer" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      screen.getByText('A worker named "writer" already exists in the draft.'),
    ).toBeTruthy();
    expect(replaceDraft).not.toHaveBeenCalled();
  });

  it("submits valid add-entity forms into the pending graph draft", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
    fireEvent.click(screen.getByRole("button", { name: "Work type" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "essay" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "First state" }), {
      target: { value: "queued" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(replaceDraft).toHaveBeenCalledTimes(1);
    const nextDraft = replaceDraft.mock.calls[0]?.[0];
    expect(nextDraft.additions.workTypes).toEqual([
      {
        name: "essay",
        states: [
          {
            name: "queued",
            type: "INITIAL",
          },
        ],
      },
    ]);
  });

  it("distinguishes work-state creation from work-type creation and blocks missing work-type association", async () => {
    const replaceDraft = vi.fn();
    const user = userEvent.setup();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
    fireEvent.click(screen.getByRole("button", { name: "Work state" }));

    expect(screen.getByText("Add work state")).toBeTruthy();
    expect(
      screen.getByText("Append a new ordered state to an existing work type."),
    ).toBeTruthy();
    expect(
      screen.getByRole("combobox", { name: "Work type" }),
    ).toHaveTextContent("story");
    expect(
      screen.getByRole("combobox", { name: "State type" }),
    ).toHaveTextContent("PROCESSING");

    await selectLabeledComboboxOption(user, "Work type", "Select a work type");
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      screen.getByText("Choose a work type before adding a work state."),
    ).toBeTruthy();
    expect(replaceDraft).not.toHaveBeenCalled();
  });

  it("submits valid work-state add-entity forms into the pending graph draft", async () => {
    const replaceDraft = vi.fn();
    const user = userEvent.setup();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithEditableFactory(),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
    fireEvent.click(screen.getByRole("button", { name: "Work state" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "approved" },
    });
    await selectLabeledComboboxOption(user, "State type", "TERMINAL");
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(replaceDraft).toHaveBeenCalledTimes(1);
    const nextDraft = replaceDraft.mock.calls[0]?.[0];

    expect(nextDraft.additions.workStates).toEqual([
      {
        state: {
          name: "approved",
          type: "TERMINAL",
        },
        workTypeName: "story",
      },
    ]);
  }, 30_000);

  it("renders document-only workstations in edit mode when the snapshot plane diverges", async () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();
    wireMockEditableFactoryGraph(
      {
        useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
        useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
      },
      createMockGraphEditorDraftState({
        baseDocument: divergentDocumentPlaneFactoryDocument,
        latestDocument: divergentDocumentPlaneFactoryDocument,
        pendingFactoryDefinition: divergentDocumentPlaneFactoryDocument,
      }),
    );

    renderCurrentActivity({
      currentFactoryDocument: divergentDocumentPlaneFactoryDocument,
      snapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: "Select Document Only workstation",
        }),
      ).toBeTruthy();
    });
    expect(
      screen.queryByRole("button", {
        name: "Select Snapshot Only workstation",
      }),
    ).toBeNull();
  });

  it("does not render snapshot-only workstations in editor mode while the factory document is loading", async () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();
    wireMockEditableFactoryGraph(
      {
        useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
        useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
      },
      createMockGraphEditorDraftState({
        baseDocument: null,
        latestDocument: null,
        pendingFactoryDefinition: null,
      }),
    );

    renderCurrentActivity({
      currentFactoryDocument: null,
      currentFactoryDocumentStatus: "pending",
      snapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    await waitFor(() => {
      expect(
        screen.getByRole("region", { name: "Factory graph editor tools" }),
      ).toBeTruthy();
    });
    expect(
      screen.queryByRole("button", {
        name: "Select Snapshot Only workstation",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Select Document Only workstation",
      }),
    ).toBeNull();
  });

  it("does not render the editor-only visibility preset controls in embedded editor mode", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);

    renderCurrentActivity({
      snapshot: workerDenseSnapshot(),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(
      await screen.findByRole("region", {
        name: "Factory graph editor tools",
      }),
    ).toBeTruthy();

    expect(screen.queryByRole("button", { name: "Infrastructure" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Workflow" })).toBeNull();
    expect(screen.queryByRole("button", { name: "All" })).toBeNull();
  });

  it("removes a workstation without opening a confirmation", () => {
    const result = removeFactoryGraphNode({
      baseFactoryDefinition: baseFactoryDefinitionDocument,
      draft: defaultDraftState.draft,
      nodeId: "workstation:review",
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      return;
    }
    expect(result.value.removals.workstations).toEqual(["review"]);
  });

  it("shows a loading editor state while the editable definition is still fetching", async () => {
    renderCurrentActivity({
      currentFactoryDocument: null,
      currentFactoryDocumentStatus: "pending",
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(screen.queryByText("Loading editor definition")).toBeNull();
    expect(
      within(toolbar)
        .getByRole("button", { name: "Add" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      within(toolbar)
        .getByRole("button", { name: "Delete, no graph items selected" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("requires save, discard, or keep editing before leaving editor mode with unsaved changes", async () => {
    const resetDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      hasChanges: true,
      resetDraft,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(screen.getByRole("button", { name: "Leave editor" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    expect(
      within(dialog).getByRole("button", { name: "Save changes" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Keep editing" }),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Discard changes" }),
    );

    expect(resetDraft).toHaveBeenCalledTimes(1);
  });

  it("keeps editor mode when choosing Keep editing on the leave dialog", async () => {
    const resetDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      hasChanges: true,
      resetDraft,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(screen.getByRole("button", { name: "Leave editor" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Keep editing" }),
    );

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(resetDraft).not.toHaveBeenCalled();
    expect(
      screen
        .getByRole("button", { name: "Leave editor" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.queryByText("Unsaved changes")).toBeNull();
  });

  it("shows explicit save and discard actions for pending graph changes", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    const actions = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(actions).getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();
    expect(
      within(actions).getByRole("button", { name: "Save changes" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Pending graph changes" }),
    ).toBeNull();
  });

  it("shows consolidated unsaved chrome with a single header status and warning toggle", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    const statusSection = document.querySelector(
      '[data-action-row-section="statuses"]',
    );
    expect(statusSection).toBeNull();
    expect(within(toolbar).queryAllByRole("status")).toHaveLength(0);

    const toggle = screen.getByRole("button", {
      name: "Leave editor",
    });
    expect(toggle.className).toContain("border-af-warning-border");
    expect(toggle.className).toContain("bg-warning-container");
    expect(toggle.className).toContain("text-on-warning-container");
  });

  it("confirms pending save changes before saving the graph draft", async () => {
    const saveAsync = vi.fn().mockResolvedValue(baseFactoryDefinitionDocument);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryDocumentSave).mockReturnValue({
      error: null,
      isPending: false,
      reset: vi.fn(),
      save: vi.fn(),
      saveAsync,
    } as never);
    const draftWithReviewer = {
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    };
    vi.mocked(useFactoryGraphDraftState).mockReturnValue(
      draftWithReviewer as never,
    );

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(
      within(
        await screen.findByRole("region", {
          name: "Factory graph editor tools",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Save factory graph changes?",
    });
    expect(
      within(dialog).getByText("This save will apply 1 created entity."),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(saveAsync).toHaveBeenCalledWith(
        buildMockGraphSavePayload(draftWithReviewer),
      );
    });
  });

  it("warns when a newer editable-definition version arrives during a dirty draft", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      baseDocument: baseFactoryDefinitionDocument,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
      latestDocument: {
        ...baseFactoryDefinitionDocument,
        version: {
          logical: "9",
          physical: "2026-05-19T01:45:00Z",
        },
      },
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(
      await screen.findByText("A newer factory definition is available"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("blocks topology save while active work is still running", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(1),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(await screen.findByText("Topology edits are blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Save is unavailable while active work is still running in the current factory.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("saves the pending editable definition before leaving editor mode", async () => {
    const saveAsync = vi.fn().mockResolvedValue(baseFactoryDefinitionDocument);
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryDocumentSave).mockReturnValue({
      error: null,
      isPending: false,
      reset: vi.fn(),
      save: vi.fn(),
      saveAsync,
    } as never);
    const dirtyDraftState = {
      ...defaultDraftState,
      hasChanges: true,
      replaceDraft,
    };
    vi.mocked(useFactoryGraphDraftState).mockReturnValue(
      dirtyDraftState as never,
    );

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
    fireEvent.click(screen.getByRole("button", { name: "Leave editor" }));
    fireEvent.click(
      within(
        await screen.findByRole("dialog", {
          name: "Leave graph editor with unsaved changes?",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );

    await waitFor(() => {
      expect(saveAsync).toHaveBeenCalledWith(
        buildMockGraphSavePayload(dirtyDraftState),
      );
    });
    expect(replaceDraft).toHaveBeenCalledTimes(1);
  });
});
