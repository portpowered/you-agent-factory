import "../../../../testing/bun-current-selection-isolated-hook-mocks";
import { beforeEach, describe, expect, it, vi } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { createDeferredPromise } from "../../../testing/app-shell-export-test-utils";
import {
  factoryDocumentSaveError,
  mockFactoryDocumentSave,
} from "../../../testing/factory-document-save-mocks";
import {
  useCurrentFactoryDocumentMock,
  useSaveCurrentFactoryMock,
} from "../../../../testing/bun-current-factory-definition-public-mocks";
import {
  useCurrentWorkstationPromptTemplateContractMock,
  useCurrentWorkstationPromptTemplateValidationMock,
} from "../../../../testing/bun-current-selection-isolated-hook-mocks";
import { CurrentSelectionWidget } from "./current-selection-widget";
import { resetSelectionHistoryStore } from "../state/selectionHistoryStore";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";

let saveCurrentFactoryMutation: ReturnType<
  typeof mockFactoryDocumentSave
>["mutateAsync"];

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused current-selection save regressions share one render harness and mocked save seam.
describe("CurrentSelectionWidget workstation save flow", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
    useSaveCurrentFactoryMock.mockReturnValue({
      isPending: false,
      mutateAsync: saveCurrentFactoryMutation,
    } as never);
    useCurrentWorkstationPromptTemplateContractMock.mockReturnValue({
      data: {
        availableVariables: [
          {
            category: "ROOT",
            description: "Current work identifier.",
            example: "{{ .WorkID }}",
            path: ".WorkID",
          },
        ],
        inputCount: 1,
        unavailableAccessPatterns: [],
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);
    useCurrentWorkstationPromptTemplateValidationMock.mockReturnValue({
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

  it("keeps the header save action disabled until the workstation draft changes", () => {
    saveCurrentFactoryMutation.mockResolvedValue(
      buildEditableFactoryDefinition(),
    );

    renderWorkstationSelection();
    expandEditableConfiguration();

    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated review instructions." },
    });

    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("blocks saving while prompt diagnostics remain and re-enables save after the draft is corrected", async () => {
    useCurrentWorkstationPromptTemplateValidationMock.mockImplementation(
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
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Use {{ (index .Inputs 1).Payload }}." },
    });

    await waitFor(() => {
      expect(
        screen
          .getByRole("button", { name: "Save changes" })
          .getAttribute("disabled"),
      ).not.toBeNull();
    });
    expect(screen.getByText("Prompt diagnostics")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Use {{ .WorkID }}." },
    });

    await waitFor(() => {
      expect(
        screen
          .getByRole("button", { name: "Save changes" })
          .getAttribute("disabled"),
      ).toBeNull();
    });
    await waitFor(() => {
      expect(screen.queryByText("Prompt diagnostics")).toBeNull();
    });
  });

  it("confirms before saving and refreshes the form to the saved workstation values", async () => {
    const savedFactory = buildEditableFactoryDefinition({
      prompt: "Review the diff and verify browser behavior.",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the diff and verify browser behavior." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      screen.getByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factoryDefinition: expect.objectContaining({
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
    await waitFor(() => {
      expect(
        screen.getByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeTruthy();
    });

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the diff and verify browser behavior.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("preserves edited workstation input when the save request fails", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      factoryDocumentSaveError("factory_not_idle"),
    );

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft while the save fails." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Saving failed. Current factory runtime must be idle before activation.",
        ),
      ).toBeTruthy();
    });

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft while the save fails.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("shows generic save failures without discarding the dirty draft", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      factoryDocumentSaveError("generic"),
    );

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft through a generic failure." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(screen.getByText("Saving failed. Network dropped")).toBeTruthy();
    });

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft through a generic failure.",
    );
  });

  it("shows a recoverable stale-write warning without discarding the dirty draft", async () => {
    saveCurrentFactoryMutation.mockRejectedValue(
      factoryDocumentSaveError("stale_version"),
    );

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft through a stale write." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Current factory definition is stale. Refresh the graph before saving.",
        ),
      ).toBeTruthy();
    });
    expect(
      screen.getByText(
        "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
      ),
    ).toBeTruthy();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft through a stale write.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
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

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Worker"), {
      target: { value: "planner" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Saving failed. Worker selection must reference a configured worker.",
        ),
      ).toBeTruthy();
    });
    expect(screen.getByLabelText("Worker").getAttribute("aria-invalid")).toBe(
      "true",
    );
  });

  it("allows the overwrite confirmation to be cancelled before saving", () => {
    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Changed prompt before cancelling save." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      screen.getByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(saveCurrentFactoryMutation).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Changed prompt before cancelling save.",
    );
  });

  it("saves a worker switch through the existing workstation edit flow", async () => {
    const savedFactory = buildEditableFactoryDefinition({
      workerName: "planner",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Worker"), {
      target: { value: "planner" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factoryDefinition: expect.objectContaining({
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
    const savedFactory = buildEditableFactoryDefinition({
      behavior: "REPEATER",
    });
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    renderWorkstationSelection("zh-CN");
    expandEditableConfiguration("展开可编辑配置", "配置");

    fireEvent.change(screen.getByLabelText("类型"), {
      target: { value: "REPEATER" },
    });
    expect(
      screen.getByRole("option", { name: "重复器" }).getAttribute("value"),
    ).toBe("REPEATER");

    fireEvent.click(screen.getByRole("button", { name: "保存更改" }));
    fireEvent.click(screen.getByRole("button", { name: "覆盖工厂" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          factoryDefinition: expect.objectContaining({
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

  it("warns in the save confirmation when newer server values would be overwritten", () => {
    const refreshedFactory = buildEditableFactoryDefinition({
      prompt: "Server changed prompt",
      workerName: "planner",
    });
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    const queryClient = createCurrentSelectionWidgetQueryClient();
    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep my local prompt change." },
    });
    fireEvent.change(screen.getByLabelText("Worker"), {
      target: { value: "reviewer" },
    });

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(refreshedFactory),
    );

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode,
            selection: { kind: "node", nodeId: selectedNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      screen.getByText(
        "Saving will overwrite newer server values for prompt, worker with the draft currently shown in the editor.",
      ),
    ).toBeTruthy();
  });

  it("preserves worker models while saving workstation prompt changes against a shared-worker definition", async () => {
    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildSharedWorkerFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockResolvedValue(
      buildSharedWorkerFactoryDefinition({
        prompt: "Updated only the review workstation prompt.",
      }),
    );

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated only the review workstation prompt." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledWith(
        expect.objectContaining({
          baseVersion: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          factoryDefinition: expect.objectContaining({
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
    const deferredSave = createDeferredPromise<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the latest branch diff before approval." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
    expect(
      screen.getAllByRole("button", { name: "Saving..." })[0],
    ).toBeTruthy();

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: planNode,
            selection: { kind: "node", nodeId: planNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(screen.getByText("Plan")).toBeTruthy();
    expect(
      screen.queryByText(
        "Running factory saved. The editable workstation values were refreshed to the saved definition.",
      ),
    ).toBeNull();
    expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();

    deferredSave.resolve(
      buildMultiWorkstationEditableFactoryDefinition({
        reviewPrompt: "Review the latest branch diff before approval.",
      }),
    );

    await waitFor(() => {
      expect(
        screen.queryByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeNull();
    });
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
  });

  it("clears saved feedback after leaving workstation detail and returning to the same workstation", async () => {
    const savedFactory = buildMultiWorkstationEditableFactoryDefinition({
      reviewPrompt: "Review the saved factory before approval.",
    });
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the saved factory before approval." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeTruthy();
    });

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(savedFactory),
    );

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection()}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expect(
      screen.queryByText(
        "Running factory saved. The editable workstation values were refreshed to the saved definition.",
      ),
    ).toBeNull();

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: reviewNode,
            selection: { kind: "node", nodeId: reviewNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(
      screen.queryByText(
        "Running factory saved. The editable workstation values were refreshed to the saved definition.",
      ),
    ).toBeNull();
    expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the saved factory before approval.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("disables overwrite confirmation while saving so the workstation action cannot double-submit", async () => {
    const deferredSave = createDeferredPromise<CurrentFactoryDocument>();
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    renderWorkstationSelection();
    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Save this prompt once while the request is pending." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    const confirmButton = screen.getByRole("button", {
      name: "Overwrite factory",
    });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
    const saveDialog = screen.getByRole("dialog", {
      name: "Overwrite the running factory definition?",
    });
    expect(
      within(saveDialog).getByRole("button", { name: "Saving..." }).disabled,
    ).toBe(true);

    fireEvent.click(
      within(saveDialog).getByRole("button", { name: "Saving..." }),
    );
    expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);

    deferredSave.resolve(
      buildEditableFactoryDefinition({
        prompt: "Save this prompt once while the request is pending.",
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeTruthy();
    });
  });

  it("clears saved feedback after switching to another workstation and returning", async () => {
    const savedFactory = buildMultiWorkstationEditableFactoryDefinition({
      reviewPrompt: "Review the saved factory before approval.",
    });
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockResolvedValue(savedFactory);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the saved factory before approval." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeTruthy();
    });

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(savedFactory),
    );

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: planNode,
            selection: { kind: "node", nodeId: planNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(
      screen.queryByText(
        "Running factory saved. The editable workstation values were refreshed to the saved definition.",
      ),
    ).toBeNull();

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: reviewNode,
            selection: { kind: "node", nodeId: reviewNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(
      screen.queryByText(
        "Running factory saved. The editable workstation values were refreshed to the saved definition.",
      ),
    ).toBeNull();
    expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the saved factory before approval.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("does not leak a failed save message into a different workstation after switching selections", async () => {
    const deferredSave = createDeferredPromise<CurrentFactoryDocument>();
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep the failed review draft scoped here." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: planNode,
            selection: { kind: "node", nodeId: planNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    deferredSave.reject(factoryDocumentSaveError("factory_not_idle"));

    await waitFor(() => {
      expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    });
    expect(
      screen.queryByText(
        "Saving failed. Current factory runtime must be idle before activation.",
      ),
    ).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Plan the implementation.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("clears failed save feedback after leaving workstation detail and returning to the same workstation", async () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockRejectedValue(
      factoryDocumentSaveError("factory_not_idle"),
    );

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this failed draft from leaking back in." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Saving failed. Current factory runtime must be idle before activation.",
        ),
      ).toBeTruthy();
    });

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection()}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expect(screen.queryByText(/^Saving failed\./)).toBeNull();

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: reviewNode,
            selection: { kind: "node", nodeId: reviewNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    expect(
      screen.queryByText(
        "Saving failed. Current factory runtime must be idle before activation.",
      ),
    ).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("clears failed save feedback after switching to another workstation and returning", async () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    useCurrentFactoryDocumentMock.mockReturnValue(
      buildEditableDefinitionResult(buildMultiWorkstationEditableFactoryDefinition()),
    );
    saveCurrentFactoryMutation.mockRejectedValue(
      factoryDocumentSaveError("factory_not_idle"),
    );

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: reviewNode,
          selection: { kind: "node", nodeId: reviewNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this failed draft from leaking back in." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Saving failed. Current factory runtime must be idle before activation.",
        ),
      ).toBeTruthy();
    });

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: planNode,
            selection: { kind: "node", nodeId: planNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(screen.queryByText(/^Saving failed\./)).toBeNull();

    rerender(
      <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode: reviewNode,
            selection: { kind: "node", nodeId: reviewNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />,
    );

    expandEditableConfiguration();

    expect(screen.queryByText(/^Saving failed\./)).toBeNull();
    expect(
      screen.queryByText(
        "Saving failed. Current factory runtime must be idle before activation.",
      ),
    ).toBeNull();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });
});

function expandEditableConfiguration(
  buttonName = "Expand editable configuration",
  headingName = "Configuration",
) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  fireEvent.click(
    within(section).getByRole("button", {
      name: buttonName,
    }),
  );
}

function renderWorkstationSelection(locale?: string) {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

  return renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildCurrentSelection({
        selectedNode,
        selection: { kind: "node", nodeId: selectedNode.node_id },
      })}
      locale={locale}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
}

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    failedWorkItems: [],
    openTerminalWorkDetail: () => undefined,
    redoSelection: () => undefined,
    selectedNode: null,
    selectedNodeActiveExecutions: [],
    selectedNodeProviderSessions: [],
    selectedNodeWorkstationRequests: [],
    selectedStateCurrentWorkItems: [],
    selectedStatePlace: null,
    selectedStateTerminalHistoryWorkItems: [],
    selectedStateTokenCount: 0,
    selectedWorkDispatchAttempts: [],
    selectedWorkID: null,
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkWorkstationRequests: [],
    selectedWorkstationRequest: null,
    selection: null,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkByID: () => undefined,
    selectWorkItem: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildEditableDefinitionResult(
  data: CurrentFactoryDocument | undefined,
) {
  return {
    data,
    error: null,
    failureCount: 0,
    failureReason: null,
    fetchStatus: "idle",
    isError: false,
    isFetched: true,
    isFetchedAfterMount: true,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: false,
    isPaused: false,
    isPending: false,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: true,
    isSuccess: true,
    promise: Promise.resolve(data),
    refetch: vi.fn(),
    status: "success",
  } as never;
}

function buildEditableFactoryDefinition(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER";
  prompt?: string;
  workerName?: string;
  workerOptions?: string[];
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: (overrides?.workerOptions ?? ["reviewer", "planner"]).map(
      (name, index) => ({
        model: `gpt-5.${index + 5}`,
        name,
        type: "MODEL_WORKER",
      }),
    ),
    workstations: [
      {
        behavior: overrides?.behavior ?? "STANDARD",
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides?.workerName ?? "reviewer",
      },
    ],
    workTypes: [],
  };
}

function buildMultiWorkstationEditableFactoryDefinition(overrides?: {
  planPrompt?: string;
  reviewPrompt?: string;
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.reviewPrompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
      {
        body: overrides?.planPrompt ?? "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/plan.md",
        worker: "planner",
      },
    ],
    workTypes: [],
  };
}

function buildSharedWorkerFactoryDefinition(overrides?: {
  prompt?: string;
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "processor",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "processor",
      },
      {
        body: "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/plan.md",
        worker: "processor",
      },
    ],
    workTypes: [],
  };
}

