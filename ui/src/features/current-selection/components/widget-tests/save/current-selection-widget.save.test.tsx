import "../../../../../testing/vitest-dom-capabilities.setup";

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
} from "../../../../../api/current-factory-definition";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { staleFactoryVersionTarget } from "../../../../../testing/factory-validation-target-fixtures";
import { selectLabeledComboboxOption } from "../../../../../testing/select-test-helpers";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import {
  currentSelectionConfigurationSection,
  expectNoInlineSaveOutcomesIn,
  expectNoSaveToastDelivery,
  expectWorkerSaveSuccessToast,
  expectWorkStateSaveSuccessToast,
  expectWorkstationSaveFailedToast,
  expectWorkstationSaveSuccessToast,
  expectWorkstationStaleSaveWarningToast,
} from "../../../base/components/detail-card/current-selection-save-toast-test-helpers";
import {
  buildDetailCardMultiResourceFactoryDocument,
  expandDetailCardResourceConfiguration,
} from "../../../base/components/detail-card/detail-card-save-test-helpers";
import {
  buildDetailCardCurrentSelection,
  buildDetailCardEditableFactoryDocument,
  buildDetailCardFactoryDocumentQueryResult,
  buildDetailCardFactoryDocumentSaveHookReturn,
  buildDetailCardMultiWorkstationFactoryDocument,
  buildDetailCardSharedWorkerFactoryDocument,
  buildDetailCardWorkStateFactoryDocument,
  clickWorkstationSave,
  createDetailCardDeferredFactoryDocumentSave,
  DETAIL_CARD_NOW,
  DETAIL_CARD_SAVE_FACTORY_VERSION,
  expandDetailCardWorkstationConfiguration,
  workstationFooterSaveButton,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { useCurrentWorkstationPromptTemplateValidation } from "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  renderWithExistingQueryClient,
  renderWithQueryClient,
} from "../../widget/current-selection-widget-test-utils";

const saveCurrentFactoryMutation = vi.fn();

