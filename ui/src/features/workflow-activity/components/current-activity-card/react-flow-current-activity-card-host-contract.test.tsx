// @component-test-runner vitest: components package declarations contain relative imports Bun cannot execute.
import "../../../../testing/vitest-dom-capabilities.setup";
import "@testing-library/jest-dom/vitest";
import "./test-support/react-flow-current-activity-card-component.mocks";

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  buildMockGraphSavePayload,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
  wireMockEditableFactoryGraph,
} from "../../../../testing/graph-editor-harness";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useFactoryGraphDraftState } from "../../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { useEditableFactoryGraph } from "../../../factory-graph-editor/hooks/use-editable-factory-graph";
import {
  dashboardSnapshotWithActiveWorkItemCount,
  defaultDraftState,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
} from "./test-support/react-flow-current-activity-card-component.harness";

describe("ReactFlowCurrentActivityCard host contracts", () => {
  registerCurrentActivityCardTestLifecycle();

  it("switches from the observer surface to the graph editor", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    expect(
      screen.getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory graph editor tools" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    expect(
      await screen.findByRole("region", {
        name: "Factory graph editor tools",
      }),
    ).toBeTruthy();
  });

  it("keeps classifier workstations read-only", () => {
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
          output: { state: "complete", workType: "story" },
        },
      ];
    }

    renderCurrentActivity({
      currentFactoryDocument: null,
      currentFactoryDocumentStatus: "pending",
      snapshot,
    });

    expect(
      screen
        .getByRole("button", {
          name: /Factory graph editing does not yet support classifier workstation routes/,
        })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      screen.getByRole("region", { name: "Factory graph editor tools" }),
    ).toBeTruthy();
  });

  it("renders the editable document plane instead of snapshot-only workstations", async () => {
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

    expect(
      await screen.findByRole("button", {
        name: "Select Document Only workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Select Snapshot Only workstation",
      }),
    ).toBeNull();
  });
});

describe("ReactFlowCurrentActivityCard host mutation contracts", () => {
  registerCurrentActivityCardTestLifecycle();

  it("disables graph mutations while the editable definition is loading", async () => {
    renderCurrentActivity({
      currentFactoryDocument: null,
      currentFactoryDocumentStatus: "pending",
      snapshot: semanticWorkflowDashboardSnapshot,
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
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

  it("routes confirmed graph saves through the document save boundary", async () => {
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
              type: "MODEL_WORKER" as const,
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
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(saveAsync).toHaveBeenCalledWith(
        buildMockGraphSavePayload(draftWithReviewer),
      );
    });
  });
});
