// biome-ignore lint/style/noExcessiveLinesPerFile: graph-draft conflict notification regressions share one mocked save/notify harness.
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import type { CurrentFactoryDocument } from "../../../../../api/current-factory-definition";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { settleCurrentSelectionEffects } from "../../../../../testing/current-selection-test-utils";
import { bunVi as vi } from "../../../../../testing/bun/vi-compat";
import { selectLabeledComboboxOption } from "../../../../../testing/select-test-helpers";
import { useStrictConsoleGuard } from "../../../../../testing/strict-console-guard";
import { useFactoryGraphTopologyEditorBridge } from "../../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import {
  buildDetailCardCurrentSelection,
  buildDetailCardEditableFactoryDocument,
  buildDetailCardFactoryDocumentQueryResult,
  buildDetailCardFactoryDocumentSaveHookReturn,
  buildDetailCardMultiResourceFactoryDocument,
  buildDetailCardWorkStateFactoryDocument,
  DETAIL_CARD_NOW,
  expandDetailCardResourceConfiguration,
  expandDetailCardWorkerConfiguration,
  expandDetailCardWorkstationConfiguration,
  workstationFooterSaveButton,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { renderWithQueryClient } from "../../widget/current-selection-widget-test-utils";

const saveCurrentFactoryMutation = vi.fn();

const toast = {
  dismiss: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
  warning: vi.fn(),
};
const actualSonner = await import("sonner");
const currentFactoryDefinitionHooks = await import(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition"
);
const useCurrentFactoryDocument = vi.fn<
  typeof currentFactoryDefinitionHooks.useCurrentFactoryDocument
>();
const useFactoryDocumentSave = vi.fn();
const useCurrentWorkstationPromptTemplateValidation = vi.fn();

mock.module("sonner", () => ({ ...actualSonner, toast }));
mock.module(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  () => ({
    ...currentFactoryDefinitionHooks,
    useCurrentFactoryDocument,
  }),
);

mock.module(
  "../../../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave,
  }),
);

mock.module(
  "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation,
  }),
);

const {
  expectGraphDraftConflictWarningToast,
  expectNoGraphDraftConflictWarningToast,
  expectWorkerSaveSuccessToast,
} = await import(
  "../../../base/components/detail-card/current-selection-save-toast-test-helpers"
);
const { CurrentSelectionWidget } = await import(
  "../../widget/current-selection-widget"
);

function resetGraphDraftBridge() {
  useFactoryGraphTopologyEditorBridge.setState({
    graphDraftHasPendingChanges: false,
    handlers: null,
  });
}

function markGraphDraftDirty() {
  useFactoryGraphTopologyEditorBridge
    .getState()
    .setGraphDraftHasPendingChanges(true);
}

function buildDetailCardWorkTypeFactoryDocument(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body: "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
  };
}

async function renderWorkstationSelection() {
  const selectedNode =
    semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;

  const result = renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        selectedNode,
        selection: { kind: "node", nodeId: selectedNode.node_id },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
  await settleCurrentSelectionEffects();
  return result;
}

async function renderWorkerSelection() {
  const result = renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        currentFactoryDefinition: buildDetailCardEditableFactoryDocument(),
        selectedWorkerName: "reviewer",
        selection: { kind: "worker", workerName: "reviewer" },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
  await settleCurrentSelectionEffects();
  return result;
}

async function renderResourceSelection(resourceName = "agent-slot") {
  const result = renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        currentFactoryDefinition: buildDetailCardMultiResourceFactoryDocument(),
        selectedResourceName: resourceName,
        selection: { kind: "resource", resourceName },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
  await settleCurrentSelectionEffects();
  return result;
}

async function renderWorkTypeSelection() {
  const result = renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        selectedWorkTypeName: "story",
        selection: { kind: "work-type", workTypeName: "story" },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
  await settleCurrentSelectionEffects();
  return result;
}