vi.mock("sonner", () => ({
  toast: {
    dismiss: vi.fn(),
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

vi.mock(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation: vi.fn(),
  }),
);

describe("CurrentSelectionWidget workstation save flow", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardEditableFactoryDocument(),
      ),
    );
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
  });

  it("blocks saving while prompt diagnostics remain and re-enables save after the draft is corrected", async () => {
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockImplementation(
      (_workstationName, prompt) =>
        ({
          data:
            prompt === "Use {{ .WorkID }}."
              ? {
                  diagnostics: [],
                  valid: true,
                }
              : {
                  diagnostics: [
                    {
                      endOffset: 18,
                      kind: "UNAVAILABLE_VARIABLE",
                      message: "Only input 0 is available.",
                      path: ".Inputs[1]",
                      sourceText: "(index .Inputs 1)",
                      startOffset: 2,
                    },
                  ],
                  valid: false,
                },
          error: null,
          isError: false,
          isPending: false,
          isSuccess: true,
          status: "success",
        }) as never,
    );

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Use {{ (index .Inputs 1).Payload }}." },
    });

    await waitFor(() => {
      expect(
        workstationFooterSaveButton().getAttribute("disabled"),
      ).not.toBeNull();
    });
    expect(screen.getByText("Prompt diagnostics")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Use {{ .WorkID }}." },
    });

    await waitFor(() => {
      expect(workstationFooterSaveButton().getAttribute("disabled")).toBeNull();
    });
    await waitFor(() => {
      expect(screen.queryByText("Prompt diagnostics")).toBeNull();
    });
  });

  it("keeps save enabled after syntax recovery while prompt validation refetches settled results", async () => {
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockImplementation(
      (_workstationName, prompt) => {
        const isValidPrompt = prompt === "Use {{ .WorkID }}.";
        return {
          data: isValidPrompt
            ? {
                diagnostics: [],
                valid: true,
              }
            : {
                diagnostics: [
                  {
                    endOffset: 24,
                    kind: "SYNTAX_ERROR",
                    message: "line 1: unexpected EOF",
                    startOffset: 0,
                  },
                ],
                valid: false,
              },
          error: null,
          isError: false,
          isFetching: isValidPrompt,
          isPending: false,
          isSuccess: true,
          status: "success",
        } as never;
      },
    );

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "{{ if .WorkID }}" },
    });

    await waitFor(() => {
      expect(
        workstationFooterSaveButton().getAttribute("disabled"),
      ).not.toBeNull();
    });

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Use {{ .WorkID }}." },
    });

    await waitFor(() => {
      expect(workstationFooterSaveButton().getAttribute("disabled")).toBeNull();
    });

    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
  });

  it("saves immediately and refreshes the form to the saved workstation values", async () => {
    const savedFactory = buildDetailCardEditableFactoryDocument({
      prompt: "Review the diff and verify browser behavior.",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the diff and verify browser behavior." },
    });
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factory: expect.objectContaining({
            version: {
              logical: "7",
              physical: "2026-05-23T15:52:00Z",
            },
            workstations: [
              expect.objectContaining({
                body: "Review the diff and verify browser behavior.",
              }),
            ],
          }),
        }),
      );
    });
    await expectWorkstationSaveSuccessToast();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the diff and verify browser behavior.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("preserves edited workstation input when the save request fails", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft while the save fails." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast(
      "Current factory runtime must be idle before activation.",
    );
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft while the save fails.",
    );
    expect(workstationFooterSaveButton().getAttribute("disabled")).toBeNull();
  });

  it("shows generic save failures without discarding the dirty draft", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(new Error("Network dropped"));

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft through a generic failure." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast("Network dropped");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft through a generic failure.",
    );
  });

  it("shows a recoverable stale-write warning without discarding the dirty draft", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        {
          code: "STALE_FACTORY_VERSION",
          status: 409,
          targets: [staleFactoryVersionTarget()],
        },
      ),
    );

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft through a stale write." },
    });
    clickWorkstationSave();

    await expectWorkstationStaleSaveWarningToast(
      "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
    );
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft through a stale write.",
    );
    expect(workstationFooterSaveButton().getAttribute("disabled")).toBeNull();
  });

  it("maps targeted save validation failures onto the affected workstation field", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Worker selection must reference a configured worker.",
        {
          code: "BAD_REQUEST",
          status: 400,
          targets: [
            {
              code: "factory.worker.danglingReference",
              message: "Worker selection must reference a configured worker.",
              severity: "error",
              subject: {
                id: "worker",
                location: "DEFINITION",
                type: "WORKSTATION",
              },
            },
          ],
        },
      ),
    );

    const user = userEvent.setup();

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    await selectLabeledComboboxOption(user, "Worker", "planner");
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast(
      "Worker selection must reference a configured worker.",
    );
    expect(screen.getByLabelText("Worker").getAttribute("aria-invalid")).toBe(
      "true",
    );
  });

  it("saves a worker switch through the existing workstation edit flow", async () => {
    const savedFactory = buildDetailCardEditableFactoryDocument({
      workerName: "planner",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const user = userEvent.setup();

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    await selectLabeledComboboxOption(user, "Worker", "planner");
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factory: expect.objectContaining({
            version: {
              logical: "7",
              physical: "2026-05-23T15:52:00Z",
            },
            workstations: [
              expect.objectContaining({
                name: "Review",
                worker: "planner",
              }),
            ],
          }),
        }),
      );
    });
  });

  it("saves a localized behavior selection using the canonical enum value", async () => {
    const savedFactory = buildDetailCardEditableFactoryDocument({
      behavior: "REPEATER",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const user = userEvent.setup();

    renderWorkstationSelection("zh-CN");
    expandDetailCardWorkstationConfiguration("展开可编辑配置", "配置");

    await selectLabeledComboboxOption(user, "类型", "重复器");
    expect(screen.getByRole("combobox", { name: "类型" })).toHaveTextContent(
      "重复器",
    );

    fireEvent.click(
      screen.getAllByRole("button", { name: "保存更改" }).at(-1) ??
        screen.getAllByRole("button", { name: "保存更改" })[0],
    );

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({
            workstations: [
              expect.objectContaining({
                behavior: "REPEATER",
                name: "Review",
              }),
            ],
          }),
        }),
      );
    });
  });

  it("warns in the configuration when newer server values would be overwritten", async () => {
    const user = userEvent.setup();
    const refreshedFactory = buildDetailCardEditableFactoryDocument({
      prompt: "Server changed prompt",
      workerName: "planner",
    });
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    const queryClient = createCurrentSelectionWidgetQueryClient();
    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep my local prompt change." },
    });
    await selectLabeledComboboxOption(user, "Worker", "reviewer");

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(refreshedFactory),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(
      screen.getByText(
        "The running factory changed after you started editing. Saving now will overwrite newer server values for prompt, worker.",
      ),
    ).toBeTruthy();
  });

  it("preserves worker models while saving workstation prompt changes against a shared-worker definition", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardSharedWorkerFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockResolvedValue(
      buildDetailCardSharedWorkerFactoryDocument({
        prompt: "Updated only the review workstation prompt.",
      }),
    );

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated only the review workstation prompt." },
    });
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factory: expect.objectContaining({
            version: {
              logical: "7",
              physical: "2026-05-23T15:52:00Z",
            },
            workers: [
              expect.objectContaining({
                model: "gpt-5.5",
                name: "processor",
              }),
            ],
            workstations: [
              expect.objectContaining({
                body: "Updated only the review workstation prompt.",
                name: "Review",
              }),
              expect.objectContaining({
                body: "Plan the implementation.",
                name: "Plan",
              }),
            ],
          }),
        }),
      );
    });
  });

  it("keeps save feedback scoped to the workstation that started the save after switching selections", async () => {
    const deferredSave =
      createDetailCardDeferredFactoryDocumentSave<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the latest branch diff before approval." },
    });
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
    expect(
      screen.getAllByRole("button", { name: "Saving..." })[0],
    ).toBeTruthy();

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: planNode,
          selection: { kind: "node", nodeId: planNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expect(screen.getByText("Plan")).toBeTruthy();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();

    deferredSave.resolve(
      buildDetailCardMultiWorkstationFactoryDocument({
        reviewPrompt: "Review the latest branch diff before approval.",
      }),
    );

    await expectNoSaveToastDelivery();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
  });

  it("clears saved feedback after leaving workstation detail and returning to the same workstation", async () => {
    const savedFactory = buildDetailCardMultiWorkstationFactoryDocument({
      reviewPrompt: "Review the saved factory before approval.",
    });
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the saved factory before approval." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveSuccessToast();

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(savedFactory),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the saved factory before approval.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("disables workstation save while saving so the action cannot double-submit", async () => {
    const deferredSave =
      createDetailCardDeferredFactoryDocumentSave<CurrentFactoryDocument>();
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    renderWorkstationSelection();
    expandDetailCardWorkstationConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Save this prompt once while the request is pending." },
    });
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });

    const savingButton = screen.getByRole("button", { name: "Saving..." });
    expect(savingButton.getAttribute("disabled")).not.toBeNull();
    fireEvent.click(savingButton);
    expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);

    deferredSave.resolve(
      buildDetailCardEditableFactoryDocument({
        prompt: "Save this prompt once while the request is pending.",
      }),
    );

    await expectWorkstationSaveSuccessToast();
  });

  it("clears saved feedback after switching to another workstation and returning", async () => {
    const savedFactory = buildDetailCardMultiWorkstationFactoryDocument({
      reviewPrompt: "Review the saved factory before approval.",
    });
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the saved factory before approval." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveSuccessToast();

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(savedFactory),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: planNode,
          selection: { kind: "node", nodeId: planNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the saved factory before approval.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("does not leak a failed save message into a different workstation after switching selections", async () => {
    const deferredSave =
      createDetailCardDeferredFactoryDocumentSave<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep the failed review draft scoped here." },
    });
    clickWorkstationSave();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: planNode,
          selection: { kind: "node", nodeId: planNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    deferredSave.reject(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    await expectNoSaveToastDelivery();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("clears failed save feedback after leaving workstation detail and returning to the same workstation", async () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this failed draft from leaking back in." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast(
      "Current factory runtime must be idle before activation.",
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("clears failed save feedback after switching to another workstation and returning", async () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiWorkstationFactoryDocument(),
      ),
    );
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this failed draft from leaking back in." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast(
      "Current factory runtime must be idle before activation.",
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: planNode,
          selection: { kind: "node", nodeId: planNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );
    expect(
      workstationFooterSaveButton().getAttribute("disabled"),
    ).not.toBeNull();
  });
});

describe("CurrentSelectionWidget resource save flow", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardMultiResourceFactoryDocument(),
      ),
    );
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
    resetSelectionHistoryStore();
  });

  it("keeps save feedback scoped to the resource that started the save after switching selections", async () => {
    const deferredSave =
      createDetailCardDeferredFactoryDocumentSave<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "agent-slot",
          selection: { kind: "resource", resourceName: "agent-slot" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();
    fireEvent.change(screen.getByLabelText("Capacity"), {
      target: { value: "4" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save resource" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
    expect(
      screen.getAllByRole("button", { name: "Saving resource..." })[0],
    ).toBeTruthy();

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "voice-model",
          selection: { kind: "resource", resourceName: "voice-model" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();

    expect(screen.getByDisplayValue("5")).toBeTruthy();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Resource configuration"),
    );
    expect(
      screen
        .getByRole("button", { name: "Save resource" })
        .getAttribute("disabled"),
    ).not.toBeNull();

    deferredSave.resolve({
      ...buildDetailCardMultiResourceFactoryDocument(),
      resources: [
        {
          capacity: 4,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
        {
          capacity: 5,
          model: "gpt-audio",
          name: "voice-model",
          type: "MODEL",
        },
      ],
    });

    await expectNoSaveToastDelivery();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Resource configuration"),
    );
    expect((screen.getByLabelText("Capacity") as HTMLInputElement).value).toBe(
      "5",
    );
  });

  it("does not leak a failed resource save message into a different resource after switching selections", async () => {
    const deferredSave =
      createDetailCardDeferredFactoryDocumentSave<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "agent-slot",
          selection: { kind: "resource", resourceName: "agent-slot" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();
    fireEvent.change(screen.getByLabelText("Capacity"), {
      target: { value: "9" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save resource" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "voice-model",
          selection: { kind: "resource", resourceName: "voice-model" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();

    deferredSave.reject(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    await expectNoSaveToastDelivery();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Resource configuration"),
    );
    expect((screen.getByLabelText("Capacity") as HTMLInputElement).value).toBe(
      "5",
    );
  });

  it("resets editable resource fields when switching from resource to worker selection", () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "agent-slot",
          selection: { kind: "resource", resourceName: "agent-slot" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();
    fireEvent.change(screen.getByLabelText("Capacity"), {
      target: { value: "9" },
    });
    expect((screen.getByLabelText("Capacity") as HTMLInputElement).value).toBe(
      "9",
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedWorkerName: "reviewer",
          selection: { kind: "worker", workerName: "reviewer" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(screen.queryByLabelText("Capacity")).toBeNull();
    expect(screen.getByLabelText("Model")).toBeTruthy();
    expect((screen.getByLabelText("Model") as HTMLInputElement).value).toBe(
      "gpt-5.5",
    );
  });
});

describe("CurrentSelectionWidget work state save flow", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardWorkStateFactoryDocument(),
      ),
    );
    vi.mocked(useFactoryDocumentSave).mockReturnValue(
      buildDetailCardFactoryDocumentSaveHookReturn(
        saveCurrentFactoryMutation,
      ) as never,
    );
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("wires header save for editable work state rename and blocks duplicate names", async () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );

    if (!selectedStatePlace) {
      throw new Error("expected implemented state fixture");
    }

    renderWorkStateSelection(selectedStatePlace);

    await waitFor(() => {
      expect(screen.getByLabelText("State name")).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("State name"), {
      target: { value: "complete" },
    });

    await waitFor(() => {
      const saveButtons = screen.getAllByRole("button", {
        name: "Save work state",
      });
      expect(saveButtons).toHaveLength(1);
      expect(saveButtons[0]?.getAttribute("disabled")).not.toBeNull();
    });
    expect(
      screen.getByText(
        'A work state named "complete" already exists for this work type.',
      ),
    ).toBeTruthy();
  });

  it("persists work state rename inline and updates selection to the new place id", async () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
        (place) => place.place_id === "story:implemented",
      );
    const selectStateNode = vi.fn();
    const savedFactory = buildDetailCardWorkStateFactoryDocument({
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
    });

    if (!selectedStatePlace) {
      throw new Error("expected implemented state fixture");
    }

    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkStateSelection(selectedStatePlace, selectStateNode);

    await waitFor(() => {
      expect(screen.getByLabelText("State name")).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("State name"), {
      target: { value: "ready" },
    });
    const saveWorkStateButtons = screen.getAllByRole("button", {
      name: "Save work state",
    });
    expect(saveWorkStateButtons).toHaveLength(1);
    fireEvent.click(saveWorkStateButtons[0]);

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: DETAIL_CARD_SAVE_FACTORY_VERSION,
          factory: expect.objectContaining({
            workTypes: [
              expect.objectContaining({
                name: "story",
                states: expect.arrayContaining([
                  expect.objectContaining({
                    name: "ready",
                    type: "PROCESSING",
                  }),
                ]),
              }),
            ],
          }),
        }),
      );
    });
    await waitFor(() => {
      expect(selectStateNode).toHaveBeenCalledWith("story:ready");
    });
    await expectWorkStateSaveSuccessToast("ready");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Work state configuration"),
    );
  });
});

