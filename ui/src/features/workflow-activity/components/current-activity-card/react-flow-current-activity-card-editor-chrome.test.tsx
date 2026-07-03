import "@testing-library/jest-dom/vitest";
import "./react-flow-current-activity-card-component.mocks";

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
  currentFactoryDocumentFromSnapshot,
  dashboardSnapshotWithActiveWorkItemCount,
  dashboardSnapshotWithEditableFactory,
  defaultDraftState,
  refreshFactoryFromTopology,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
  workerDenseSnapshot,
} from "./react-flow-current-activity-card-component.harness";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: editor chrome scenarios share one current deployment harness seam.
describe("ReactFlowCurrentActivityCard editor chrome", () => {
  registerCurrentActivityCardTestLifecycle();

  it("keeps editor controls unavailable until the graph editor mode is enabled", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    expect(screen.getByRole("button", { name: "Edit mode" })).toBeTruthy();
    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar).getByRole("button", {
        name: "Show or hide",
      }),
    ).toBeTruthy();
    expect(within(toolbar).queryByRole("button", { name: "Add" })).toBeNull();
    expect(screen.queryByText("Observe")).toBeNull();
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
      screen.queryByText(
        'Editor unavailable: Factory graph editing does not yet support classifier workstation routes. "review" stays read-only in this view until labeled route editing is available.',
      ),
    ).toBeNull();

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar).getByRole("button", {
        name: "Show or hide",
      }),
    ).toBeTruthy();
    expect(within(toolbar).queryByRole("button", { name: "Add" })).toBeNull();
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

  it("keeps editor mode on the shared observer graph surface", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      graph: {
        edges: [],
        nodes: [
          {
            id: "worker:writer",
            key: { kind: "worker", name: "writer" },
            kind: "worker",
            label: "writer",
          },
          {
            id: "workstation:review",
            key: { kind: "workstation", name: "review" },
            kind: "workstation",
            label: "review",
          },
        ],
      },
      hasChanges: true,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workstations: [
            {
              inputs: [],
              name: "review",
              outputs: [],
              type: "MODEL_WORKSTATION",
              worker: "writer",
            },
          ],
        },
      },
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });
    expect(screen.queryByText("Pending")).toBeNull();
  });

  it("renders worker and resource nodes from the canonical snapshot factory in observer mode", async () => {
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
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });

    await waitFor(() => {
      expect(document.querySelector('[data-id="worker:writer"]')).toBeTruthy();
      expect(
        document.querySelector('[data-id="worker:reviewer"]'),
      ).toBeTruthy();
      expect(document.querySelector('[data-id="worker:stalled"]')).toBeTruthy();
      expect(document.querySelector('[data-id="resource:gpu"]')).toBeTruthy();
    });
  });

  it("renders event snapshot workstations in observe mode while the factory document is pending", async () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();
    if (snapshot.factory) {
      snapshot.factory.workstations = snapshot.factory.workstations?.filter(
        (workstation) => workstation.name !== "Document Only",
      );
    }
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

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("button", {
        name: "Select Snapshot Only workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Select Document Only workstation",
      }),
    ).toBeNull();
  });

  it("renders nested bundled docs as observe-mode graph nodes from the saved factory document", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    refreshFactoryFromTopology(snapshot);
    const nestedDocPath = "factory/docs/standards/review.md";
    const savedDocument = {
      ...currentFactoryDocumentFromSnapshot(snapshot),
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "# Review standards" },
            targetPath: nestedDocPath,
            type: "DOC",
          },
        ],
      },
    };
    snapshot.factory = savedDocument;

    renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Select review.md doc" }),
      ).toBeTruthy();
    });
    expect(screen.getByText(nestedDocPath)).toBeTruthy();
  });

  it("renders bundled docs as observe-mode graph nodes from the saved factory document", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    refreshFactoryFromTopology(snapshot);
    const savedDocument = {
      ...currentFactoryDocumentFromSnapshot(snapshot),
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "# Planning" },
            targetPath: "factory/docs/planning.md",
            type: "DOC",
          },
        ],
      },
    };
    snapshot.factory = savedDocument;

    renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Select overview.md doc" }),
      ).toBeTruthy();
    });
    expect(
      screen.getByRole("button", { name: "Select planning.md doc" }),
    ).toBeTruthy();
  });

  it("renders event snapshot workstations in observe mode when the document plane diverges from the snapshot", async () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();
    if (snapshot.factory) {
      snapshot.factory.workstations = snapshot.factory.workstations?.filter(
        (workstation) => workstation.name !== "Document Only",
      );
    }
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

    expect(
      await screen.findByRole("button", {
        name: "Select Snapshot Only workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Select Document Only workstation",
      }),
    ).toBeNull();
  });

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

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Infrastructure" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Workflow" })).toBeNull();
    expect(screen.queryByRole("button", { name: "All" })).toBeNull();
  });

  it("renders supported workstation and work-state editor handles on the shared observer graph", async () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const document = currentFactoryDocumentFromSnapshot(snapshot);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: document,
      error: null,
      status: "success",
    } as never);
    wireMockEditableFactoryGraph(
      {
        useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
        useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
      },
      createMockGraphEditorDraftState({
        baseDocument: document,
        latestDocument: document,
        pendingFactoryDefinition: document,
      }),
    );

    renderCurrentActivity({
      snapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(
      await screen.findAllByLabelText(
        "Route successful output from this workstation.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Accept an input work state for this workstation.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Route this work state into a workstation input.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Receive workstation output into this work state.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Route successful output from this workstation.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText("Assign this worker to a workstation."),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Accept a worker assignment for this workstation.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText(
        "Accept a resource requirement for this workstation.",
      ),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByLabelText("Provide this resource to a workstation."),
    ).not.toHaveLength(0);
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

  it("keeps worker nodes visible but not workstation-style deletion targets", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithEditableFactory(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select writer worker",
      }),
    ).toBeTruthy();
    expect(await screen.findByLabelText("worker:writer")).toBeTruthy();
    expect(
      await screen.findByRole("button", {
        name: /Select .* workstation/,
      }),
    ).toBeTruthy();

    const removeWorker = removeFactoryGraphNode({
      baseFactoryDefinition: baseFactoryDefinitionDocument,
      draft: defaultDraftState.draft,
      nodeId: "worker:writer",
    });
    expect(removeWorker).toMatchObject({
      ok: false,
      reason: "BLOCKED_REMOVAL",
    });
  });

  it("keeps removed server-backed workstations visible with a pending-removal badge", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        removals: {
          ...defaultDraftState.draft.removals,
          workstations: ["review"],
        },
      },
      graph: {
        edges: [],
        nodes: [],
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(
      await screen.findByRole("button", {
        name: "Select Review workstation",
      }),
    ).toBeTruthy();
    const toggle = screen.getByRole("button", {
      name: "Leave editor",
    });
    expect(toggle.className).toContain("border-af-warning-border");
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
      '[data-dashboard-action-row-section="statuses"]',
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
