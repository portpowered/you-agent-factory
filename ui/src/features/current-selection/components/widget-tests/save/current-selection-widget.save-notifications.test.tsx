import "../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, screen } from "@testing-library/react";
import { toast } from "sonner";

import { CurrentFactoryDefinitionError } from "../../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import {
  currentSelectionConfigurationSection,
  expectNoInlineSaveOutcomesIn,
  expectNoSaveToastDelivery,
  expectResourceSaveFailedToast,
  expectResourceSaveSuccessToast,
  expectWorkerSaveSuccessToast,
  expectWorkstationSaveFailedToast,
} from "../../../base/components/detail-card/current-selection-save-toast-test-helpers";
import {
  buildDetailCardEditableFactoryDocument,
  buildDetailCardFactoryDocumentQueryResult,
  buildDetailCardFactoryDocumentSaveHookReturn,
  buildDetailCardMultiResourceFactoryDocument,
  expandDetailCardResourceConfiguration,
  expandDetailCardWorkstationConfiguration,
} from "../../../base/components/detail-card/detail-card-save-test-helpers";
import {
  buildDetailCardCurrentSelection,
  clickWorkstationSave,
  createDetailCardDeferredFactoryDocumentSave,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: cross-entity save notification regressions share one mocked save seam.
describe("CurrentSelectionWidget save notification delivery", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.warning).mockClear();
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

  it("shows a worker success toast without inline save outcome copy", async () => {
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
    saveCurrentFactoryMutation.mockResolvedValue(
      buildDetailCardEditableFactoryDocument({ model: "gpt-5.9" }),
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition: buildDetailCardEditableFactoryDocument(),
          selectedWorkerName: "reviewer",
          selection: { kind: "worker", workerName: "reviewer" },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );

    fireEvent.change(screen.getByLabelText("Model"), {
      target: { value: "gpt-5.9" },
    });
    const saveWorkerButtons = screen.getAllByRole("button", {
      name: "Save worker",
    });
    fireEvent.click(
      saveWorkerButtons[saveWorkerButtons.length - 1] ?? saveWorkerButtons[0],
    );

    await expectWorkerSaveSuccessToast("reviewer");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Worker configuration"),
    );
  });

  it("shows a resource success toast without inline save outcome copy", async () => {
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
    saveCurrentFactoryMutation.mockResolvedValue({
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

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "agent-slot",
          selection: { kind: "resource", resourceName: "agent-slot" },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();
    fireEvent.change(screen.getByLabelText("Capacity"), {
      target: { value: "4" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save resource" }));

    await expectResourceSaveSuccessToast("agent-slot");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Resource configuration"),
    );
  });

  it("shows a persistent error toast without inline save failure copy", async () => {
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
    saveCurrentFactoryMutation.mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory runtime must be idle before activation.",
        { code: "FACTORY_NOT_IDLE" },
      ),
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition:
            buildDetailCardMultiResourceFactoryDocument(),
          selectedResourceName: "agent-slot",
          selection: { kind: "resource", resourceName: "agent-slot" },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardResourceConfiguration();
    fireEvent.change(screen.getByLabelText("Capacity"), {
      target: { value: "9" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save resource" }));

    await expectResourceSaveFailedToast(
      "Current factory runtime must be idle before activation.",
    );
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Resource configuration"),
    );
  });

  it("does not replay a workstation success toast after switching selection", async () => {
    const selectedNode =
      semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;
    const deferredSave = createDetailCardDeferredFactoryDocumentSave();
    const queryClient = createCurrentSelectionWidgetQueryClient();
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
    saveCurrentFactoryMutation.mockReturnValue(deferredSave.promise);

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Deferred workstation save." },
    });
    clickWorkstationSave();

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition: buildDetailCardEditableFactoryDocument(),
          selectedWorkerName: "reviewer",
          selection: { kind: "worker", workerName: "reviewer" },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );

    deferredSave.resolve(
      buildDetailCardEditableFactoryDocument({
        prompt: "Deferred workstation save.",
      }),
    );

    await expectNoSaveToastDelivery();
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Worker configuration"),
    );
  });

  it("shows a workstation failure toast without inline save failure copy", async () => {
    const selectedNode =
      semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;
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
    saveCurrentFactoryMutation.mockRejectedValue(new Error("Network dropped"));

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={Date.parse("2026-04-08T12:00:04Z")}
        selectedWorkExecutionDetails={null}
      />,
    );
    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft through a generic failure." },
    });
    clickWorkstationSave();

    await expectWorkstationSaveFailedToast("Network dropped");
    expectNoInlineSaveOutcomesIn(
      currentSelectionConfigurationSection("Configuration"),
    );
  });
});