describe("CurrentSelectionWidget worker save flow", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardEditableFactoryDocument(),
      ),
    );
    vi.mocked(useFactoryDocumentSave).mockReturnValue(
      buildDetailCardFactoryDocumentSaveHookReturn(
        saveCurrentFactoryMutation,
      ) as never,
    );
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("persists worker edits inline without a confirmation dialog", async () => {
    const savedFactory = buildDetailCardEditableFactoryDocument({
      model: "gpt-5.9",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkerSelection();

    fireEvent.change(screen.getByLabelText("Model"), {
      target: { value: "gpt-5.9" },
    });
    const saveWorkerButtons = screen.getAllByRole("button", {
      name: "Save worker",
    });
    fireEvent.click(
      saveWorkerButtons[saveWorkerButtons.length - 1] ?? saveWorkerButtons[0],
    );

    expect(
      screen.queryByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeNull();

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: DETAIL_CARD_SAVE_FACTORY_VERSION,
          factory: expect.objectContaining({
            version: DETAIL_CARD_SAVE_FACTORY_VERSION,
            workers: expect.arrayContaining([
              expect.objectContaining({
                model: "gpt-5.9",
                name: "reviewer",
              }),
            ]),
          }),
        }),
      );
    });
    await expectWorkerSaveSuccessToast("reviewer");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Worker configuration"),
    );
    for (const saveButton of screen.getAllByRole("button", {
      name: "Save worker",
    })) {
      expect(saveButton.getAttribute("disabled")).not.toBeNull();
    }
  });
});

function renderWorkstationSelection(locale?: string) {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

  return renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        selectedNode,
        selection: { kind: "node", nodeId: selectedNode.node_id },
      })}
      locale={locale}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
}

function renderWorkerSelection() {
  return renderWithQueryClient(
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
}

function renderWorkStateSelection(
  selectedStatePlace: NonNullable<
    ReturnType<typeof buildDetailCardCurrentSelection>["selectedStatePlace"]
  >,
  selectStateNode: ReturnType<typeof vi.fn> = vi.fn(),
) {
  return renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildDetailCardCurrentSelection({
        selectStateNode,
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
}