async function renderWorkStateSelection() {
  const selectedStatePlace =
    semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review.input_places?.find(
      (place) => place.place_id === "story:implemented",
    );

  if (!selectedStatePlace) {
    throw new Error("expected implemented state fixture");
  }

  const result = renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        selectedStatePlace,
        selection: {
          kind: "state-node",
          placeId: selectedStatePlace.place_id,
        },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
  await settleCurrentSelectionEffects();
  return result;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: cross-entity graph-draft conflict regressions share one mocked save seam.
describe("CurrentSelectionWidget graph draft conflict warning boundaries", () => {
  useStrictConsoleGuard({
    allowlist: [
      {
        name: "widget-save-notifications-mutation-settle",
        level: "error",
        match: "CurrentSelectionWidgetSaveNotifications",
        reason:
          "Mocked document-save mutations resolve after userEvent; the save-notification subtree re-renders before the enclosing act scope closes.",
      },
    ],
  });

  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
    resetSelectionHistoryStore();
    resetGraphDraftBridge();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(useFactoryDocumentSave).mockReturnValue(
      buildDetailCardFactoryDocumentSaveHookReturn(
        saveCurrentFactoryMutation,
      ) as never,
    );
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue({
      data: {
        diagnostics: [],
        valid: true,
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
    resetSelectionHistoryStore();
    resetGraphDraftBridge();
  });

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: cross-entity topology conflict regressions share one mocked save seam.
  describe("topology-affecting saves with a dirty graph draft", () => {
    it("warns after a workstation worker assignment save", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardEditableFactoryDocument({
            workerOptions: ["reviewer", "planner"],
          }),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue(
        buildDetailCardEditableFactoryDocument({
          workerName: "planner",
          workerOptions: ["reviewer", "planner"],
        }),
      );
      markGraphDraftDirty();

      await renderWorkstationSelection();
      const user = userEvent.setup();
      expandDetailCardWorkstationConfiguration();
      await selectLabeledComboboxOption(user, "Worker", "planner");
      await settleCurrentSelectionEffects();
      await user.click(workstationFooterSaveButton());
      await settleCurrentSelectionEffects();
      await expectGraphDraftConflictWarningToast();
    });

    it("warns after a worker rename save", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardEditableFactoryDocument(),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue(
        buildDetailCardEditableFactoryDocument({
          workerOptions: ["senior-reviewer"],
        }),
      );
      markGraphDraftDirty();

      await renderWorkerSelection();
      expandDetailCardWorkerConfiguration();
      fireEvent.change(screen.getByLabelText("Worker name"), {
        target: { value: "senior-reviewer" },
      });
      await settleCurrentSelectionEffects();
      const saveWorkerButtons = screen.getAllByRole("button", {
        name: "Save worker",
      });
      await userEvent
        .setup()
        .click(
          saveWorkerButtons[saveWorkerButtons.length - 1] ??
            saveWorkerButtons[0],
        );
      await expectGraphDraftConflictWarningToast();
    });

    it("warns after a resource rename save", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardMultiResourceFactoryDocument(),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue({
        ...buildDetailCardMultiResourceFactoryDocument(),
        resources: [
          {
            capacity: 2,
            name: "expanded-slot",
            type: "INVOCATION_SLOT",
          },
          {
            capacity: 5,
            model: "gpt-audio",
            name: "voice-model",
            type: "MODEL",
          },
        ],
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CURSOR",
            name: "reviewer",
            resources: [{ capacity: 1, name: "expanded-slot" }],
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          {
            body: "Review the latest story changes before approval.",
            id: "review",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Review",
            outputs: [{ state: "approved", workType: "story" }],
            promptFile: "prompts/review.md",
            resources: [{ capacity: 1, name: "expanded-slot" }],
            worker: "reviewer",
          },
        ],
      });
      markGraphDraftDirty();

      await renderResourceSelection();
      expandDetailCardResourceConfiguration();
      fireEvent.change(screen.getByLabelText("Name"), {
        target: { value: "expanded-slot" },
      });
      await settleCurrentSelectionEffects();
      await userEvent
        .setup()
        .click(screen.getByRole("button", { name: "Save resource" }));
      await expectGraphDraftConflictWarningToast();
    });

    it("warns after a work type rename save", async () => {
      const initialFactory = buildDetailCardWorkTypeFactoryDocument();
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(initialFactory),
      );
      saveCurrentFactoryMutation.mockResolvedValue({
        ...initialFactory,
        workTypes: [
          {
            name: "feature",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [
          {
            ...initialFactory.workstations?.[0],
            inputs: [{ state: "queued", workType: "feature" }],
            outputs: [{ state: "approved", workType: "feature" }],
          },
        ],
      });
      markGraphDraftDirty();

      await renderWorkTypeSelection();
      fireEvent.change(screen.getByLabelText("Work type"), {
        target: { value: "feature" },
      });
      await settleCurrentSelectionEffects();
      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "Save changes" }));
      await settleCurrentSelectionEffects();
      await user.click(
        screen.getByRole("button", { name: "Overwrite factory" }),
      );
      await settleCurrentSelectionEffects();
      await expectGraphDraftConflictWarningToast();
    });

    it("warns after a work state rename save", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardWorkStateFactoryDocument(),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue(
        buildDetailCardWorkStateFactoryDocument({
          workTypes: [
            {
              name: "story",
              states: [
                { name: "ready", type: "PROCESSING" },
                { name: "complete", type: "TERMINAL" },
                { name: "blocked", type: "FAILED" },
              ],
            },
          ],
        }),
      );
      markGraphDraftDirty();

      await renderWorkStateSelection();
      await waitFor(() => {
        expect(screen.getByLabelText("State name")).toBeTruthy();
      });
      fireEvent.change(screen.getByLabelText("State name"), {
        target: { value: "ready" },
      });
      await settleCurrentSelectionEffects();
      await userEvent
        .setup()
        .click(screen.getByRole("button", { name: "Save work state" }));
      await expectGraphDraftConflictWarningToast();
    });
  });

  describe("non-topology save with a dirty graph draft", () => {
    it("does not warn when saving a worker model change", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardEditableFactoryDocument(),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue(
        buildDetailCardEditableFactoryDocument({ model: "gpt-5.9" }),
      );
      markGraphDraftDirty();

      await renderWorkerSelection();
      fireEvent.change(screen.getByLabelText("Model"), {
        target: { value: "gpt-5.9" },
      });
      await settleCurrentSelectionEffects();
      const saveWorkerButtons = screen.getAllByRole("button", {
        name: "Save worker",
      });
      await userEvent
        .setup()
        .click(
          saveWorkerButtons[saveWorkerButtons.length - 1] ??
            saveWorkerButtons[0],
        );
      await expectWorkerSaveSuccessToast("reviewer");
      await expectNoGraphDraftConflictWarningToast();
    });
  });

  describe("topology-affecting save with a clean graph draft", () => {
    it("does not warn when renaming a work state with a clean graph draft", async () => {
      vi.mocked(useCurrentFactoryDocument).mockReturnValue(
        buildDetailCardFactoryDocumentQueryResult(
          buildDetailCardWorkStateFactoryDocument(),
        ),
      );
      saveCurrentFactoryMutation.mockResolvedValue(
        buildDetailCardWorkStateFactoryDocument({
          workTypes: [
            {
              name: "story",
              states: [
                { name: "ready", type: "PROCESSING" },
                { name: "complete", type: "TERMINAL" },
                { name: "blocked", type: "FAILED" },
              ],
            },
          ],
        }),
      );

      await renderWorkStateSelection();
      await waitFor(() => {
        expect(screen.getByLabelText("State name")).toBeTruthy();
      });
      fireEvent.change(screen.getByLabelText("State name"), {
        target: { value: "ready" },
      });
      await userEvent
        .setup()
        .click(screen.getByRole("button", { name: "Save work state" }));

      await expectNoGraphDraftConflictWarningToast();
    });
  });
});
